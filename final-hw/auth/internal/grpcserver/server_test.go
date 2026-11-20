package grpcserver

import (
	"context"
	"io"
	"log"
	"net"
	"os"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	"auth/internal/repository/memory"
	"auth/internal/service"
	"auth/internal/token"
	"proto/authv1"
)

func TestMain(m *testing.M) {
	log.SetOutput(io.Discard)
	os.Exit(m.Run())
}

func newClient(t *testing.T) authv1.AuthServiceClient {
	t.Helper()
	issuer, err := token.NewJWTIssuer("test-secret-test-secret-test-secret", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer(grpc.UnaryInterceptor(LoggingInterceptor))
	authv1.RegisterAuthServiceServer(srv, New(service.New(memory.NewUserRepo(), issuer, bcrypt.MinCost)))
	go srv.Serve(lis)
	t.Cleanup(srv.Stop)

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return authv1.NewAuthServiceClient(conn)
}

func TestRegisterAndLogin(t *testing.T) {
	ctx := context.Background()
	c := newClient(t)

	reg, err := c.Register(ctx, &authv1.RegisterRequest{Login: "ivan", Password: "password123"})
	if err != nil {
		t.Fatal(err)
	}
	if reg.GetId() != 1 || reg.GetLogin() != "ivan" {
		t.Errorf("Register = %v", reg)
	}

	login, err := c.Login(ctx, &authv1.LoginRequest{Login: "ivan", Password: "password123"})
	if err != nil {
		t.Fatal(err)
	}
	if login.GetToken() == "" || login.GetExpiresAt() <= time.Now().Unix() {
		t.Errorf("Login = %v", login)
	}

	tests := []struct {
		name string
		call func() error
		code codes.Code
	}{
		{"duplicate", func() error {
			_, err := c.Register(ctx, &authv1.RegisterRequest{Login: "ivan", Password: "password123"})
			return err
		}, codes.AlreadyExists},
		{"bad_password", func() error {
			_, err := c.Register(ctx, &authv1.RegisterRequest{Login: "petr", Password: "1"})
			return err
		}, codes.InvalidArgument},
		{"wrong_password", func() error {
			_, err := c.Login(ctx, &authv1.LoginRequest{Login: "ivan", Password: "wrong-password"})
			return err
		}, codes.Unauthenticated},
		{"empty_login", func() error {
			_, err := c.Login(ctx, &authv1.LoginRequest{})
			return err
		}, codes.InvalidArgument},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if code := status.Code(tc.call()); code != tc.code {
				t.Errorf("код %s, ожидали %s", code, tc.code)
			}
		})
	}
}
