package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"ledger"
)

func main() {
	load := flag.String("load", "", "загрузить бюджеты из JSON-файла, например -load budgets.json")
	flag.Parse()

	cfg, err := ledger.ConfigFromEnv()
	if err != nil {
		log.Fatal("ошибка конфигурации: ", err)
	}
	if err := ledger.Init(context.Background(), cfg); err != nil {
		log.Fatal("не удалось запустить ledger: ", err)
	}
	defer ledger.Close()

	log.Println("Ledger service started")

	if *load != "" {
		f, err := os.Open(*load)
		if err != nil {
			log.Fatal("ошибка открытия файла: ", err)
		}
		err = ledger.LoadBudgets(bufio.NewReader(f))
		f.Close()
		if err != nil {
			log.Fatal("ошибка загрузки бюджетов: ", err)
		}
		log.Printf("бюджеты загружены из %s", *load)
	}

	budgets, err := ledger.ListBudgets()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Бюджеты в БД: %d\n", len(budgets))
	for _, b := range budgets {
		spent, err := ledger.CategoryTotal(b.Category)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("  %-12s лимит %10.2f, потрачено %10.2f\n", b.Category, b.Limit, spent)
	}

	txs, err := ledger.ListTransactions()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Транзакции в БД: %d\n", len(txs))
	for _, tx := range txs {
		fmt.Printf("  %d | %s | %-12s | %10.2f | %s\n", tx.ID, tx.Date, tx.Category, tx.Amount, tx.Description)
	}
}
