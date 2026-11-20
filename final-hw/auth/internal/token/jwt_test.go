package token

import (
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"auth/internal/domain"
)

const secret = "test-secret-test-secret-test-secret"

func TestIssue(t *testing.T) {
	issuer, err := NewJWTIssuer(secret, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2025, 9, 10, 12, 0, 0, 0, time.UTC)
	issuer.now = func() time.Time { return now }

	signed, exp, err := issuer.Issue(domain.User{ID: 42, Login: "ivan"})
	if err != nil {
		t.Fatal(err)
	}
	if !exp.Equal(now.Add(time.Hour)) {
		t.Errorf("exp = %v", exp)
	}

	var claims Claims
	_, err = jwt.ParseWithClaims(signed, &claims, func(*jwt.Token) (any, error) { return []byte(secret), nil },
		jwt.WithValidMethods([]string{"HS256"}), jwt.WithTimeFunc(func() time.Time { return now }))
	if err != nil {
		t.Fatal(err)
	}
	if claims.Subject != "42" || claims.Login != "ivan" || claims.Issuer != Issuer {
		t.Errorf("claims %+v", claims)
	}

	_, err = jwt.ParseWithClaims(signed, &Claims{}, func(*jwt.Token) (any, error) { return []byte("другой-секрет"), nil })
	if err == nil {
		t.Error("токен не должен проходить проверку с другим секретом")
	}
}

func TestNewJWTIssuer(t *testing.T) {
	tests := []struct {
		name   string
		secret string
		ttl    time.Duration
		err    string
	}{
		{"ok", secret, time.Hour, ""},
		{"short_secret", "short", time.Hour, "не короче 32"},
		{"zero_ttl", secret, 0, "больше 0"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewJWTIssuer(tc.secret, tc.ttl)
			if tc.err == "" && err != nil || tc.err != "" && (err == nil || !strings.Contains(err.Error(), tc.err)) {
				t.Errorf("err = %v, ожидали %q", err, tc.err)
			}
		})
	}
}
