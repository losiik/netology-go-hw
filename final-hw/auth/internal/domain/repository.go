package domain

import (
	"context"
	"time"
)

type UserRepository interface {
	Create(ctx context.Context, u User) (User, error)
	GetByLogin(ctx context.Context, login string) (User, error)
}

type TokenIssuer interface {
	Issue(u User) (string, time.Time, error)
}
