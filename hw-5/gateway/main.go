package main

import (
	"log"
	"net/http"

	"gateway/internal/api"
)

func main() {
	log.Println("Gateway started on :8080")
	if err := http.ListenAndServe(":8080", api.NewRouter()); err != nil {
		log.Fatal(err)
	}
}
