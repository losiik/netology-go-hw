package main

import (
	"bufio"
	"fmt"
	"maps"
	"os"
	"slices"
)

func add(tx Transaction) {
	if err := AddTransaction(tx); err != nil {
		fmt.Printf("Отказ: %s %.2f (%s): %v\n", tx.Category, tx.Amount, tx.Description, err)
		return
	}
	fmt.Printf("Добавлено: %s %.2f (%s)\n", tx.Category, tx.Amount, tx.Description)
}

func printBudgets() {
	fmt.Println("Бюджеты:")
	for _, c := range slices.Sorted(maps.Keys(budgets)) {
		b := budgets[c]
		fmt.Printf("  %-12s лимит %8.2f, потрачено %8.2f\n", b.Category, b.Limit, categoryTotal(b.Category))
	}
}

func printTransactions() {
	fmt.Println("Список транзакций:")
	for _, tx := range ListTransactions() {
		fmt.Printf("%d | %s | %-12s | %8.2f | %s\n", tx.ID, tx.Date, tx.Category, tx.Amount, tx.Description)
	}
}

func main() {
	fmt.Println("Ledger service started")

	transactions = []Transaction{}
	budgets = map[string]Budget{}

	SetBudget(Budget{Category: "Еда", Limit: 5000, Period: "2026-11"})

	f, err := os.Open("budgets.json")
	if err != nil {
		fmt.Println("Ошибка открытия файла:", err)
	} else {
		if err := LoadBudgets(bufio.NewReader(f)); err != nil {
			fmt.Println("Ошибка загрузки бюджетов:", err)
		} else {
			fmt.Println("Бюджеты загружены из budgets.json")
		}
		f.Close()
	}
	printBudgets()

	fmt.Println()
	fmt.Println("Тест 1: транзакции в пределах лимита")
	add(Transaction{Amount: 1500, Category: "Еда", Description: "Продукты", Date: "2026-11-18"})
	add(Transaction{Amount: 2500, Category: "Еда", Description: "Ресторан", Date: "2026-11-19"})
	add(Transaction{Amount: 1200, Category: "Транспорт", Description: "Проездной", Date: "2026-11-19"})

	fmt.Println()
	fmt.Println("Тест 2: транзакция превышает лимит")
	add(Transaction{Amount: 1500, Category: "Еда", Description: "Доставка", Date: "2026-11-20"})
	add(Transaction{Amount: 1200, Category: "Развлечения", Description: "Концерт", Date: "2026-11-20"})

	fmt.Println()
	fmt.Println("Тест 3: ровно до лимита")
	add(Transaction{Amount: 1000, Category: "Еда", Description: "Кафе", Date: "2026-11-20"})

	fmt.Println()
	fmt.Println("Тест 4: категория без бюджета")
	add(Transaction{Amount: 10000, Category: "Здоровье", Description: "Стоматолог", Date: "2026-11-20"})

	fmt.Println()
	fmt.Println("Тест 5: сумма 0")
	add(Transaction{Amount: 0, Category: "Еда", Description: "Пустая", Date: "2026-11-20"})

	fmt.Println()
	fmt.Println("Тест 6: увеличили лимит и повторили")
	SetBudget(Budget{Category: "Развлечения", Limit: 2000, Period: "2026-11"})
	add(Transaction{Amount: 1200, Category: "Развлечения", Description: "Концерт", Date: "2026-11-20"})

	fmt.Println()
	printBudgets()
	fmt.Println()
	printTransactions()
}
