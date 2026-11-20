package auth

import (
	"context"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

const Issuer = "cashcraft-auth"

var ErrInvalidToken = errors.New("неверный или просроченный токен")

type Claims struct {
	Login string `json:"login"`
	jwt.RegisteredClaims
}

type Verifier struct {
	secret []byte
	now    func() time.Time
}

func NewVerifier(secret string) (*Verifier, error) {
	if len(secret) < 32 {
		return nil, errors.New("JWT_SECRET должен быть не короче 32 символов")
	}
	return &Verifier{secret: []byte(secret), now: time.Now}, nil
}

func (v *Verifier) Verify(token string) (Claims, error) {
	var claims Claims
	_, err := jwt.ParseWithClaims(token, &claims,
		func(*jwt.Token) (any, error) { return v.secret, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(Issuer),
		jwt.WithExpirationRequired(),
		jwt.WithTimeFunc(v.now),
	)
	if err != nil || claims.Subject == "" {
		return Claims{}, ErrInvalidToken
	}
	return claims, nil
}

type ctxKey struct{}

func WithClaims(ctx context.Context, c Claims) context.Context {
	return context.WithValue(ctx, ctxKey{}, c)
}

func FromContext(ctx context.Context) (Claims, bool) {
	c, ok := ctx.Value(ctxKey{}).(Claims)
	return c, ok
}

const UserIDHeader = "x-user-id"

func UserMetadataInterceptor(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
	if c, ok := FromContext(ctx); ok {
		ctx = metadata.AppendToOutgoingContext(ctx, UserIDHeader, c.Subject)
	}
	return invoker(ctx, method, req, reply, cc, opts...)
}
