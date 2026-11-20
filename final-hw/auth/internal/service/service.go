package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"golang.org/x/crypto/bcrypt"

	"auth/internal/domain"
)

type Service interface {
	Register(ctx context.Context, login, password string) (domain.User, error)
	Login(ctx context.Context, login, password string) (string, time.Time, error)
}

type authService struct {
	users     domain.UserRepository
	tokens    domain.TokenIssuer
	cost      int
	dummyHash []byte
}

func New(users domain.UserRepository, tokens domain.TokenIssuer, cost int) Service {
	dummy, _ := bcrypt.GenerateFromPassword([]byte("dummy-password"), cost)
	return &authService{users: users, tokens: tokens, cost: cost, dummyHash: dummy}
}

func (s *authService) Register(ctx context.Context, login, password string) (domain.User, error) {
	login = domain.NormalizeLogin(login)
	if err := domain.ValidateCredentials(login, password); err != nil {
		return domain.User{}, domain.NewValidationError(err)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), s.cost)
	if err != nil {
		return domain.User{}, fmt.Errorf("хеширование пароля: %w", err)
	}

	u, err := s.users.Create(ctx, domain.User{Login: login, PasswordHash: string(hash)})
	if err != nil {
		return domain.User{}, err
	}
	log.Printf("зарегистрирован пользователь %s (id=%d)", u.Login, u.ID)
	return u, nil
}

func (s *authService) Login(ctx context.Context, login, password string) (string, time.Time, error) {
	login = domain.NormalizeLogin(login)
	if login == "" || password == "" {
		return "", time.Time{}, domain.NewValidationError(errors.New("логин и пароль обязательны"))
	}

	u, err := s.users.GetByLogin(ctx, login)
	if errors.Is(err, domain.ErrNotFound) {
		bcrypt.CompareHashAndPassword(s.dummyHash, []byte(password))
		return "", time.Time{}, domain.ErrInvalidCredentials
	}
	if err != nil {
		return "", time.Time{}, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		return "", time.Time{}, domain.ErrInvalidCredentials
	}

	token, exp, err := s.tokens.Issue(u)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("выпуск токена: %w", err)
	}
	log.Printf("вход пользователя %s", u.Login)
	return token, exp, nil
}
