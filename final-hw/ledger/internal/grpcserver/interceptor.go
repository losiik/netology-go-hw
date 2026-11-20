package grpcserver

import (
	"context"
	"log"
	"strconv"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"ledger/internal/domain"
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

const UserIDHeader = "x-user-id"

func UserInterceptor(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	if strings.HasPrefix(info.FullMethod, "/grpc.health.v1.Health/") {
		return handler(ctx, req)
	}
	md, _ := metadata.FromIncomingContext(ctx)
	values := md.Get(UserIDHeader)
	if len(values) != 1 {
		return nil, status.Error(codes.Unauthenticated, "не передан "+UserIDHeader)
	}
	id, err := strconv.ParseInt(values[0], 10, 64)
	if err != nil || id <= 0 {
		return nil, status.Error(codes.Unauthenticated, "неверный "+UserIDHeader)
	}
	return handler(domain.WithUserID(ctx, id), req)
}
