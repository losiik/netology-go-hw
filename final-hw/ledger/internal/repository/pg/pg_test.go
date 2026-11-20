package pg

import (
	"context"
	"database/sql"
	"errors"
	"github.com/pressly/goose/v3"
	"os"
	"slices"
	"testing"

	"ledger/internal/db"
	"ledger/internal/domain"
	"ledger/migrations"
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
	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatal(err)
	}
	if err := goose.UpContext(context.Background(), conn, "."); err != nil {
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
		if _, err := repo.GetByCategory(ctx, 1, "еда"); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("ожидали ErrNotFound, получили %v", err)
		}
	})

	t.Run("empty_list", func(t *testing.T) {
		list, err := repo.List(ctx, 1)
		if err != nil {
			t.Fatal(err)
		}
		if list == nil || len(list) != 0 {
			t.Errorf("ожидали пустой срез, получили %#v", list)
		}
	})

	t.Run("upsert_and_get", func(t *testing.T) {
		if err := repo.Upsert(ctx, domain.Budget{UserID: 1, Category: "еда", Limit: 100.5}); err != nil {
			t.Fatal(err)
		}
		if err := repo.Upsert(ctx, domain.Budget{UserID: 1, Category: "еда", Limit: 200.25}); err != nil {
			t.Fatal(err)
		}
		b, err := repo.GetByCategory(ctx, 1, "еда")
		if err != nil {
			t.Fatal(err)
		}
		if b != (domain.Budget{UserID: 1, Category: "еда", Limit: 200.25}) {
			t.Errorf("бюджет %+v", b)
		}
	})

	t.Run("list_sorted", func(t *testing.T) {
		if err := repo.Upsert(ctx, domain.Budget{UserID: 1, Category: "автомобиль", Limit: 1}); err != nil {
			t.Fatal(err)
		}
		list, err := repo.List(ctx, 1)
		if err != nil {
			t.Fatal(err)
		}
		if len(list) != 2 || list[0].Category != "автомобиль" || list[1].Category != "еда" {
			t.Errorf("список %+v", list)
		}
	})

	t.Run("other_user", func(t *testing.T) {
		if err := repo.Upsert(ctx, domain.Budget{UserID: 2, Category: "еда", Limit: 5}); err != nil {
			t.Fatal(err)
		}
		mine, _ := repo.GetByCategory(ctx, 1, "еда")
		other, _ := repo.GetByCategory(ctx, 2, "еда")
		if mine.Limit != 200.25 || other.Limit != 5 {
			t.Errorf("бюджеты пользователей смешались: %+v / %+v", mine, other)
		}
		list, _ := repo.List(ctx, 2)
		if len(list) != 1 {
			t.Errorf("у второго пользователя %d бюджетов", len(list))
		}
	})

	t.Run("check_constraint", func(t *testing.T) {
		if err := repo.Upsert(ctx, domain.Budget{UserID: 1, Category: "кино", Limit: -1}); err == nil {
			t.Error("CHECK limit_amount > 0 должен отклонить отрицательный лимит")
		}
	})
}

func TestExpenseRepo(t *testing.T) {
	conn := openTestDB(t)
	repo := NewExpenseRepo(conn)
	ctx := context.Background()

	t.Run("empty", func(t *testing.T) {
		list, err := repo.List(ctx, 1)
		if err != nil {
			t.Fatal(err)
		}
		if list == nil || len(list) != 0 {
			t.Errorf("ожидали пустой срез, получили %#v", list)
		}
		sum, err := repo.SumByCategory(ctx, 1, "еда")
		if err != nil || sum != 0 {
			t.Errorf("SumByCategory = %v, %v", sum, err)
		}
	})

	for _, tx := range []domain.Transaction{
		{UserID: 1, Amount: 10.5, Category: "еда", Description: "кофе", Date: "2025-09-10"},
		{UserID: 1, Amount: 20, Category: "еда", Date: "2025-09-12"},
		{UserID: 1, Amount: 30, Category: "кино", Description: "билет", Date: "2025-09-10"},
		{UserID: 2, Amount: 999, Category: "еда", Description: "чужая", Date: "2025-09-10"},
	} {
		if _, err := repo.Add(ctx, tx); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("list_order_and_fields", func(t *testing.T) {
		list, err := repo.List(ctx, 1)
		if err != nil {
			t.Fatal(err)
		}
		want := []domain.Transaction{
			{ID: 2, UserID: 1, Amount: 20, Category: "еда", Description: "", Date: "2025-09-12"},
			{ID: 3, UserID: 1, Amount: 30, Category: "кино", Description: "билет", Date: "2025-09-10"},
			{ID: 1, UserID: 1, Amount: 10.5, Category: "еда", Description: "кофе", Date: "2025-09-10"},
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
		sum, err := repo.SumByCategory(ctx, 1, "еда")
		if err != nil {
			t.Fatal(err)
		}
		if sum != 30.5 {
			t.Errorf("SumByCategory(еда) = %v, ожидали 30.5", sum)
		}
	})

	t.Run("categories", func(t *testing.T) {
		got, err := repo.Categories(ctx, 1, "2025-09-10", "2025-09-11")
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(got, []string{"еда", "кино"}) {
			t.Errorf("Categories = %v", got)
		}
		got, err = repo.Categories(ctx, 1, "2020-01-01", "2020-01-31")
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
			got, err := repo.SumByCategoryPeriod(ctx, 1, tc.category, tc.from, tc.to)
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
		if _, err := repo.Categories(cctx, 1, "2025-09-01", "2025-09-30"); !errors.Is(err, context.Canceled) {
			t.Errorf("Categories: ожидали context.Canceled, получили %v", err)
		}
		if _, err := repo.SumByCategoryPeriod(cctx, 1, "еда", "2025-09-01", "2025-09-30"); !errors.Is(err, context.Canceled) {
			t.Errorf("SumByCategoryPeriod: ожидали context.Canceled, получили %v", err)
		}
		if _, err := repo.List(cctx, 1); !errors.Is(err, context.Canceled) {
			t.Errorf("List: ожидали context.Canceled, получили %v", err)
		}
	})

	t.Run("other_user", func(t *testing.T) {
		list, err := repo.List(ctx, 2)
		if err != nil || len(list) != 1 || list[0].Description != "чужая" {
			t.Errorf("List(2) = %+v, %v", list, err)
		}
		cats, _ := repo.Categories(ctx, 3, "2025-09-01", "2025-09-30")
		if len(cats) != 0 {
			t.Errorf("у пользователя без трат категории %v", cats)
		}
	})

	t.Run("bad_date", func(t *testing.T) {
		if _, err := repo.Add(ctx, domain.Transaction{Amount: 1, Category: "еда", Date: "не дата"}); err == nil {
			t.Error("ожидалась ошибка БД для неверной даты")
		}
	})
}
