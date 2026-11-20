package app

import (
	"context"
	"errors"
	"log"
	"os"

	"ledger/internal/cache"
	"ledger/internal/db"
	"ledger/internal/repository/memory"
	"ledger/internal/repository/pg"
	"ledger/internal/service"
)

type Config struct {
	DatabaseURL string
	Redis       cache.Config
	GRPCAddr    string
	Migrate     bool
}

func ConfigFromEnv() (Config, error) {
	rc, err := cache.ConfigFromEnv()
	if err != nil {
		return Config{}, err
	}
	addr := os.Getenv("LEDGER_GRPC_ADDR")
	if addr == "" {
		addr = ":9090"
	}
	return Config{
		DatabaseURL: db.DSNFromEnv(),
		Redis:       rc,
		GRPCAddr:    addr,
		Migrate:     os.Getenv("MIGRATE") != "false",
	}, nil
}

func New(ctx context.Context, cfg Config) (service.Service, func() error, error) {
	conn, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, nil, err
	}
	log.Printf("подключено к PostgreSQL: %s", db.SafeDSN(cfg.DatabaseURL))

	if cfg.Migrate {
		if err := Migrate(ctx, conn); err != nil {
			conn.Close()
			return nil, nil, err
		}
	}

	rdb, err := cache.New(ctx, cfg.Redis)
	if err != nil {
		conn.Close()
		return nil, nil, err
	}
	log.Printf("подключено к Redis: %s, db=%d", cfg.Redis.Addr, cfg.Redis.DB)

	svc := service.New(
		pg.NewBudgetRepo(conn),
		pg.NewExpenseRepo(conn),
		cache.NewRedisCache(rdb),
	)

	closeFn := func() error {
		return errors.Join(rdb.Close(), conn.Close())
	}
	return svc, closeFn, nil
}

func NewInMemory() service.Service {
	return service.New(memory.NewBudgetRepo(), memory.NewExpenseRepo(), nil)
}
