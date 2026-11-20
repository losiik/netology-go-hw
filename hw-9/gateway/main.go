package main

import (
	"context"
	"log"
	"net/http"

	"gateway/internal/api"
	"ledger"
)

func main() {
	timeout, err := api.TimeoutFromEnv()
	if err != nil {
		log.Fatal(err)
	}

	svc, closeFn, err := ledger.New(context.Background())
	if err != nil {
		log.Fatal("не удалось инициализировать ledger: ", err)
	}
	defer func() {
		if err := closeFn(); err != nil {
			log.Println("ошибка закрытия ledger:", err)
		}
	}()

	log.Printf("Gateway started on :8080, таймаут запросов %s", timeout)
	if err := http.ListenAndServe(":8080", api.NewRouter(svc, timeout)); err != nil {
		log.Println(err)
	}
}
