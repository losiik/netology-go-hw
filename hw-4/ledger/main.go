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

func setBudget(b Budget) {
	if err := SetBudget(b); err != nil {
		fmt.Printf("Отказ: бюджет %q %.2f: %v\n", b.Category, b.Limit, err)
		return
	}
	fmt.Printf("Бюджет установлен: %s %.2f\n", b.Category, b.Limit)
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

	fmt.Println()
	fmt.Println("Проверка CheckValid для разных типов:")
	items := []Validatable{
		Transaction{Amount: 100, Category: "Еда", Date: "2026-11-20"},
		Transaction{Amount: -50, Category: "Еда", Date: "2026-11-20"},
		Transaction{Amount: 100, Category: "", Date: "2026-11-20"},
		Transaction{Amount: 100, Category: "Еда", Date: "20.11.2026"},
		Budget{Category: "Еда", Limit: 5000},
		Budget{Category: "Еда", Limit: 0},
		Budget{Category: " ", Limit: 1000},
	}
	for _, v := range items {
		if err := CheckValid(v); err != nil {
			fmt.Printf("  %T %+v -> ошибка: %v\n", v, v, err)
		} else {
			fmt.Printf("  %T %+v -> ok\n", v, v)
		}
	}

	fmt.Println()
	setBudget(Budget{Category: "Еда", Limit: 5000, Period: "2026-11"})
	setBudget(Budget{Category: "Подарки", Limit: -100, Period: "2026-11"})

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
	fmt.Println("Тест 5: невалидные транзакции")
	add(Transaction{Amount: 0, Category: "Еда", Description: "Пустая", Date: "2026-11-20"})
	add(Transaction{Amount: -300, Category: "Транспорт", Description: "Возврат", Date: "2026-11-20"})
	add(Transaction{Amount: 200, Category: "", Description: "Без категории", Date: "2026-11-20"})
	add(Transaction{Amount: 200, Category: "Транспорт", Description: "Такси", Date: "вчера"})

	fmt.Println()
	fmt.Println("Тест 6: увеличили лимит и повторили")
	setBudget(Budget{Category: "Развлечения", Limit: 2000, Period: "2026-11"})
	add(Transaction{Amount: 1200, Category: "Развлечения", Description: "Концерт", Date: "2026-11-20"})

	fmt.Println()
	printBudgets()
	fmt.Println()
	printTransactions()
}
