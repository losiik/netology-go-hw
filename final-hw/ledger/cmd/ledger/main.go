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

	"ledger/internal/app"
	"ledger/internal/grpcserver"
	"proto/ledgerv1"
)

func main() {
	healthcheck := flag.Bool("health", false, "проверить, что gRPC-сервер отвечает, и выйти")
	flag.Parse()

	cfg, err := app.ConfigFromEnv()
	if err != nil {
		log.Fatal("ошибка конфигурации: ", err)
	}
	if *healthcheck {
		os.Exit(checkHealth(cfg.GRPCAddr))
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	svc, closeFn, err := app.New(ctx, cfg)
	if err != nil {
		log.Fatal("не удалось запустить ledger: ", err)
	}
	defer func() {
		if err := closeFn(); err != nil {
			log.Println("ошибка закрытия соединений:", err)
		}
	}()

	lis, err := net.Listen("tcp", cfg.GRPCAddr)
	if err != nil {
		log.Fatal(err)
	}

	srv := grpc.NewServer(
		grpc.ChainUnaryInterceptor(grpcserver.LoggingInterceptor, grpcserver.UserInterceptor),
		grpc.MaxRecvMsgSize(16<<20),
		grpc.MaxSendMsgSize(32<<20),
	)
	ledgerv1.RegisterLedgerServiceServer(srv, grpcserver.New(svc))
	healthpb.RegisterHealthServer(srv, health.NewServer())

	go func() {
		<-ctx.Done()
		log.Println("остановка ledger...")
		srv.GracefulStop()
	}()

	log.Printf("Ledger gRPC started on %s", cfg.GRPCAddr)
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
		log.Println("ledger не готов:", err)
		return 1
	}
	return 0
}
