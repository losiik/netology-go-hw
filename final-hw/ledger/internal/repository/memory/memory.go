package memory

import (
	"context"
	"maps"
	"slices"
	"sort"
	"sync"

	"ledger/internal/domain"
)

type budgetKey struct {
	userID   int64
	category string
}

type BudgetRepo struct {
	mu      sync.Mutex
	budgets map[budgetKey]domain.Budget
}

func NewBudgetRepo() *BudgetRepo {
	return &BudgetRepo{budgets: map[budgetKey]domain.Budget{}}
}

func (r *BudgetRepo) Upsert(_ context.Context, b domain.Budget) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.budgets[budgetKey{b.UserID, b.Category}] = b
	return nil
}

func (r *BudgetRepo) GetByCategory(_ context.Context, userID int64, category string) (domain.Budget, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	b, ok := r.budgets[budgetKey{userID, category}]
	if !ok {
		return domain.Budget{}, domain.ErrNotFound
	}
	return b, nil
}

func (r *BudgetRepo) List(_ context.Context, userID int64) ([]domain.Budget, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := []domain.Budget{}
	for _, b := range r.budgets {
		if b.UserID == userID {
			result = append(result, b)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Category < result[j].Category })
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

func (r *ExpenseRepo) List(_ context.Context, userID int64) ([]domain.Transaction, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := []domain.Transaction{}
	for _, tx := range r.expenses {
		if tx.UserID == userID {
			result = append(result, tx)
		}
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Date != result[j].Date {
			return result[i].Date > result[j].Date
		}
		return result[i].ID > result[j].ID
	})
	return result, nil
}

func (r *ExpenseRepo) SumByCategory(_ context.Context, userID int64, category string) (float64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var total float64
	for _, tx := range r.expenses {
		if tx.UserID == userID && tx.Category == category {
			total += tx.Amount
		}
	}
	return total, nil
}

func (r *ExpenseRepo) Categories(ctx context.Context, userID int64, from, to string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	set := map[string]bool{}
	for _, tx := range r.expenses {
		if tx.UserID == userID && tx.Date >= from && tx.Date <= to {
			set[tx.Category] = true
		}
	}
	return slices.Sorted(maps.Keys(set)), nil
}

func (r *ExpenseRepo) SumByCategoryPeriod(ctx context.Context, userID int64, category, from, to string) (float64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	var total float64
	for _, tx := range r.expenses {
		if tx.UserID == userID && tx.Category == category && tx.Date >= from && tx.Date <= to {
			total += tx.Amount
		}
	}
	return total, nil
}

var (
	_ domain.BudgetRepository  = (*BudgetRepo)(nil)
	_ domain.ExpenseRepository = (*ExpenseRepo)(nil)
)
