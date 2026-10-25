package grpcapi_test

import (
	"context"
	"log/slog"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	userv1 "github.com/lnwbigbangsm-ops/backend-golang-coding-test/gen/user/v1"
	"github.com/lnwbigbangsm-ops/backend-golang-coding-test/internal/adapter/inbound/grpcapi"
	"github.com/lnwbigbangsm-ops/backend-golang-coding-test/internal/core/domain"
	"github.com/lnwbigbangsm-ops/backend-golang-coding-test/internal/core/port"
	"github.com/lnwbigbangsm-ops/backend-golang-coding-test/internal/core/service"
	"github.com/lnwbigbangsm-ops/backend-golang-coding-test/internal/mocks"
)

// newTestClient starts the gRPC server on an in-memory listener. Tokens are
// mocked: "valid-token" authenticates, anything else is rejected.
func newTestClient(t *testing.T) userv1.UserServiceClient {
	t.Helper()
	repo := mocks.NewMemoryUserRepository()
	users := service.NewUserService(repo, &mocks.PasswordHasher{})
	auth := &mocks.AuthService{AuthenticateFn: func(_ context.Context, token string) (*port.TokenClaims, error) {
		if token == "valid-token" {
			return &port.TokenClaims{UserID: "caller"}, nil
		}
		return nil, domain.ErrInvalidToken
	}}

	lis := bufconn.Listen(1 << 20)
	srv := grpcapi.NewServer(users, auth, slog.New(slog.DiscardHandler))
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return userv1.NewUserServiceClient(conn)
}

func withToken(token string) context.Context {
	return metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer "+token)
}

func TestGRPC_Auth(t *testing.T) {
	client := newTestClient(t)
	tests := map[string]context.Context{
		"no metadata":   context.Background(),
		"invalid token": withToken("nope"),
		"wrong scheme":  metadata.AppendToOutgoingContext(context.Background(), "authorization", "Basic abc"),
	}
	for name, ctx := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := client.GetUser(ctx, &userv1.GetUserRequest{Id: "x"})
			if status.Code(err) != codes.Unauthenticated {
				t.Errorf("code = %v, want Unauthenticated", status.Code(err))
			}
		})
	}
}

func TestGRPC_CreateAndGet(t *testing.T) {
	client := newTestClient(t)
	ctx := withToken("valid-token")

	created, err := client.CreateUser(ctx, &userv1.CreateUserRequest{Name: "Alice", Email: "alice@example.com", Password: "password123"})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	u := created.GetUser()
	if u.GetId() == "" || u.GetEmail() != "alice@example.com" || u.GetCreatedAt() == nil {
		t.Fatalf("created user = %v", u)
	}

	got, err := client.GetUser(ctx, &userv1.GetUserRequest{Id: u.GetId()})
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	if got.GetUser().GetName() != "Alice" {
		t.Errorf("got user = %v", got.GetUser())
	}
}

func TestGRPC_ErrorCodes(t *testing.T) {
	client := newTestClient(t)
	ctx := withToken("valid-token")
	valid := &userv1.CreateUserRequest{Name: "Alice", Email: "alice@example.com", Password: "password123"}
	if _, err := client.CreateUser(ctx, valid); err != nil {
		t.Fatal(err)
	}

	_, err := client.CreateUser(ctx, valid)
	if status.Code(err) != codes.AlreadyExists {
		t.Errorf("duplicate code = %v", status.Code(err))
	}
	_, err = client.CreateUser(ctx, &userv1.CreateUserRequest{Email: "bad"})
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("invalid code = %v", status.Code(err))
	}
	_, err = client.GetUser(ctx, &userv1.GetUserRequest{Id: "missing"})
	if status.Code(err) != codes.NotFound {
		t.Errorf("missing code = %v", status.Code(err))
	}
}
