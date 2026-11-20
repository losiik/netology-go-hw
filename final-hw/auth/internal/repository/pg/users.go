package pg

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"

	"auth/internal/domain"
)

type UserRepo struct {
	db *sql.DB
}

func NewUserRepo(db *sql.DB) *UserRepo {
	return &UserRepo{db: db}
}

func (r *UserRepo) Create(ctx context.Context, u domain.User) (domain.User, error) {
	err := r.db.QueryRowContext(ctx,
		"INSERT INTO users (login, password_hash) VALUES ($1, $2) RETURNING id, created_at",
		u.Login, u.PasswordHash).Scan(&u.ID, &u.CreatedAt)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return domain.User{}, domain.ErrUserExists
	}
	if err != nil {
		return domain.User{}, fmt.Errorf("создание пользователя: %w", err)
	}
	return u, nil
}

func (r *UserRepo) GetByLogin(ctx context.Context, login string) (domain.User, error) {
	var u domain.User
	err := r.db.QueryRowContext(ctx,
		"SELECT id, login, password_hash, created_at FROM users WHERE login = $1", login).
		Scan(&u.ID, &u.Login, &u.PasswordHash, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.User{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.User{}, fmt.Errorf("чтение пользователя: %w", err)
	}
	return u, nil
}

var _ domain.UserRepository = (*UserRepo)(nil)
