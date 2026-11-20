package app

import (
	"context"
	"io"
	"log"
	"os"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	log.SetOutput(io.Discard)
	os.Exit(m.Run())
}

func TestConfigFromEnv(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		ttl     time.Duration
		addr    string
		migrate bool
		wantErr bool
	}{
		{"defaults", map[string]string{"JWT_SECRET": "s"}, DefaultTokenTTL, ":9091", true, false},
		{"custom", map[string]string{"JWT_SECRET": "s", "JWT_TTL": "30m", "AUTH_GRPC_ADDR": ":7000", "MIGRATE": "false"}, 30 * time.Minute, ":7000", false, false},
		{"no_secret", map[string]string{}, 0, "", false, true},
		{"bad_ttl", map[string]string{"JWT_SECRET": "s", "JWT_TTL": "сутки"}, 0, "", false, true},
		{"negative_ttl", map[string]string{"JWT_SECRET": "s", "JWT_TTL": "-1h"}, 0, "", false, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for _, k := range []string{"JWT_SECRET", "JWT_TTL", "AUTH_GRPC_ADDR", "MIGRATE"} {
				t.Setenv(k, tc.env[k])
			}
			cfg, err := ConfigFromEnv()
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if tc.wantErr {
				return
			}
			if cfg.TokenTTL != tc.ttl || cfg.GRPCAddr != tc.addr || cfg.Migrate != tc.migrate {
				t.Errorf("cfg = %+v", cfg)
			}
		})
	}
}

func TestNewErrors(t *testing.T) {
	ctx := context.Background()
	secret := "test-secret-test-secret-test-secret"

	if _, _, err := New(ctx, Config{JWTSecret: "short", TokenTTL: time.Hour}); err == nil {
		t.Error("короткий секрет должен давать ошибку")
	}
	_, _, err := New(ctx, Config{JWTSecret: secret, TokenTTL: time.Hour, DatabaseURL: "postgres://postgres:postgres@127.0.0.1:1/x?sslmode=disable"})
	if err == nil {
		t.Error("ожидалась ошибка подключения к БД")
	}
}

func TestNew(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL не задан")
	}
	ctx := context.Background()
	svc, closeFn, err := New(ctx, Config{
		DatabaseURL: url,
		JWTSecret:   "test-secret-test-secret-test-secret",
		TokenTTL:    time.Hour,
		Migrate:     true,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer closeFn()

	login := "app_test_" + time.Now().Format("150405.000000")
	if _, err := svc.Register(ctx, login, "password123"); err != nil {
		t.Fatal(err)
	}
	if token, _, err := svc.Login(ctx, login, "password123"); err != nil || token == "" {
		t.Errorf("Login: %q, %v", token, err)
	}
}
