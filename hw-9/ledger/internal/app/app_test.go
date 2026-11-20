package app

import (
	"context"
	"io"
	"log"
	"os"
	"testing"

	"ledger/internal/cache"
	"ledger/internal/domain"
)

func TestMain(m *testing.M) {
	log.SetOutput(io.Discard)
	os.Exit(m.Run())
}

func TestNewBadDatabase(t *testing.T) {
	cfg := Config{
		DatabaseURL: "postgres://postgres:postgres@127.0.0.1:1/cashapp?sslmode=disable",
		Redis:       cache.Config{Addr: "127.0.0.1:1"},
	}
	svc, closeFn, err := New(context.Background(), cfg)
	if err == nil {
		closeFn()
		t.Fatal("ожидалась ошибка подключения к БД")
	}
	if svc != nil || closeFn != nil {
		t.Error("при ошибке сервис и closeFn должны быть nil")
	}
}

func TestNewCanceledContext(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL не задан")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := New(ctx, Config{DatabaseURL: url, Redis: cache.Config{Addr: "localhost:6379"}}); err == nil {
		t.Error("с отменённым контекстом Ping должен вернуть ошибку")
	}
}

func TestNewBadRedis(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL не задан")
	}
	_, _, err := New(context.Background(), Config{DatabaseURL: url, Redis: cache.Config{Addr: "127.0.0.1:1"}})
	if err == nil {
		t.Fatal("ожидалась ошибка подключения к Redis")
	}
}

func TestNew(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL не задан")
	}
	svc, closeFn, err := New(context.Background(), Config{DatabaseURL: url, Redis: cache.Config{Addr: "localhost:6379", DB: 15}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := closeFn(); err != nil {
			t.Errorf("closeFn: %v", err)
		}
	}()

	if _, err := svc.ListTransactions(context.Background()); err != nil {
		t.Errorf("ListTransactions: %v", err)
	}
}

func TestNewInMemory(t *testing.T) {
	ctx := context.Background()
	svc := NewInMemory()
	if err := svc.SetBudget(ctx, domain.Budget{Category: "еда", Limit: 10}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddTransaction(ctx, domain.Transaction{Amount: 11, Category: "еда", Date: "2025-09-10"}); err != domain.ErrBudgetExceeded {
		t.Errorf("ожидали ErrBudgetExceeded, получили %v", err)
	}
}
