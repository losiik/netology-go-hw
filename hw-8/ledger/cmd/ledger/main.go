package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"ledger/internal/app"
)

func main() {
	load := flag.String("load", "", "загрузить бюджеты из JSON-файла, например -load budgets.json")
	flag.Parse()

	ctx := context.Background()

	cfg, err := app.ConfigFromEnv()
	if err != nil {
		log.Fatal("ошибка конфигурации: ", err)
	}
	svc, closeFn, err := app.New(ctx, cfg)
	if err != nil {
		log.Fatal("не удалось запустить ledger: ", err)
	}
	defer closeFn()

	log.Println("Ledger service started")

	if *load != "" {
		f, err := os.Open(*load)
		if err != nil {
			log.Fatal("ошибка открытия файла: ", err)
		}
		err = svc.LoadBudgets(ctx, bufio.NewReader(f))
		f.Close()
		if err != nil {
			log.Fatal("ошибка загрузки бюджетов: ", err)
		}
		log.Printf("бюджеты загружены из %s", *load)
	}

	budgets, err := svc.ListBudgets(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Бюджеты в БД: %d\n", len(budgets))
	for _, b := range budgets {
		fmt.Printf("  %-12s лимит %10.2f\n", b.Category, b.Limit)
	}

	txs, err := svc.ListTransactions(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Транзакции в БД: %d\n", len(txs))
	for _, tx := range txs {
		fmt.Printf("  %d | %s | %-12s | %10.2f | %s\n", tx.ID, tx.Date, tx.Category, tx.Amount, tx.Description)
	}
}
