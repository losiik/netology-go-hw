package service

import (
	"context"
	"errors"
	"io"
	"log"
	"maps"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"ledger/internal/domain"
	"ledger/internal/repository/memory"
)

func TestMain(m *testing.M) {
	log.SetOutput(io.Discard)
	os.Exit(m.Run())
}

type fakeCache struct {
	mu   sync.Mutex
	data map[string][]byte
	ttl  map[string]time.Duration
	hits int
}

func newFakeCache() *fakeCache {
	return &fakeCache{data: map[string][]byte{}, ttl: map[string]time.Duration{}}
}

func (c *fakeCache) Get(_ context.Context, key string) ([]byte, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.data[key]
	if ok {
		c.hits++
	}
	return v, ok, nil
}

func (c *fakeCache) Set(_ context.Context, key string, value []byte, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data[key] = value
	c.ttl[key] = ttl
	return nil
}

func (c *fakeCache) Delete(_ context.Context, keys ...string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, k := range keys {
		delete(c.data, k)
	}
	return nil
}

func (c *fakeCache) has(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.data[key]
	return ok
}

type countingExpenses struct {
	*memory.ExpenseRepo
	summaryCalls int
}

func (r *countingExpenses) Categories(ctx context.Context, from, to string) ([]string, error) {
	r.summaryCalls++
	return r.ExpenseRepo.Categories(ctx, from, to)
}

type brokenRepo struct{}

var errDB = errors.New("db is down")

func (brokenRepo) Upsert(context.Context, domain.Budget) error { return errDB }
func (brokenRepo) GetByCategory(context.Context, string) (domain.Budget, error) {
	return domain.Budget{}, errDB
}
func (brokenRepo) List(context.Context) ([]domain.Budget, error) { return nil, errDB }

func newService() Service {
	return New(memory.NewBudgetRepo(), memory.NewExpenseRepo(), nil)
}

func TestBudgetFlow(t *testing.T) {
	ctx := context.Background()
	svc := newService()

	if err := svc.SetBudget(ctx, domain.Budget{Category: "еда", Limit: 5000}); err != nil {
		t.Fatalf("SetBudget: %v", err)
	}

	t.Run("within_limit", func(t *testing.T) {
		tx, err := svc.AddTransaction(ctx, domain.Transaction{Amount: 3000, Category: "еда", Date: "2025-09-10"})
		if err != nil {
			t.Fatalf("ожидали nil, получили %v", err)
		}
		if tx.ID != 1 {
			t.Errorf("ID = %d, ожидали 1", tx.ID)
		}
	})

	t.Run("exactly_limit", func(t *testing.T) {
		if _, err := svc.AddTransaction(ctx, domain.Transaction{Amount: 2000, Category: "еда", Date: "2025-09-11"}); err != nil {
			t.Fatalf("ожидали nil, получили %v", err)
		}
	})

	t.Run("exceeded", func(t *testing.T) {
		before, _ := svc.ListTransactions(ctx)
		_, err := svc.AddTransaction(ctx, domain.Transaction{Amount: 0.01, Category: "еда", Date: "2025-09-12"})
		if !errors.Is(err, domain.ErrBudgetExceeded) {
			t.Fatalf("ожидали ErrBudgetExceeded, получили %v", err)
		}
		after, _ := svc.ListTransactions(ctx)
		if len(after) != len(before) {
			t.Errorf("транзакций было %d, стало %d", len(before), len(after))
		}
	})

	t.Run("no_budget_category", func(t *testing.T) {
		if _, err := svc.AddTransaction(ctx, domain.Transaction{Amount: 100000, Category: "кино", Date: "2025-09-12"}); err != nil {
			t.Fatalf("ожидали nil, получили %v", err)
		}
	})

	t.Run("raise_limit", func(t *testing.T) {
		if err := svc.SetBudget(ctx, domain.Budget{Category: "еда", Limit: 6000}); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.AddTransaction(ctx, domain.Transaction{Amount: 1000, Category: "еда", Date: "2025-09-13"}); err != nil {
			t.Fatalf("ожидали nil после увеличения лимита, получили %v", err)
		}
	})

	t.Run("list", func(t *testing.T) {
		list, err := svc.ListTransactions(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(list) != 4 {
			t.Fatalf("транзакций %d, ожидали 4", len(list))
		}
		if list[0].Date != "2025-09-13" {
			t.Errorf("первой должна быть самая новая транзакция, получили %+v", list[0])
		}
	})
}

func TestFloatLimit(t *testing.T) {
	ctx := context.Background()
	svc := newService()

	if err := svc.SetBudget(ctx, domain.Budget{Category: "еда", Limit: 0.3}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddTransaction(ctx, domain.Transaction{Amount: 0.1, Category: "еда", Date: "2025-09-10"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddTransaction(ctx, domain.Transaction{Amount: 0.2, Category: "еда", Date: "2025-09-10"}); err != nil {
		t.Errorf("0.1 + 0.2 не превышает лимит 0.3, получили %v", err)
	}
}

func TestValidationErrors(t *testing.T) {
	ctx := context.Background()
	svc := newService()

	t.Run("transaction", func(t *testing.T) {
		_, err := svc.AddTransaction(ctx, domain.Transaction{Amount: -1, Category: "еда", Date: "2025-09-10"})
		if !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("ожидали ErrValidation, получили %v", err)
		}
		if !strings.Contains(err.Error(), "сумма") {
			t.Errorf("текст ошибки %q", err.Error())
		}
		if list, _ := svc.ListTransactions(ctx); len(list) != 0 {
			t.Errorf("транзакция сохранилась: %+v", list)
		}
	})

	t.Run("budget", func(t *testing.T) {
		err := svc.SetBudget(ctx, domain.Budget{Category: "еда", Limit: 0})
		if !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("ожидали ErrValidation, получили %v", err)
		}
		if list, _ := svc.ListBudgets(ctx); len(list) != 0 {
			t.Errorf("бюджет сохранился: %+v", list)
		}
	})

	t.Run("report_period", func(t *testing.T) {
		_, err := svc.GetReportSummary(ctx, "2025-10-01", "2025-09-01")
		if !errors.Is(err, domain.ErrValidation) || !errors.Is(err, domain.ErrInvalidPeriod) {
			t.Errorf("ожидали ErrValidation и ErrInvalidPeriod, получили %v", err)
		}
	})
}

func TestRepositoryErrors(t *testing.T) {
	ctx := context.Background()
	svc := New(brokenRepo{}, memory.NewExpenseRepo(), nil)

	if err := svc.SetBudget(ctx, domain.Budget{Category: "еда", Limit: 1}); !errors.Is(err, errDB) {
		t.Errorf("SetBudget: ожидали errDB, получили %v", err)
	}
	if _, err := svc.ListBudgets(ctx); !errors.Is(err, errDB) {
		t.Errorf("ListBudgets: ожидали errDB, получили %v", err)
	}
	_, err := svc.AddTransaction(ctx, domain.Transaction{Amount: 1, Category: "еда", Date: "2025-09-10"})
	if !errors.Is(err, errDB) {
		t.Errorf("AddTransaction: ожидали errDB, получили %v", err)
	}
	if errors.Is(err, domain.ErrValidation) {
		t.Error("ошибка БД не должна быть ошибкой валидации")
	}
}

func TestReportSummary(t *testing.T) {
	ctx := context.Background()
	cache := newFakeCache()
	expenses := &countingExpenses{ExpenseRepo: memory.NewExpenseRepo()}
	svc := New(memory.NewBudgetRepo(), expenses, cache)

	for _, tx := range []domain.Transaction{
		{Amount: 100.10, Category: "еда", Date: "2025-09-01"},
		{Amount: 200.20, Category: "еда", Date: "2025-09-15"},
		{Amount: 300, Category: "кино", Date: "2025-09-30"},
		{Amount: 999, Category: "еда", Date: "2025-10-01"},
	} {
		if _, err := svc.AddTransaction(ctx, tx); err != nil {
			t.Fatal(err)
		}
	}

	from, to := "2025-09-01", "2025-09-30"
	key := ReportCacheKey(from, to)
	want := domain.ReportSummary{
		From: from, To: to, Total: 600.30,
		Categories: map[string]float64{"еда": 300.30, "кино": 300},
	}
	check := func(t *testing.T, got domain.ReportSummary) {
		t.Helper()
		if got.Total != want.Total || !maps.Equal(got.Categories, want.Categories) {
			t.Errorf("отчёт %+v, ожидали %+v", got, want)
		}
	}

	t.Run("cache_miss", func(t *testing.T) {
		got, err := svc.GetReportSummary(ctx, from, to)
		if err != nil {
			t.Fatal(err)
		}
		check(t, got)
		if expenses.summaryCalls != 1 {
			t.Errorf("обращений к репозиторию %d, ожидали 1", expenses.summaryCalls)
		}
		if !cache.has(key) || cache.ttl[key] != ReportCacheTTL {
			t.Errorf("отчёт не записан в кеш с TTL %v", ReportCacheTTL)
		}
	})

	t.Run("cache_hit", func(t *testing.T) {
		got, err := svc.GetReportSummary(ctx, from, to)
		if err != nil {
			t.Fatal(err)
		}
		check(t, got)
		if expenses.summaryCalls != 1 {
			t.Errorf("при попадании в кеш репозиторий вызван повторно (%d)", expenses.summaryCalls)
		}
		if cache.hits != 1 {
			t.Errorf("попаданий в кеш %d, ожидали 1", cache.hits)
		}
	})

	t.Run("empty_period", func(t *testing.T) {
		got, err := svc.GetReportSummary(ctx, "2020-01-01", "2020-01-31")
		if err != nil {
			t.Fatal(err)
		}
		if got.Total != 0 || got.Categories == nil || len(got.Categories) != 0 {
			t.Errorf("ожидали пустой отчёт, получили %+v", got)
		}
	})
}

func TestBudgetsCache(t *testing.T) {
	ctx := context.Background()
	cache := newFakeCache()
	svc := New(memory.NewBudgetRepo(), memory.NewExpenseRepo(), cache)

	if err := svc.SetBudget(ctx, domain.Budget{Category: "еда", Limit: 100}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ListBudgets(ctx); err != nil {
		t.Fatal(err)
	}
	if !cache.has(BudgetsCacheKey) {
		t.Fatalf("после ListBudgets ключ %s не появился в кеше", BudgetsCacheKey)
	}

	if err := svc.SetBudget(ctx, domain.Budget{Category: "кино", Limit: 50}); err != nil {
		t.Fatal(err)
	}
	if cache.has(BudgetsCacheKey) {
		t.Errorf("после SetBudget ключ %s должен быть удалён", BudgetsCacheKey)
	}

	list, err := svc.ListBudgets(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].Category != "еда" || list[1].Category != "кино" {
		t.Errorf("бюджеты %+v", list)
	}

	cache.data[BudgetsCacheKey] = []byte("{broken")
	if list, err := svc.ListBudgets(ctx); err != nil || len(list) != 2 {
		t.Errorf("битый кеш должен игнорироваться: %+v, %v", list, err)
	}
}

func TestLoadBudgets(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantErr   string
		wantCount int
	}{
		{"ok", `[{"category":"еда","limit":5000},{"category":"кино","limit":1000}]`, "", 2},
		{"ok_with_bom", "\xef\xbb\xbf" + `[{"category":"еда","limit":5000}]`, "", 1},
		{"empty_array", `[]`, "", 0},
		{"bad_json", `[{"category":`, "не удалось разобрать JSON", 0},
		{"wrong_type", `[{"category":"еда","limit":"abc"}]`, "не удалось разобрать JSON", 0},
		{"invalid_limit", `[{"category":"еда","limit":100},{"category":"кино","limit":-1}]`, "бюджет #2", 0},
		{"empty_category", `[{"category":"","limit":100}]`, "бюджет #1", 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			svc := newService()

			err := svc.LoadBudgets(ctx, strings.NewReader(tc.input))
			if tc.wantErr == "" && err != nil {
				t.Fatalf("ожидали nil, получили %v", err)
			}
			if tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)) {
				t.Fatalf("ожидали ошибку с %q, получили %v", tc.wantErr, err)
			}
			list, _ := svc.ListBudgets(ctx)
			if len(list) != tc.wantCount {
				t.Errorf("бюджетов %d, ожидали %d", len(list), tc.wantCount)
			}
		})
	}
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("read failed") }

func TestLoadBudgetsReadError(t *testing.T) {
	err := newService().LoadBudgets(context.Background(), errReader{})
	if err == nil || !strings.Contains(err.Error(), "не удалось прочитать") {
		t.Errorf("ожидали ошибку чтения, получили %v", err)
	}
}

func TestConcurrentAddRespectsLimit(t *testing.T) {
	ctx := context.Background()
	svc := newService()
	if err := svc.SetBudget(ctx, domain.Budget{Category: "еда", Limit: 100}); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			svc.AddTransaction(ctx, domain.Transaction{Amount: 10, Category: "еда", Date: "2025-09-10"})
		}()
	}
	wg.Wait()

	list, _ := svc.ListTransactions(ctx)
	if len(list) != 10 {
		t.Errorf("добавлено %d транзакций по 10 при лимите 100, ожидали 10", len(list))
	}
}
