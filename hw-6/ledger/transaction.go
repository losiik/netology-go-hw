package ledger

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

var ErrBudgetExceeded = errors.New("budget exceeded")

type Transaction struct {
	ID          int
	Amount      float64
	Category    string
	Description string
	Date        string
}

var (
	mu           sync.Mutex
	transactions = []Transaction{}
)

func (t Transaction) Validate() error {
	if t.Amount <= 0 {
		return fmt.Errorf("сумма должна быть больше 0, получено %.2f", t.Amount)
	}
	if strings.TrimSpace(t.Category) == "" {
		return errors.New("категория транзакции не может быть пустой")
	}
	if _, err := time.Parse("2006-01-02", t.Date); err != nil {
		return fmt.Errorf("неверный формат даты %q, нужен ГГГГ-ММ-ДД", t.Date)
	}
	return nil
}

func AddTransaction(tx Transaction) (Transaction, error) {
	if err := tx.Validate(); err != nil {
		return Transaction{}, fmt.Errorf("невалидная транзакция: %w", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if b, ok := budgets[tx.Category]; ok {
		if categoryTotal(tx.Category)+tx.Amount > b.Limit {
			return Transaction{}, ErrBudgetExceeded
		}
	}
	tx.ID = len(transactions) + 1
	transactions = append(transactions, tx)
	return tx, nil
}

func ListTransactions() []Transaction {
	mu.Lock()
	defer mu.Unlock()

	result := make([]Transaction, len(transactions))
	copy(result, transactions)
	return result
}

func CategoryTotal(category string) float64 {
	mu.Lock()
	defer mu.Unlock()
	return categoryTotal(category)
}

func categoryTotal(category string) float64 {
	var total float64
	for _, tx := range transactions {
		if tx.Category == category {
			total += tx.Amount
		}
	}
	return total
}
