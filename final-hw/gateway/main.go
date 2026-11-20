package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"gateway/internal/api"
	"gateway/internal/auth"
	"gateway/internal/config"
	"proto/authv1"
	"proto/ledgerv1"
)

func dial(addr string) *grpc.ClientConn {
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(32<<20), grpc.MaxCallSendMsgSize(16<<20)),
		grpc.WithUnaryInterceptor(auth.UserMetadataInterceptor),
	)
	if err != nil {
		log.Fatalf("не удалось создать gRPC-клиент для %s: %v", addr, err)
	}
	return conn
}

func main() {
	cfg, err := config.FromEnv()
	if err != nil {
		log.Fatal("ошибка конфигурации: ", err)
	}
	verifier, err := auth.NewVerifier(cfg.JWTSecret)
	if err != nil {
		log.Fatal("ошибка конфигурации: ", err)
	}

	ledgerConn := dial(cfg.LedgerAddr)
	defer ledgerConn.Close()
	authConn := dial(cfg.AuthAddr)
	defer authConn.Close()

	router := api.NewRouter(api.Deps{
		Ledger:   ledgerv1.NewLedgerServiceClient(ledgerConn),
		Auth:     authv1.NewAuthServiceClient(authConn),
		Verifier: verifier,
		Timeout:  cfg.RequestTimeout,
	})

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		log.Println("остановка gateway...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()

	log.Printf("Gateway started on %s, ledger=%s, auth=%s, таймаут запросов %s",
		cfg.HTTPAddr, cfg.LedgerAddr, cfg.AuthAddr, cfg.RequestTimeout)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
