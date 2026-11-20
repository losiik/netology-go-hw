package main

import (
	"context"
	"log"
	"net/http"

	"gateway/internal/api"
	"ledger"
)

func main() {
	svc, closeFn, err := ledger.New(context.Background())
	if err != nil {
		log.Fatal("не удалось инициализировать ledger: ", err)
	}
	defer func() {
		if err := closeFn(); err != nil {
			log.Println("ошибка закрытия ledger:", err)
		}
	}()

	log.Println("Gateway started on :8080")
	if err := http.ListenAndServe(":8080", api.NewRouter(svc)); err != nil {
		log.Println(err)
	}
}
