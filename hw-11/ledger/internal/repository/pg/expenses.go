package pg

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"ledger/internal/domain"
)

type ExpenseRepo struct {
	db *sql.DB
}

func NewExpenseRepo(db *sql.DB) *ExpenseRepo {
	return &ExpenseRepo{db: db}
}

func (r *ExpenseRepo) Add(ctx context.Context, tx domain.Transaction) (domain.Transaction, error) {
	err := r.db.QueryRowContext(ctx,
		"INSERT INTO expenses (amount, category, description, date) VALUES ($1, $2, $3, $4) RETURNING id",
		tx.Amount, tx.Category, tx.Description, tx.Date).Scan(&tx.ID)
	if err != nil {
		return domain.Transaction{}, fmt.Errorf("добавление транзакции: %w", err)
	}
	return tx, nil
}

func (r *ExpenseRepo) List(ctx context.Context) ([]domain.Transaction, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT id, amount, category, COALESCE(description, ''), date FROM expenses ORDER BY date DESC, id DESC")
	if err != nil {
		return nil, fmt.Errorf("чтение транзакций: %w", err)
	}
	defer rows.Close()

	result := []domain.Transaction{}
	for rows.Next() {
		var tx domain.Transaction
		var date time.Time
		if err := rows.Scan(&tx.ID, &tx.Amount, &tx.Category, &tx.Description, &date); err != nil {
			return nil, fmt.Errorf("чтение транзакции: %w", err)
		}
		tx.Date = date.Format(domain.DateLayout)
		result = append(result, tx)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("чтение транзакций: %w", err)
	}
	return result, nil
}

func (r *ExpenseRepo) SumByCategory(ctx context.Context, category string) (float64, error) {
	var total float64
	err := r.db.QueryRowContext(ctx,
		"SELECT COALESCE(SUM(amount), 0) FROM expenses WHERE category = $1", category).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("подсчёт расходов: %w", err)
	}
	return total, nil
}

func (r *ExpenseRepo) Categories(ctx context.Context, from, to string) ([]string, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT DISTINCT category FROM expenses WHERE date BETWEEN $1 AND $2 ORDER BY category", from, to)
	if err != nil {
		return nil, fmt.Errorf("чтение категорий: %w", err)
	}
	defer rows.Close()

	result := []string{}
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, fmt.Errorf("чтение категории: %w", err)
		}
		result = append(result, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("чтение категорий: %w", err)
	}
	return result, nil
}

func (r *ExpenseRepo) SumByCategoryPeriod(ctx context.Context, category, from, to string) (float64, error) {
	var total float64
	err := r.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(amount), 0) FROM expenses
		 WHERE category = $1 AND date BETWEEN $2 AND $3`, category, from, to).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("подсчёт расходов за период: %w", err)
	}
	return total, nil
}

var (
	_ domain.BudgetRepository  = (*BudgetRepo)(nil)
	_ domain.ExpenseRepository = (*ExpenseRepo)(nil)
)
