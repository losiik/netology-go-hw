package memory

import (
	"context"
	"maps"
	"slices"
	"sort"
	"sync"

	"ledger/internal/domain"
)

type BudgetRepo struct {
	mu      sync.Mutex
	budgets map[string]domain.Budget
}

func NewBudgetRepo() *BudgetRepo {
	return &BudgetRepo{budgets: map[string]domain.Budget{}}
}

func (r *BudgetRepo) Upsert(_ context.Context, b domain.Budget) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.budgets[b.Category] = b
	return nil
}

func (r *BudgetRepo) GetByCategory(_ context.Context, category string) (domain.Budget, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	b, ok := r.budgets[category]
	if !ok {
		return domain.Budget{}, domain.ErrNotFound
	}
	return b, nil
}

func (r *BudgetRepo) List(_ context.Context) ([]domain.Budget, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make([]domain.Budget, 0, len(r.budgets))
	for _, c := range slices.Sorted(maps.Keys(r.budgets)) {
		result = append(result, r.budgets[c])
	}
	return result, nil
}

type ExpenseRepo struct {
	mu       sync.Mutex
	expenses []domain.Transaction
}

func NewExpenseRepo() *ExpenseRepo {
	return &ExpenseRepo{}
}

func (r *ExpenseRepo) Add(_ context.Context, tx domain.Transaction) (domain.Transaction, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	tx.ID = len(r.expenses) + 1
	r.expenses = append(r.expenses, tx)
	return tx, nil
}

func (r *ExpenseRepo) List(_ context.Context) ([]domain.Transaction, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := slices.Clone(r.expenses)
	if result == nil {
		result = []domain.Transaction{}
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Date != result[j].Date {
			return result[i].Date > result[j].Date
		}
		return result[i].ID > result[j].ID
	})
	return result, nil
}

func (r *ExpenseRepo) SumByCategory(_ context.Context, category string) (float64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var total float64
	for _, tx := range r.expenses {
		if tx.Category == category {
			total += tx.Amount
		}
	}
	return total, nil
}

func (r *ExpenseRepo) SummaryByCategory(_ context.Context, from, to string) ([]domain.CategorySummary, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	totals := map[string]float64{}
	for _, tx := range r.expenses {
		if tx.Date >= from && tx.Date <= to {
			totals[tx.Category] += tx.Amount
		}
	}
	result := []domain.CategorySummary{}
	for _, c := range slices.Sorted(maps.Keys(totals)) {
		result = append(result, domain.CategorySummary{Category: c, Total: totals[c]})
	}
	return result, nil
}

var (
	_ domain.BudgetRepository  = (*BudgetRepo)(nil)
	_ domain.ExpenseRepository = (*ExpenseRepo)(nil)
)
