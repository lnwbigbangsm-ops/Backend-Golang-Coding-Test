// Package grpcapi is the gRPC transport adapter.
package grpcapi

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	userv1 "github.com/lnwbigbangsm-ops/backend-golang-coding-test/gen/user/v1"
	"github.com/lnwbigbangsm-ops/backend-golang-coding-test/internal/core/domain"
	"github.com/lnwbigbangsm-ops/backend-golang-coding-test/internal/core/port"
)

// Server implements userv1.UserServiceServer.
type Server struct {
	userv1.UnimplementedUserServiceServer
	users  port.UserService
	logger *slog.Logger
}

// NewServer builds a gRPC server with JWT and logging interceptors and the
// user service registered.
func NewServer(users port.UserService, auth port.AuthService, logger *slog.Logger) *grpc.Server {
	s := grpc.NewServer(grpc.ChainUnaryInterceptor(
		LoggingInterceptor(logger),
		AuthInterceptor(auth),
	))
	userv1.RegisterUserServiceServer(s, &Server{users: users, logger: logger})
	return s
}

func (s *Server) CreateUser(ctx context.Context, req *userv1.CreateUserRequest) (*userv1.CreateUserResponse, error) {
	// Same rule as REST POST /users: the authenticated caller becomes the creator.
	var creator string
	if c, ok := ctx.Value(claimsKey{}).(*port.TokenClaims); ok {
		creator = c.UserID
	}
	user, err := s.users.Create(ctx, port.CreateUserInput{
		Name: req.GetName(), Email: req.GetEmail(), Password: req.GetPassword(), CreatedBy: creator,
	})
	if err != nil {
		return nil, s.toStatus(ctx, err)
	}
	return &userv1.CreateUserResponse{User: toProto(user)}, nil
}

func (s *Server) GetUser(ctx context.Context, req *userv1.GetUserRequest) (*userv1.GetUserResponse, error) {
	user, err := s.users.Get(ctx, req.GetId())
	if err != nil {
		return nil, s.toStatus(ctx, err)
	}
	return &userv1.GetUserResponse{User: toProto(user)}, nil
}

func toProto(u *domain.User) *userv1.User {
	return &userv1.User{
		Id:        u.ID,
		Name:      u.Name,
		Email:     u.Email,
		CreatedAt: timestamppb.New(u.CreatedAt),
	}
}

func (s *Server) toStatus(ctx context.Context, err error) error {
	var verr *domain.ValidationError
	switch {
	case errors.As(err, &verr):
		return status.Error(codes.InvalidArgument, verr.Error())
	case errors.Is(err, domain.ErrUserNotFound):
		return status.Error(codes.NotFound, "user not found")
	case errors.Is(err, domain.ErrEmailAlreadyExists):
		return status.Error(codes.AlreadyExists, "email already exists")
	case errors.Is(err, domain.ErrForbidden):
		return status.Error(codes.PermissionDenied, "permission denied")
	default:
		s.logger.ErrorContext(ctx, "grpc call failed", "error", err)
		return status.Error(codes.Internal, "internal error")
	}
}

// claimsKey stores the authenticated caller's claims in the call context.
type claimsKey struct{}

// AuthInterceptor requires an "authorization: Bearer <jwt>" metadata entry
// and puts the verified claims into the call context.
func AuthInterceptor(auth port.AuthService) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		md, _ := metadata.FromIncomingContext(ctx)
		values := md.Get("authorization")
		if len(values) == 0 {
			return nil, status.Error(codes.Unauthenticated, "missing authorization metadata")
		}
		scheme, token, found := strings.Cut(strings.TrimSpace(values[0]), " ")
		if !found || !strings.EqualFold(scheme, "Bearer") || strings.TrimSpace(token) == "" {
			return nil, status.Error(codes.Unauthenticated, "malformed authorization metadata")
		}
		claims, err := auth.Authenticate(ctx, strings.TrimSpace(token))
		if err != nil {
			if errors.Is(err, domain.ErrInvalidToken) {
				return nil, status.Error(codes.Unauthenticated, "invalid or expired token")
			}
			return nil, status.Error(codes.Internal, "internal error")
		}
		return handler(context.WithValue(ctx, claimsKey{}, claims), req)
	}
}

// LoggingInterceptor logs the method, status code and duration of each call.
func LoggingInterceptor(logger *slog.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		start := time.Now()
		resp, err := handler(ctx, req)
		logger.InfoContext(ctx, "grpc request",
			"method", info.FullMethod,
			"code", status.Code(err).String(),
			"duration", time.Since(start),
		)
		return resp, err
	}
}
