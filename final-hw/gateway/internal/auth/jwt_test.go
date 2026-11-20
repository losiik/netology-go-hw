package auth

import (
	"context"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

const secret = "test-secret-test-secret-test-secret"

func sign(t *testing.T, method jwt.SigningMethod, key any, claims Claims) string {
	t.Helper()
	s, err := jwt.NewWithClaims(method, claims).SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestVerify(t *testing.T) {
	v, err := NewVerifier(secret)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2025, 9, 10, 12, 0, 0, 0, time.UTC)
	v.now = func() time.Time { return now }

	valid := Claims{Login: "ivan", RegisteredClaims: jwt.RegisteredClaims{
		Subject: "1", Issuer: Issuer, ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
	}}
	expired := valid
	expired.ExpiresAt = jwt.NewNumericDate(now.Add(-time.Second))
	noExp := valid
	noExp.ExpiresAt = nil
	otherIssuer := valid
	otherIssuer.Issuer = "someone-else"
	noSubject := valid
	noSubject.Subject = ""

	tests := []struct {
		name  string
		token string
		ok    bool
	}{
		{"valid", sign(t, jwt.SigningMethodHS256, []byte(secret), valid), true},
		{"expired", sign(t, jwt.SigningMethodHS256, []byte(secret), expired), false},
		{"no_exp", sign(t, jwt.SigningMethodHS256, []byte(secret), noExp), false},
		{"other_issuer", sign(t, jwt.SigningMethodHS256, []byte(secret), otherIssuer), false},
		{"wrong_secret", sign(t, jwt.SigningMethodHS256, []byte("another-secret-another-secret-123"), valid), false},
		{"hs512", sign(t, jwt.SigningMethodHS512, []byte(secret), valid), false},
		{"none", sign(t, jwt.SigningMethodNone, jwt.UnsafeAllowNoneSignatureType, valid), false},
		{"no_subject", sign(t, jwt.SigningMethodHS256, []byte(secret), noSubject), false},
		{"garbage", "abc", false},
		{"empty", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			claims, err := v.Verify(tc.token)
			if tc.ok {
				if err != nil || claims.Login != "ivan" || claims.Subject != "1" {
					t.Errorf("Verify = %+v, %v", claims, err)
				}
				return
			}
			if err != ErrInvalidToken {
				t.Errorf("ожидали ErrInvalidToken, получили %v", err)
			}
		})
	}
}

func TestNewVerifier(t *testing.T) {
	if _, err := NewVerifier("short"); err == nil {
		t.Error("короткий секрет должен давать ошибку")
	}
}

func TestContext(t *testing.T) {
	ctx := WithClaims(context.Background(), Claims{Login: "ivan"})
	c, ok := FromContext(ctx)
	if !ok || c.Login != "ivan" {
		t.Errorf("FromContext = %+v, %v", c, ok)
	}
	if _, ok := FromContext(context.Background()); ok {
		t.Error("в пустом контексте не должно быть claims")
	}
}

func TestUserMetadataInterceptor(t *testing.T) {
	var got []string
	invoker := func(ctx context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		md, _ := metadata.FromOutgoingContext(ctx)
		got = md.Get(UserIDHeader)
		return nil
	}

	ctx := WithClaims(context.Background(), Claims{RegisteredClaims: jwt.RegisteredClaims{Subject: "42"}})
	UserMetadataInterceptor(ctx, "/m", nil, nil, nil, invoker)
	if len(got) != 1 || got[0] != "42" {
		t.Errorf("x-user-id = %v", got)
	}

	UserMetadataInterceptor(context.Background(), "/m", nil, nil, nil, invoker)
	if len(got) != 0 {
		t.Errorf("без токена x-user-id не передаётся, получили %v", got)
	}
}
