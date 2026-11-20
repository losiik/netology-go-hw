package main

import "fmt"

func main() {
	fmt.Println("Ledger service started")

	transactions = []Transaction{}

	txs := []Transaction{
		{Amount: 350, Category: "Еда", Description: "Обед в столовой", Date: "2026-11-18"},
		{Amount: 1200, Category: "Транспорт", Description: "Проездной на месяц", Date: "2026-11-19"},
		{Amount: 799.99, Category: "Развлечения", Description: "Подписка на кино", Date: "2026-11-20"},
		{Amount: 0, Category: "Еда", Description: "Пустая транзакция", Date: "2026-11-20"},
	}

	for _, tx := range txs {
		if err := AddTransaction(tx); err != nil {
			fmt.Println("Ошибка:", err)
		}
	}

	fmt.Println("Список транзакций:")
	for _, tx := range ListTransactions() {
		fmt.Printf("%d | %s | %-12s | %8.2f | %s\n", tx.ID, tx.Date, tx.Category, tx.Amount, tx.Description)
	}
}
