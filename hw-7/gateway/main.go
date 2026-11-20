package main

import (
	"context"
	"log"
	"net/http"

	"gateway/internal/api"
	"ledger"
)

func main() {
	cfg, err := ledger.ConfigFromEnv()
	if err != nil {
		log.Fatal("ошибка конфигурации: ", err)
	}
	if err := ledger.Init(context.Background(), cfg); err != nil {
		log.Fatal("не удалось подключить ledger: ", err)
	}
	defer ledger.Close()

	log.Println("Gateway started on :8080")
	if err := http.ListenAndServe(":8080", api.NewRouter()); err != nil {
		log.Fatal(err)
	}
}
