package grpcserver

import (
	"context"
	"log"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

func LoggingInterceptor(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	if strings.HasPrefix(info.FullMethod, "/grpc.health.v1.Health/") {
		return handler(ctx, req)
	}
	start := time.Now()
	resp, err := handler(ctx, req)
	log.Printf("grpc %s %s %s", info.FullMethod, status.Code(err), time.Since(start))
	return resp, err
}
