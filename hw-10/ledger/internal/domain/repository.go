package domain

import (
	"context"
	"time"
)

type BudgetRepository interface {
	Upsert(ctx context.Context, b Budget) error
	GetByCategory(ctx context.Context, category string) (Budget, error)
	List(ctx context.Context) ([]Budget, error)
}

type ExpenseRepository interface {
	Add(ctx context.Context, tx Transaction) (Transaction, error)
	List(ctx context.Context) ([]Transaction, error)
	SumByCategory(ctx context.Context, category string) (float64, error)
	Categories(ctx context.Context, from, to string) ([]string, error)
	SumByCategoryPeriod(ctx context.Context, category, from, to string) (float64, error)
}

type Cache interface {
	Get(ctx context.Context, key string) ([]byte, bool, error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Delete(ctx context.Context, keys ...string) error
}
