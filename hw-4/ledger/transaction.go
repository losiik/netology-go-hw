package main

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

type Transaction struct {
	ID          int
	Amount      float64
	Category    string
	Description string
	Date        string
}

var transactions []Transaction

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

func AddTransaction(tx Transaction) error {
	if err := tx.Validate(); err != nil {
		return fmt.Errorf("невалидная транзакция: %w", err)
	}
	if b, ok := budgets[tx.Category]; ok {
		if categoryTotal(tx.Category)+tx.Amount > b.Limit {
			return errors.New("budget exceeded")
		}
	}
	tx.ID = len(transactions) + 1
	transactions = append(transactions, tx)
	return nil
}

func ListTransactions() []Transaction {
	result := make([]Transaction, len(transactions))
	copy(result, transactions)
	return result
}
