package pg

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"slices"
	"testing"

	"ledger/internal/db"
	"ledger/internal/domain"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL не задан, тест с БД пропущен")
	}
	conn, err := db.Open(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	truncate := func() {
		if _, err := conn.ExecContext(context.Background(), "TRUNCATE expenses, budgets RESTART IDENTITY"); err != nil {
			t.Fatal(err)
		}
	}
	truncate()
	t.Cleanup(func() {
		truncate()
		conn.Close()
	})
	return conn
}

func TestBudgetRepo(t *testing.T) {
	conn := openTestDB(t)
	repo := NewBudgetRepo(conn)
	ctx := context.Background()

	t.Run("not_found", func(t *testing.T) {
		if _, err := repo.GetByCategory(ctx, "еда"); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("ожидали ErrNotFound, получили %v", err)
		}
	})

	t.Run("empty_list", func(t *testing.T) {
		list, err := repo.List(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if list == nil || len(list) != 0 {
			t.Errorf("ожидали пустой срез, получили %#v", list)
		}
	})

	t.Run("upsert_and_get", func(t *testing.T) {
		if err := repo.Upsert(ctx, domain.Budget{Category: "еда", Limit: 100.5}); err != nil {
			t.Fatal(err)
		}
		if err := repo.Upsert(ctx, domain.Budget{Category: "еда", Limit: 200.25}); err != nil {
			t.Fatal(err)
		}
		b, err := repo.GetByCategory(ctx, "еда")
		if err != nil {
			t.Fatal(err)
		}
		if b != (domain.Budget{Category: "еда", Limit: 200.25}) {
			t.Errorf("бюджет %+v", b)
		}
	})

	t.Run("list_sorted", func(t *testing.T) {
		if err := repo.Upsert(ctx, domain.Budget{Category: "автомобиль", Limit: 1}); err != nil {
			t.Fatal(err)
		}
		list, err := repo.List(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(list) != 2 || list[0].Category != "автомобиль" || list[1].Category != "еда" {
			t.Errorf("список %+v", list)
		}
	})

	t.Run("check_constraint", func(t *testing.T) {
		if err := repo.Upsert(ctx, domain.Budget{Category: "кино", Limit: -1}); err == nil {
			t.Error("CHECK limit_amount > 0 должен отклонить отрицательный лимит")
		}
	})
}

func TestExpenseRepo(t *testing.T) {
	conn := openTestDB(t)
	repo := NewExpenseRepo(conn)
	ctx := context.Background()

	t.Run("empty", func(t *testing.T) {
		list, err := repo.List(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if list == nil || len(list) != 0 {
			t.Errorf("ожидали пустой срез, получили %#v", list)
		}
		sum, err := repo.SumByCategory(ctx, "еда")
		if err != nil || sum != 0 {
			t.Errorf("SumByCategory = %v, %v", sum, err)
		}
	})

	for _, tx := range []domain.Transaction{
		{Amount: 10.5, Category: "еда", Description: "кофе", Date: "2025-09-10"},
		{Amount: 20, Category: "еда", Date: "2025-09-12"},
		{Amount: 30, Category: "кино", Description: "билет", Date: "2025-09-10"},
	} {
		if _, err := repo.Add(ctx, tx); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("list_order_and_fields", func(t *testing.T) {
		list, err := repo.List(ctx)
		if err != nil {
			t.Fatal(err)
		}
		want := []domain.Transaction{
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
	})

	t.Run("sum_by_category", func(t *testing.T) {
		sum, err := repo.SumByCategory(ctx, "еда")
		if err != nil {
			t.Fatal(err)
		}
		if sum != 30.5 {
			t.Errorf("SumByCategory(еда) = %v, ожидали 30.5", sum)
		}
	})

	t.Run("categories", func(t *testing.T) {
		got, err := repo.Categories(ctx, "2025-09-10", "2025-09-11")
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(got, []string{"еда", "кино"}) {
			t.Errorf("Categories = %v", got)
		}
		got, err = repo.Categories(ctx, "2020-01-01", "2020-01-31")
		if err != nil || got == nil || len(got) != 0 {
			t.Errorf("ожидали пустой срез, получили %#v, %v", got, err)
		}
	})

	t.Run("sum_by_category_period", func(t *testing.T) {
		tests := []struct {
			category, from, to string
			want               float64
		}{
			{"еда", "2025-09-10", "2025-09-11", 10.5},
			{"еда", "2025-09-01", "2025-09-30", 30.5},
			{"кино", "2025-09-11", "2025-09-30", 0},
			{"такси", "2025-09-01", "2025-09-30", 0},
		}
		for _, tc := range tests {
			got, err := repo.SumByCategoryPeriod(ctx, tc.category, tc.from, tc.to)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("SumByCategoryPeriod(%s, %s, %s) = %v, ожидали %v", tc.category, tc.from, tc.to, got, tc.want)
			}
		}
	})

	t.Run("canceled_context", func(t *testing.T) {
		cctx, cancel := context.WithCancel(ctx)
		cancel()
		if _, err := repo.Categories(cctx, "2025-09-01", "2025-09-30"); !errors.Is(err, context.Canceled) {
			t.Errorf("Categories: ожидали context.Canceled, получили %v", err)
		}
		if _, err := repo.SumByCategoryPeriod(cctx, "еда", "2025-09-01", "2025-09-30"); !errors.Is(err, context.Canceled) {
			t.Errorf("SumByCategoryPeriod: ожидали context.Canceled, получили %v", err)
		}
		if _, err := repo.List(cctx); !errors.Is(err, context.Canceled) {
			t.Errorf("List: ожидали context.Canceled, получили %v", err)
		}
	})

	t.Run("bad_date", func(t *testing.T) {
		if _, err := repo.Add(ctx, domain.Transaction{Amount: 1, Category: "еда", Date: "не дата"}); err == nil {
			t.Error("ожидалась ошибка БД для неверной даты")
		}
	})
}
