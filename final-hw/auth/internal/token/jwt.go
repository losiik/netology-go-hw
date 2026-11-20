package token

import (
	"errors"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"auth/internal/domain"
)

const Issuer = "cashcraft-auth"

type Claims struct {
	Login string `json:"login"`
	jwt.RegisteredClaims
}

type JWTIssuer struct {
	secret []byte
	ttl    time.Duration
	now    func() time.Time
}

func NewJWTIssuer(secret string, ttl time.Duration) (*JWTIssuer, error) {
	if len(secret) < 32 {
		return nil, errors.New("JWT_SECRET должен быть не короче 32 символов")
	}
	if ttl <= 0 {
		return nil, errors.New("время жизни токена должно быть больше 0")
	}
	return &JWTIssuer{secret: []byte(secret), ttl: ttl, now: time.Now}, nil
}

func (i *JWTIssuer) Issue(u domain.User) (string, time.Time, error) {
	now := i.now()
	exp := now.Add(i.ttl)
	claims := Claims{
		Login: u.Login,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   strconv.FormatInt(u.ID, 10),
			Issuer:    Issuer,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
		},
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(i.secret)
	if err != nil {
		return "", time.Time{}, err
	}
	return signed, exp, nil
}

var _ domain.TokenIssuer = (*JWTIssuer)(nil)
