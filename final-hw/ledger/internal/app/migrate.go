package app

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/pressly/goose/v3"

	"ledger/migrations"
)

func Migrate(ctx context.Context, conn *sql.DB) error {
	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	if err := goose.UpContext(ctx, conn, "."); err != nil {
		return fmt.Errorf("миграции: %w", err)
	}
	return nil
}
