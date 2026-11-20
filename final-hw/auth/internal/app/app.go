package app

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/pressly/goose/v3"
	"golang.org/x/crypto/bcrypt"

	"auth/internal/db"
	"auth/internal/repository/pg"
	"auth/internal/service"
	"auth/internal/token"
	"auth/migrations"
)

const DefaultTokenTTL = 24 * time.Hour

type Config struct {
	DatabaseURL string
	JWTSecret   string
	TokenTTL    time.Duration
	GRPCAddr    string
	Migrate     bool
}

func ConfigFromEnv() (Config, error) {
	cfg := Config{
		DatabaseURL: db.DSNFromEnv(),
		JWTSecret:   os.Getenv("JWT_SECRET"),
		TokenTTL:    DefaultTokenTTL,
		GRPCAddr:    os.Getenv("AUTH_GRPC_ADDR"),
		Migrate:     os.Getenv("MIGRATE") != "false",
	}
	if cfg.GRPCAddr == "" {
		cfg.GRPCAddr = ":9091"
	}
	if cfg.JWTSecret == "" {
		return cfg, fmt.Errorf("не задан JWT_SECRET")
	}
	if v := os.Getenv("JWT_TTL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			return cfg, fmt.Errorf("неверный JWT_TTL=%q, нужен формат 24h, 30m", v)
		}
		cfg.TokenTTL = d
	}
	return cfg, nil
}

func Migrate(ctx context.Context, conn *sql.DB) error {
	goose.SetBaseFS(migrations.FS)
	goose.SetTableName("auth_goose_db_version")
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	if err := goose.UpContext(ctx, conn, "."); err != nil {
		return fmt.Errorf("миграции: %w", err)
	}
	return nil
}

func New(ctx context.Context, cfg Config) (service.Service, func() error, error) {
	issuer, err := token.NewJWTIssuer(cfg.JWTSecret, cfg.TokenTTL)
	if err != nil {
		return nil, nil, err
	}

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

	svc := service.New(pg.NewUserRepo(conn), issuer, bcrypt.DefaultCost)
	return svc, conn.Close, nil
}
