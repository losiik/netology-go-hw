package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateCredentials(t *testing.T) {
	tests := []struct {
		name     string
		login    string
		password string
		wantErr  string
	}{
		{"ok", "ivan", "password123", ""},
		{"email_login", "ivan.petrov@mail.ru", "password123", ""},
		{"short_login", "iv", "password123", "от 3 до 64"},
		{"long_login", strings.Repeat("a", 65), "password123", "от 3 до 64"},
		{"bad_chars", "иван", "password123", "латинские"},
		{"space_in_login", "iv an", "password123", "латинские"},
		{"short_password", "ivan", "1234567", "не короче 8"},
		{"long_password", "ivan", strings.Repeat("p", 73), "не длиннее 72"},
		{"min_password", "ivan", "12345678", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateCredentials(tc.login, tc.password)
			if tc.wantErr == "" {
				if err != nil {
					t.Errorf("неожиданная ошибка: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("ошибка %v, ожидали содержит %q", err, tc.wantErr)
			}
		})
	}
}

func TestNormalizeLogin(t *testing.T) {
	tests := map[string]string{
		"Ivan":         "ivan",
		"  ivan  ":     "ivan",
		"IVAN@Mail.RU": "ivan@mail.ru",
	}
	for in, want := range tests {
		if got := NormalizeLogin(in); got != want {
			t.Errorf("NormalizeLogin(%q) = %q, ожидали %q", in, got, want)
		}
	}
}

func TestValidationError(t *testing.T) {
	inner := errors.New("плохой логин")
	err := NewValidationError(inner)
	if !errors.Is(err, ErrValidation) || !errors.Is(err, inner) || err.Error() != "плохой логин" {
		t.Errorf("ValidationError работает неверно: %v", err)
	}
}
