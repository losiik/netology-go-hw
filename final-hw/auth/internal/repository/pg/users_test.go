package pg

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/pressly/goose/v3"

	"auth/internal/db"
	"auth/internal/domain"
	"auth/migrations"
)

func TestUserRepo(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL не задан, тест с БД пропущен")
	}
	ctx := context.Background()
	conn, err := db.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	goose.SetBaseFS(migrations.FS)
	goose.SetTableName("auth_goose_db_version")
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatal(err)
	}
	if err := goose.UpContext(ctx, conn, "."); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(ctx, "TRUNCATE users RESTART IDENTITY"); err != nil {
		t.Fatal(err)
	}

	repo := NewUserRepo(conn)
	u, err := repo.Create(ctx, domain.User{Login: "ivan", PasswordHash: "hash"})
	if err != nil {
		t.Fatal(err)
	}
	if u.ID != 1 || u.CreatedAt.IsZero() {
		t.Errorf("Create = %+v", u)
	}

	if _, err := repo.Create(ctx, domain.User{Login: "ivan", PasswordHash: "other"}); !errors.Is(err, domain.ErrUserExists) {
		t.Errorf("дубль: %v", err)
	}

	got, err := repo.GetByLogin(ctx, "ivan")
	if err != nil || got.ID != 1 || got.PasswordHash != "hash" {
		t.Errorf("GetByLogin = %+v, %v", got, err)
	}
	if _, err := repo.GetByLogin(ctx, "nobody"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("неизвестный логин: %v", err)
	}
}
