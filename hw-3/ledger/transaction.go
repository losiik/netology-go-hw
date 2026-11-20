package main

import "errors"

type Transaction struct {
	ID          int
	Amount      float64
	Category    string
	Description string
	Date        string
}

var transactions []Transaction

func AddTransaction(tx Transaction) error {
	if tx.Amount == 0 {
		return errors.New("сумма транзакции не может быть 0")
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
