package config

import (
	"errors"
	"fmt"
	"os"
	"time"
)

const DefaultRequestTimeout = 2 * time.Second

type Config struct {
	HTTPAddr       string
	LedgerAddr     string
	AuthAddr       string
	JWTSecret      string
	RequestTimeout time.Duration
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func FromEnv() (Config, error) {
	cfg := Config{
		HTTPAddr:       getenv("HTTP_ADDR", ":8080"),
		LedgerAddr:     getenv("LEDGER_ADDR", "localhost:9090"),
		AuthAddr:       getenv("AUTH_ADDR", "localhost:9091"),
		JWTSecret:      os.Getenv("JWT_SECRET"),
		RequestTimeout: DefaultRequestTimeout,
	}
	if cfg.JWTSecret == "" {
		return cfg, errors.New("не задан JWT_SECRET")
	}
	if v := os.Getenv("REQUEST_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return cfg, fmt.Errorf("неверный REQUEST_TIMEOUT=%q: %w", v, err)
		}
		if d <= 0 {
			return cfg, fmt.Errorf("REQUEST_TIMEOUT должен быть больше 0, получено %s", d)
		}
		cfg.RequestTimeout = d
	}
	return cfg, nil
}
