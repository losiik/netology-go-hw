package domain

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

var (
	ErrUserExists         = errors.New("пользователь с таким логином уже существует")
	ErrNotFound           = errors.New("не найдено")
	ErrInvalidCredentials = errors.New("неверный логин или пароль")
	ErrValidation         = errors.New("validation error")
)

type ValidationError struct {
	Err error
}

func NewValidationError(err error) error {
	return &ValidationError{Err: err}
}

func (e *ValidationError) Error() string {
	return e.Err.Error()
}

func (e *ValidationError) Unwrap() []error {
	return []error{ErrValidation, e.Err}
}

type User struct {
	ID           int64
	Login        string
	PasswordHash string
	CreatedAt    time.Time
}

var loginRe = regexp.MustCompile(`^[a-z0-9._@-]+$`)

func NormalizeLogin(login string) string {
	return strings.ToLower(strings.TrimSpace(login))
}

func ValidateCredentials(login, password string) error {
	if len(login) < 3 || len(login) > 64 {
		return fmt.Errorf("логин должен быть от 3 до 64 символов, получено %d", len(login))
	}
	if !loginRe.MatchString(login) {
		return errors.New("логин может содержать только латинские буквы, цифры и символы . _ @ -")
	}
	if len(password) < 8 {
		return errors.New("пароль должен быть не короче 8 символов")
	}
	if len(password) > 72 {
		return errors.New("пароль должен быть не длиннее 72 байт")
	}
	return nil
}
