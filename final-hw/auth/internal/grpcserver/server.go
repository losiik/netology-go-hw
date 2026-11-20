package grpcserver

import (
	"context"
	"errors"
	"log"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"auth/internal/domain"
	"auth/internal/service"
	"proto/authv1"
)

type Server struct {
	authv1.UnimplementedAuthServiceServer
	svc service.Service
}

func New(svc service.Service) *Server {
	return &Server{svc: svc}
}

func toStatus(method string, err error) error {
	switch {
	case errors.Is(err, domain.ErrValidation):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, domain.ErrUserExists):
		return status.Error(codes.AlreadyExists, err.Error())
	case errors.Is(err, domain.ErrInvalidCredentials):
		return status.Error(codes.Unauthenticated, err.Error())
	case errors.Is(err, context.DeadlineExceeded):
		return status.Error(codes.DeadlineExceeded, err.Error())
	case errors.Is(err, context.Canceled):
		return status.Error(codes.Canceled, err.Error())
	default:
		log.Printf("%s: %v", method, err)
		return status.Error(codes.Internal, "internal error")
	}
}

func (s *Server) Register(ctx context.Context, req *authv1.RegisterRequest) (*authv1.RegisterResponse, error) {
	u, err := s.svc.Register(ctx, req.GetLogin(), req.GetPassword())
	if err != nil {
		return nil, toStatus("Register", err)
	}
	return &authv1.RegisterResponse{Id: u.ID, Login: u.Login}, nil
}

func (s *Server) Login(ctx context.Context, req *authv1.LoginRequest) (*authv1.LoginResponse, error) {
	token, exp, err := s.svc.Login(ctx, req.GetLogin(), req.GetPassword())
	if err != nil {
		return nil, toStatus("Login", err)
	}
	return &authv1.LoginResponse{Token: token, ExpiresAt: exp.Unix()}, nil
}
