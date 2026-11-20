package service

import (
	"context"
	"errors"
	"io"
	"log"
	"os"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"auth/internal/domain"
	"auth/internal/repository/memory"
	"auth/internal/token"
)

func TestMain(m *testing.M) {
	log.SetOutput(io.Discard)
	os.Exit(m.Run())
}

func newService(t *testing.T) Service {
	t.Helper()
	issuer, err := token.NewJWTIssuer("test-secret-test-secret-test-secret", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return New(memory.NewUserRepo(), issuer, bcrypt.MinCost)
}

func TestRegister(t *testing.T) {
	ctx := context.Background()
	svc := newService(t)

	u, err := svc.Register(ctx, "  Ivan ", "password123")
	if err != nil {
		t.Fatal(err)
	}
	if u.ID != 1 || u.Login != "ivan" || u.PasswordHash == "password123" || u.PasswordHash == "" {
		t.Errorf("пользователь %+v", u)
	}

	tests := []struct {
		name     string
		login    string
		password string
		want     error
	}{
		{"duplicate", "ivan", "password123", domain.ErrUserExists},
		{"duplicate_other_case", "IVAN", "other-password", domain.ErrUserExists},
		{"short_password", "petr", "123", domain.ErrValidation},
		{"bad_login", "п", "password123", domain.ErrValidation},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := svc.Register(ctx, tc.login, tc.password); !errors.Is(err, tc.want) {
				t.Errorf("err = %v, ожидали %v", err, tc.want)
			}
		})
	}
}

func TestLogin(t *testing.T) {
	ctx := context.Background()
	svc := newService(t)
	if _, err := svc.Register(ctx, "ivan", "password123"); err != nil {
		t.Fatal(err)
	}

	token, exp, err := svc.Login(ctx, "IVAN", "password123")
	if err != nil {
		t.Fatal(err)
	}
	if token == "" || time.Until(exp) < 59*time.Minute {
		t.Errorf("token=%q exp=%v", token, exp)
	}

	tests := []struct {
		name     string
		login    string
		password string
		want     error
	}{
		{"wrong_password", "ivan", "wrong-password", domain.ErrInvalidCredentials},
		{"unknown_user", "nobody", "password123", domain.ErrInvalidCredentials},
		{"empty_login", "", "password123", domain.ErrValidation},
		{"empty_password", "ivan", "", domain.ErrValidation},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, err := svc.Login(ctx, tc.login, tc.password); !errors.Is(err, tc.want) {
				t.Errorf("err = %v, ожидали %v", err, tc.want)
			}
		})
	}
}

type brokenRepo struct{}

var errDB = errors.New("db is down")

func (brokenRepo) Create(context.Context, domain.User) (domain.User, error) {
	return domain.User{}, errDB
}
func (brokenRepo) GetByLogin(context.Context, string) (domain.User, error) {
	return domain.User{}, errDB
}

func TestRepoErrors(t *testing.T) {
	issuer, _ := token.NewJWTIssuer("test-secret-test-secret-test-secret", time.Hour)
	svc := New(brokenRepo{}, issuer, bcrypt.MinCost)
	if _, err := svc.Register(context.Background(), "ivan", "password123"); !errors.Is(err, errDB) {
		t.Errorf("Register: %v", err)
	}
	if _, _, err := svc.Login(context.Background(), "ivan", "password123"); !errors.Is(err, errDB) {
		t.Errorf("Login: %v", err)
	}
}
