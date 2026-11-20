package pg

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"ledger/internal/domain"
)

type BudgetRepo struct {
	db *sql.DB
}

func NewBudgetRepo(db *sql.DB) *BudgetRepo {
	return &BudgetRepo{db: db}
}

func (r *BudgetRepo) Upsert(ctx context.Context, b domain.Budget) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO budgets (user_id, category, limit_amount) VALUES ($1, $2, $3)
		 ON CONFLICT (user_id, category) DO UPDATE SET limit_amount = EXCLUDED.limit_amount`,
		b.UserID, b.Category, b.Limit)
	if err != nil {
		return fmt.Errorf("сохранение бюджета: %w", err)
	}
	return nil
}

func (r *BudgetRepo) GetByCategory(ctx context.Context, userID int64, category string) (domain.Budget, error) {
	b := domain.Budget{UserID: userID}
	err := r.db.QueryRowContext(ctx,
		"SELECT category, limit_amount FROM budgets WHERE user_id = $1 AND category = $2", userID, category).
		Scan(&b.Category, &b.Limit)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Budget{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Budget{}, fmt.Errorf("чтение бюджета: %w", err)
	}
	return b, nil
}

func (r *BudgetRepo) List(ctx context.Context, userID int64) ([]domain.Budget, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT category, limit_amount FROM budgets WHERE user_id = $1 ORDER BY category", userID)
	if err != nil {
		return nil, fmt.Errorf("чтение бюджетов: %w", err)
	}
	defer rows.Close()

	result := []domain.Budget{}
	for rows.Next() {
		b := domain.Budget{UserID: userID}
		if err := rows.Scan(&b.Category, &b.Limit); err != nil {
			return nil, fmt.Errorf("чтение бюджета: %w", err)
		}
		result = append(result, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("чтение бюджетов: %w", err)
	}
	return result, nil
}
