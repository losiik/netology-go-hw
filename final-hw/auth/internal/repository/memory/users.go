package memory

import (
	"context"
	"sync"
	"time"

	"auth/internal/domain"
)

type UserRepo struct {
	mu    sync.Mutex
	users map[string]domain.User
}

func NewUserRepo() *UserRepo {
	return &UserRepo{users: map[string]domain.User{}}
}

func (r *UserRepo) Create(_ context.Context, u domain.User) (domain.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.users[u.Login]; ok {
		return domain.User{}, domain.ErrUserExists
	}
	u.ID = int64(len(r.users) + 1)
	u.CreatedAt = time.Now()
	r.users[u.Login] = u
	return u, nil
}

func (r *UserRepo) GetByLogin(_ context.Context, login string) (domain.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	u, ok := r.users[login]
	if !ok {
		return domain.User{}, domain.ErrNotFound
	}
	return u, nil
}

var _ domain.UserRepository = (*UserRepo)(nil)
