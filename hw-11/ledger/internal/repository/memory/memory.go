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

func (r *ExpenseRepo) Categories(ctx context.Context, from, to string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	set := map[string]bool{}
	for _, tx := range r.expenses {
		if tx.Date >= from && tx.Date <= to {
			set[tx.Category] = true
		}
	}
	return slices.Sorted(maps.Keys(set)), nil
}

func (r *ExpenseRepo) SumByCategoryPeriod(ctx context.Context, category, from, to string) (float64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	var total float64
	for _, tx := range r.expenses {
		if tx.Category == category && tx.Date >= from && tx.Date <= to {
			total += tx.Amount
		}
	}
	return total, nil
}

var (
	_ domain.BudgetRepository  = (*BudgetRepo)(nil)
	_ domain.ExpenseRepository = (*ExpenseRepo)(nil)
)
