package ledger

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

var ErrBudgetExceeded = errors.New("budget exceeded")

const dateLayout = "2006-01-02"

type Transaction struct {
	ID          int
	Amount      float64
	Category    string
	Description string
	Date        string
}

func (t Transaction) Validate() error {
	if t.Amount <= 0 {
		return fmt.Errorf("сумма должна быть больше 0, получено %.2f", t.Amount)
	}
	if strings.TrimSpace(t.Category) == "" {
		return errors.New("категория транзакции не может быть пустой")
	}
	if _, err := time.Parse(dateLayout, t.Date); err != nil {
		return fmt.Errorf("неверный формат даты %q, нужен ГГГГ-ММ-ДД", t.Date)
	}
	return nil
}

func AddTransaction(tx Transaction) (Transaction, error) {
	if err := tx.Validate(); err != nil {
		return Transaction{}, fmt.Errorf("невалидная транзакция: %w", err)
	}
	if err := ready(); err != nil {
		return Transaction{}, err
	}

	ctx := context.Background()
	dbtx, err := pg.BeginTx(ctx, nil)
	if err != nil {
		return Transaction{}, fmt.Errorf("начало транзакции: %w", err)
	}
	defer dbtx.Rollback()

	var limit float64
	err = dbtx.QueryRowContext(ctx,
		"SELECT limit_amount FROM budgets WHERE category = $1 FOR UPDATE", tx.Category).Scan(&limit)
	hasBudget := true
	if errors.Is(err, sql.ErrNoRows) {
		hasBudget = false
	} else if err != nil {
		return Transaction{}, fmt.Errorf("чтение бюджета: %w", err)
	}

	if hasBudget {
		var spent float64
		err = dbtx.QueryRowContext(ctx,
			"SELECT COALESCE(SUM(amount), 0) FROM expenses WHERE category = $1", tx.Category).Scan(&spent)
		if err != nil {
			return Transaction{}, fmt.Errorf("подсчёт расходов: %w", err)
		}
		if spent+tx.Amount > limit {
			return Transaction{}, ErrBudgetExceeded
		}
	}

	err = dbtx.QueryRowContext(ctx,
		"INSERT INTO expenses (amount, category, description, date) VALUES ($1, $2, $3, $4) RETURNING id",
		tx.Amount, tx.Category, tx.Description, tx.Date).Scan(&tx.ID)
	if err != nil {
		return Transaction{}, fmt.Errorf("добавление транзакции: %w", err)
	}

	if err := dbtx.Commit(); err != nil {
		return Transaction{}, fmt.Errorf("commit: %w", err)
	}
	return tx, nil
}

func ListTransactions() ([]Transaction, error) {
	if err := ready(); err != nil {
		return nil, err
	}

	rows, err := pg.QueryContext(context.Background(),
		"SELECT id, amount, category, COALESCE(description, ''), date FROM expenses ORDER BY date DESC, id DESC")
	if err != nil {
		return nil, fmt.Errorf("чтение транзакций: %w", err)
	}
	defer rows.Close()

	result := []Transaction{}
	for rows.Next() {
		var tx Transaction
		var date time.Time
		if err := rows.Scan(&tx.ID, &tx.Amount, &tx.Category, &tx.Description, &date); err != nil {
			return nil, fmt.Errorf("чтение транзакции: %w", err)
		}
		tx.Date = date.Format(dateLayout)
		result = append(result, tx)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("чтение транзакций: %w", err)
	}
	return result, nil
}

func CategoryTotal(category string) (float64, error) {
	if err := ready(); err != nil {
		return 0, err
	}
	var total float64
	err := pg.QueryRowContext(context.Background(),
		"SELECT COALESCE(SUM(amount), 0) FROM expenses WHERE category = $1", category).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("подсчёт расходов: %w", err)
	}
	return total, nil
}
