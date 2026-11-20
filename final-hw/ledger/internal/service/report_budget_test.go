package service

import (
	"context"
	"testing"

	"ledger/internal/domain"
	"ledger/internal/repository/memory"
)

func TestReportBudgetPercent(t *testing.T) {
	ctx := context.Background()
	svc := newService()
	for _, b := range []domain.Budget{{Category: "еда", Limit: 1000}, {Category: "такси", Limit: 300}} {
		if err := svc.SetBudget(ctx, b); err != nil {
			t.Fatal(err)
		}
	}
	for _, tx := range []domain.Transaction{
		{Amount: 250, Category: "еда", Date: "2025-09-01"},
		{Amount: 125.5, Category: "еда", Date: "2025-09-15"},
		{Amount: 99.99, Category: "кино", Date: "2025-09-20"},
	} {
		if _, err := svc.AddTransaction(ctx, tx); err != nil {
			t.Fatal(err)
		}
	}

	got, err := svc.GetReportSummary(ctx, "2025-09-01", "2025-09-30")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		category string
		total    float64
		budget   float64
		percent  float64
		noBudget bool
	}{
		{category: "еда", total: 375.5, budget: 1000, percent: 37.55},
		{category: "кино", total: 99.99, noBudget: true},
		{category: "такси", total: 0, budget: 300, percent: 0},
	}
	if len(got.Categories) != len(tests) {
		t.Fatalf("категорий %d, ожидали %d: %+v", len(got.Categories), len(tests), got.Categories)
	}
	for i, tc := range tests {
		c := got.Categories[i]
		if c.Category != tc.category || c.Total != tc.total {
			t.Errorf("[%d] = %s %.2f, ожидали %s %.2f", i, c.Category, c.Total, tc.category, tc.total)
		}
		if tc.noBudget {
			if c.Budget != nil || c.BudgetPercent != nil {
				t.Errorf("%s: бюджета нет, а в отчёте есть бюджет", c.Category)
			}
			continue
		}
		if c.Budget == nil || *c.Budget != tc.budget || c.BudgetPercent == nil || *c.BudgetPercent != tc.percent {
			t.Errorf("%s: бюджет или процент неверные: %+v", c.Category, c)
		}
	}
	if got.Total != 475.49 {
		t.Errorf("итог %.2f, ожидали 475.49", got.Total)
	}
}

func TestReportCacheInvalidation(t *testing.T) {
	ctx := context.Background()
	cache := newFakeCache()
	svc := New(memory.NewBudgetRepo(), memory.NewExpenseRepo(), cache)
	from, to := "2025-09-01", "2025-09-30"
	key := ReportCacheKey(0, from, to)

	report := func(t *testing.T) domain.ReportSummary {
		t.Helper()
		r, err := svc.GetReportSummary(ctx, from, to)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}

	if _, err := svc.AddTransaction(ctx, domain.Transaction{Amount: 100, Category: "еда", Date: "2025-09-10"}); err != nil {
		t.Fatal(err)
	}
	report(t)
	if !cache.has(key) {
		t.Fatal("отчёт не попал в кеш")
	}

	steps := []struct {
		name   string
		action func() error
		total  float64
	}{
		{"add_transaction", func() error {
			_, err := svc.AddTransaction(ctx, domain.Transaction{Amount: 50, Category: "еда", Date: "2025-09-11"})
			return err
		}, 150},
		{"set_budget", func() error {
			return svc.SetBudget(ctx, domain.Budget{Category: "еда", Limit: 1000})
		}, 150},
		{"import", func() error {
			_, err := svc.ImportTransactions(ctx, []domain.Transaction{{Amount: 25, Category: "кино", Date: "2025-09-12"}}, 1)
			return err
		}, 175},
	}
	for _, st := range steps {
		t.Run(st.name, func(t *testing.T) {
			if err := st.action(); err != nil {
				t.Fatal(err)
			}
			if cache.has(key) {
				t.Fatal("кеш отчёта не сброшен")
			}
			if r := report(t); r.Total != st.total {
				t.Errorf("итог %.2f, ожидали %.2f", r.Total, st.total)
			}
		})
	}
}
