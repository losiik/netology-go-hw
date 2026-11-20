package ledger

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
)

func setupDB(t *testing.T) {
	t.Helper()

	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL не задан, тест с БД пропущен")
	}

	if pg == nil {
		cfg, err := ConfigFromEnv()
		if err != nil {
			t.Fatal(err)
		}
		cfg.DatabaseURL = url
		cfg.Redis.DB = 15
		if err := Init(context.Background(), cfg); err != nil {
			t.Fatalf("Init: %v", err)
		}
	}

	if err := Reset(); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	t.Cleanup(func() {
		if err := Reset(); err != nil {
			t.Errorf("Reset: %v", err)
		}
	})
}

func TestMain(m *testing.M) {
	code := m.Run()
	Close()
	os.Exit(code)
}

func TestTransactionValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		tx      Transaction
		wantErr bool
	}{
		{"valid", Transaction{Amount: 100, Category: "еда", Date: "2025-09-10"}, false},
		{"valid_fraction", Transaction{Amount: 0.01, Category: "еда", Date: "2025-09-10"}, false},
		{"zero_amount", Transaction{Amount: 0, Category: "еда", Date: "2025-09-10"}, true},
		{"negative_amount", Transaction{Amount: -50, Category: "еда", Date: "2025-09-10"}, true},
		{"empty_category", Transaction{Amount: 100, Category: "", Date: "2025-09-10"}, true},
		{"spaces_category", Transaction{Amount: 100, Category: "   ", Date: "2025-09-10"}, true},
		{"empty_date", Transaction{Amount: 100, Category: "еда", Date: ""}, true},
		{"wrong_date_format", Transaction{Amount: 100, Category: "еда", Date: "10.09.2025"}, true},
		{"invalid_date", Transaction{Amount: 100, Category: "еда", Date: "2025-13-40"}, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.tx.Validate()
			if tc.wantErr && err == nil {
				t.Errorf("ожидалась ошибка, получили nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("ожидали nil, получили ошибку: %v", err)
			}
		})
	}
}

func TestBudgetValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		b       Budget
		wantErr bool
	}{
		{"valid", Budget{Category: "еда", Limit: 5000}, false},
		{"valid_small", Budget{Category: "еда", Limit: 0.01}, false},
		{"zero_limit", Budget{Category: "еда", Limit: 0}, true},
		{"negative_limit", Budget{Category: "еда", Limit: -100}, true},
		{"empty_category", Budget{Category: "", Limit: 5000}, true},
		{"spaces_category", Budget{Category: "  ", Limit: 5000}, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.b.Validate()
			if tc.wantErr && err == nil {
				t.Errorf("ожидалась ошибка, получили nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("ожидали nil, получили ошибку: %v", err)
			}
		})
	}
}

func TestCheckValid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		v       Validatable
		wantErr bool
	}{
		{"transaction_ok", Transaction{Amount: 1, Category: "еда", Date: "2025-09-10"}, false},
		{"transaction_bad", Transaction{Amount: 0, Category: "еда", Date: "2025-09-10"}, true},
		{"budget_ok", Budget{Category: "еда", Limit: 1}, false},
		{"budget_bad", Budget{Category: "еда", Limit: 0}, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := CheckValid(tc.v)
			if (err != nil) != tc.wantErr {
				t.Errorf("CheckValid() ошибка = %v, ожидали ошибку = %v", err, tc.wantErr)
			}
		})
	}
}

func TestCheckPeriod(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		from, to string
		wantErr  bool
	}{
		{"ok", "2025-09-01", "2025-09-30", false},
		{"same_day", "2025-09-10", "2025-09-10", false},
		{"empty", "", "", true},
		{"bad_from", "01.09.2025", "2025-09-30", true},
		{"bad_to", "2025-09-01", "завтра", true},
		{"from_after_to", "2025-10-01", "2025-09-01", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := checkPeriod(tc.from, tc.to)
			if (err != nil) != tc.wantErr {
				t.Errorf("checkPeriod() ошибка = %v, ожидали ошибку = %v", err, tc.wantErr)
			}
			if err != nil && !errors.Is(err, ErrInvalidPeriod) {
				t.Errorf("ошибка должна оборачивать ErrInvalidPeriod: %v", err)
			}
		})
	}
}

func TestNotInitialized(t *testing.T) {
	if pg != nil {
		t.Skip("ledger уже инициализирован")
	}
	if _, err := ListBudgets(); !errors.Is(err, ErrNotInitialized) {
		t.Errorf("ожидали ErrNotInitialized, получили %v", err)
	}
	if _, err := AddTransaction(Transaction{Amount: 1, Category: "еда", Date: "2025-09-10"}); !errors.Is(err, ErrNotInitialized) {
		t.Errorf("ожидали ErrNotInitialized, получили %v", err)
	}
}

func TestBudgetFlow(t *testing.T) {
	setupDB(t)

	if err := SetBudget(Budget{Category: "еда", Limit: 5000}); err != nil {
		t.Fatalf("SetBudget: %v", err)
	}

	t.Run("within_limit", func(t *testing.T) {
		tx, err := AddTransaction(Transaction{Amount: 3000, Category: "еда", Description: "продукты", Date: "2025-09-10"})
		if err != nil {
			t.Fatalf("ожидали nil, получили %v", err)
		}
		if tx.ID != 1 {
			t.Errorf("ID = %d, ожидали 1", tx.ID)
		}
	})

	t.Run("exactly_limit", func(t *testing.T) {
		if _, err := AddTransaction(Transaction{Amount: 2000, Category: "еда", Date: "2025-09-11"}); err != nil {
			t.Fatalf("ожидали nil, получили %v", err)
		}
	})

	t.Run("exceeded", func(t *testing.T) {
		before, err := ListTransactions()
		if err != nil {
			t.Fatal(err)
		}
		_, err = AddTransaction(Transaction{Amount: 1, Category: "еда", Date: "2025-09-12"})
		if !errors.Is(err, ErrBudgetExceeded) {
			t.Fatalf("ожидали ErrBudgetExceeded, получили %v", err)
		}
		after, err := ListTransactions()
		if err != nil {
			t.Fatal(err)
		}
		if len(after) != len(before) {
			t.Errorf("транзакций было %d, стало %d", len(before), len(after))
		}
	})

	t.Run("other_category_no_budget", func(t *testing.T) {
		if _, err := AddTransaction(Transaction{Amount: 100000, Category: "кино", Date: "2025-09-12"}); err != nil {
			t.Fatalf("ожидали nil, получили %v", err)
		}
	})

	t.Run("raise_limit", func(t *testing.T) {
		if err := SetBudget(Budget{Category: "еда", Limit: 6000}); err != nil {
			t.Fatalf("SetBudget: %v", err)
		}
		if _, err := AddTransaction(Transaction{Amount: 1000, Category: "еда", Date: "2025-09-13"}); err != nil {
			t.Fatalf("ожидали nil после увеличения лимита, получили %v", err)
		}
	})

	t.Run("totals", func(t *testing.T) {
		list, err := ListTransactions()
		if err != nil {
			t.Fatal(err)
		}
		if len(list) != 4 {
			t.Errorf("транзакций %d, ожидали 4", len(list))
		}
		total, err := CategoryTotal("еда")
		if err != nil {
			t.Fatal(err)
		}
		if total != 6000 {
			t.Errorf("CategoryTotal(еда) = %v, ожидали 6000", total)
		}
	})
}

func TestListTransactionsOrderAndFields(t *testing.T) {
	setupDB(t)

	for _, tx := range []Transaction{
		{Amount: 10.5, Category: "еда", Description: "кофе", Date: "2025-09-10"},
		{Amount: 20, Category: "еда", Date: "2025-09-12"},
		{Amount: 30, Category: "кино", Description: "билет", Date: "2025-09-10"},
	} {
		if _, err := AddTransaction(tx); err != nil {
			t.Fatal(err)
		}
	}

	list, err := ListTransactions()
	if err != nil {
		t.Fatal(err)
	}
	want := []Transaction{
		{ID: 2, Amount: 20, Category: "еда", Description: "", Date: "2025-09-12"},
		{ID: 3, Amount: 30, Category: "кино", Description: "билет", Date: "2025-09-10"},
		{ID: 1, Amount: 10.5, Category: "еда", Description: "кофе", Date: "2025-09-10"},
	}
	if len(list) != len(want) {
		t.Fatalf("транзакций %d, ожидали %d", len(list), len(want))
	}
	for i := range want {
		if list[i] != want[i] {
			t.Errorf("[%d] = %+v, ожидали %+v", i, list[i], want[i])
		}
	}
}

func TestAddTransactionInvalid(t *testing.T) {
	setupDB(t)

	_, err := AddTransaction(Transaction{Amount: -1, Category: "еда", Date: "2025-09-10"})
	if err == nil {
		t.Fatal("ожидалась ошибка валидации")
	}
	if errors.Is(err, ErrBudgetExceeded) {
		t.Errorf("ошибка валидации не должна быть ErrBudgetExceeded")
	}
	list, err := ListTransactions()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Errorf("транзакций %d, ожидали 0", len(list))
	}
}

func TestSetBudget(t *testing.T) {
	setupDB(t)

	t.Run("invalid", func(t *testing.T) {
		if err := SetBudget(Budget{Category: "еда", Limit: 0}); err == nil {
			t.Fatal("ожидалась ошибка")
		}
	})

	t.Run("upsert", func(t *testing.T) {
		if err := SetBudget(Budget{Category: "еда", Limit: 100}); err != nil {
			t.Fatal(err)
		}
		if err := SetBudget(Budget{Category: "еда", Limit: 200}); err != nil {
			t.Fatal(err)
		}
		list, err := ListBudgets()
		if err != nil {
			t.Fatal(err)
		}
		if len(list) != 1 || list[0].Limit != 200 {
			t.Errorf("бюджеты %+v, ожидали один с лимитом 200", list)
		}
	})
}

func TestListBudgetsCache(t *testing.T) {
	setupDB(t)
	ctx := context.Background()

	if err := SetBudget(Budget{Category: "еда", Limit: 100}); err != nil {
		t.Fatal(err)
	}

	t.Run("miss_then_set", func(t *testing.T) {
		if n := rdb.Exists(ctx, budgetsCacheKey).Val(); n != 0 {
			t.Fatalf("ключ %s уже есть в кеше", budgetsCacheKey)
		}
		if _, err := ListBudgets(); err != nil {
			t.Fatal(err)
		}
		if n := rdb.Exists(ctx, budgetsCacheKey).Val(); n != 1 {
			t.Errorf("после ListBudgets ключ %s не появился в кеше", budgetsCacheKey)
		}
		if ttl := rdb.TTL(ctx, budgetsCacheKey).Val(); ttl <= 0 || ttl > budgetsCacheTTL {
			t.Errorf("TTL = %v, ожидали (0, %v]", ttl, budgetsCacheTTL)
		}
	})

	t.Run("hit_returns_cached", func(t *testing.T) {
		if _, err := pg.Exec("UPDATE budgets SET limit_amount = 999 WHERE category = 'еда'"); err != nil {
			t.Fatal(err)
		}
		list, err := ListBudgets()
		if err != nil {
			t.Fatal(err)
		}
		if len(list) != 1 || list[0].Limit != 100 {
			t.Errorf("ожидали значение из кеша (100), получили %+v", list)
		}
	})

	t.Run("set_budget_invalidates", func(t *testing.T) {
		if err := SetBudget(Budget{Category: "кино", Limit: 50}); err != nil {
			t.Fatal(err)
		}
		if n := rdb.Exists(ctx, budgetsCacheKey).Val(); n != 0 {
			t.Errorf("после SetBudget ключ %s должен быть удалён", budgetsCacheKey)
		}
		list, err := ListBudgets()
		if err != nil {
			t.Fatal(err)
		}
		want := []Budget{{Category: "еда", Limit: 999}, {Category: "кино", Limit: 50}}
		if len(list) != 2 || list[0] != want[0] || list[1] != want[1] {
			t.Errorf("бюджеты %+v, ожидали %+v", list, want)
		}
	})
}

func TestGetReportSummary(t *testing.T) {
	setupDB(t)
	ctx := context.Background()

	for _, tx := range []Transaction{
		{Amount: 100.10, Category: "еда", Date: "2025-09-01"},
		{Amount: 200.20, Category: "еда", Date: "2025-09-15"},
		{Amount: 300, Category: "кино", Date: "2025-09-30"},
		{Amount: 999, Category: "еда", Date: "2025-10-01"},
	} {
		if _, err := AddTransaction(tx); err != nil {
			t.Fatal(err)
		}
	}

	from, to := "2025-09-01", "2025-09-30"
	key := reportCacheKey(from, to)

	t.Run("cache_miss", func(t *testing.T) {
		report, err := GetReportSummary(ctx, from, to)
		if err != nil {
			t.Fatal(err)
		}
		if report.Total != 600.30 {
			t.Errorf("Total = %v, ожидали 600.30", report.Total)
		}
		want := []CategorySummary{{"еда", 300.30}, {"кино", 300}}
		if len(report.Categories) != 2 || report.Categories[0] != want[0] || report.Categories[1] != want[1] {
			t.Errorf("Categories = %+v, ожидали %+v", report.Categories, want)
		}
		if n := rdb.Exists(ctx, key).Val(); n != 1 {
			t.Errorf("ключ %s не появился в кеше", key)
		}
		if ttl := rdb.TTL(ctx, key).Val(); ttl <= 0 || ttl > reportCacheTTL {
			t.Errorf("TTL = %v, ожидали (0, %v]", ttl, reportCacheTTL)
		}
	})

	t.Run("cache_hit", func(t *testing.T) {
		if _, err := pg.Exec("DELETE FROM expenses"); err != nil {
			t.Fatal(err)
		}
		report, err := GetReportSummary(ctx, from, to)
		if err != nil {
			t.Fatal(err)
		}
		if report.Total != 600.30 {
			t.Errorf("ожидали отчёт из кеша (600.30), получили %v", report.Total)
		}
	})

	t.Run("empty_period", func(t *testing.T) {
		report, err := GetReportSummary(ctx, "2020-01-01", "2020-01-31")
		if err != nil {
			t.Fatal(err)
		}
		if report.Total != 0 || report.Categories == nil || len(report.Categories) != 0 {
			t.Errorf("ожидали пустой отчёт, получили %+v", report)
		}
	})

	t.Run("invalid_period", func(t *testing.T) {
		if _, err := GetReportSummary(ctx, "2025-10-01", "2025-09-01"); !errors.Is(err, ErrInvalidPeriod) {
			t.Errorf("ожидали ErrInvalidPeriod, получили %v", err)
		}
	})
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
			setupDB(t)

			err := LoadBudgets(strings.NewReader(tc.input))
			if tc.wantErr == "" && err != nil {
				t.Fatalf("ожидали nil, получили %v", err)
			}
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("ожидали ошибку с %q, получили nil", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("ошибка %q не содержит %q", err.Error(), tc.wantErr)
				}
			}
			list, err := ListBudgets()
			if err != nil {
				t.Fatal(err)
			}
			if len(list) != tc.wantCount {
				t.Errorf("бюджетов %d, ожидали %d", len(list), tc.wantCount)
			}
		})
	}
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) {
	return 0, errors.New("read failed")
}

func TestLoadBudgetsReadError(t *testing.T) {
	err := LoadBudgets(errReader{})
	if err == nil || !strings.Contains(err.Error(), "не удалось прочитать") {
		t.Errorf("ожидали ошибку чтения, получили %v", err)
	}
}
