package main

import (
	"context"
	"flag"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	"auth/internal/app"
	"auth/internal/grpcserver"
	"proto/authv1"
)

func main() {
	healthcheck := flag.Bool("health", false, "проверить, что gRPC-сервер отвечает, и выйти")
	flag.Parse()

	if *healthcheck {
		addr := os.Getenv("AUTH_GRPC_ADDR")
		if addr == "" {
			addr = ":9091"
		}
		os.Exit(checkHealth(addr))
	}

	cfg, err := app.ConfigFromEnv()
	if err != nil {
		log.Fatal("ошибка конфигурации: ", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	svc, closeFn, err := app.New(ctx, cfg)
	if err != nil {
		log.Fatal("не удалось запустить auth: ", err)
	}
	defer closeFn()

	lis, err := net.Listen("tcp", cfg.GRPCAddr)
	if err != nil {
		log.Fatal(err)
	}

	srv := grpc.NewServer(grpc.UnaryInterceptor(grpcserver.LoggingInterceptor))
	authv1.RegisterAuthServiceServer(srv, grpcserver.New(svc))
	healthpb.RegisterHealthServer(srv, health.NewServer())

	go func() {
		<-ctx.Done()
		log.Println("остановка auth...")
		srv.GracefulStop()
	}()

	log.Printf("Auth gRPC started on %s, время жизни токена %s", cfg.GRPCAddr, cfg.TokenTTL)
	if err := srv.Serve(lis); err != nil {
		log.Println(err)
	}
}

func checkHealth(addr string) int {
	if strings.HasPrefix(addr, ":") {
		addr = "localhost" + addr
	}
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Println(err)
		return 1
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	resp, err := healthpb.NewHealthClient(conn).Check(ctx, &healthpb.HealthCheckRequest{})
	if err != nil || resp.GetStatus() != healthpb.HealthCheckResponse_SERVING {
		log.Println("auth не готов:", err)
		return 1
	}
	return 0
}
