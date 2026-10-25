package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lnwbigbangsm-ops/backend-golang-coding-test/internal/core/domain"
	"github.com/lnwbigbangsm-ops/backend-golang-coding-test/internal/core/port"
	"github.com/lnwbigbangsm-ops/backend-golang-coding-test/internal/mocks"
)

func TestBearerToken(t *testing.T) {
	tests := []struct {
		header string
		token  string
		ok     bool
	}{
		{"Bearer abc.def.ghi", "abc.def.ghi", true},
		{"bearer abc", "abc", true},
		{"  Bearer   abc  ", "abc", true},
		{"", "", false},
		{"Bearer", "", false},
		{"Bearer ", "", false},
		{"Basic dXNlcjpwYXNz", "", false},
		{"abc.def.ghi", "", false},
	}
	for _, tt := range tests {
		token, ok := bearerToken(tt.header)
		if token != tt.token || ok != tt.ok {
			t.Errorf("bearerToken(%q) = %q, %v; want %q, %v", tt.header, token, ok, tt.token, tt.ok)
		}
	}
}

func TestAuthenticate_SetsClaims(t *testing.T) {
	auth := &mocks.AuthService{AuthenticateFn: func(_ context.Context, token string) (*port.TokenClaims, error) {
		if token == "good" {
			return &port.TokenClaims{UserID: "id-1"}, nil
		}
		return nil, domain.ErrInvalidToken
	}}
	var seen *port.TokenClaims
	h := Authenticate(auth, slog.New(slog.DiscardHandler))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen, _ = ClaimsFromContext(r.Context())
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer good")
	h.ServeHTTP(httptest.NewRecorder(), req)
	if seen == nil || seen.UserID != "id-1" {
		t.Errorf("claims in context = %+v", seen)
	}

	req.Header.Set("Authorization", "Bearer bad")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized || rec.Header().Get("WWW-Authenticate") == "" {
		t.Errorf("bad token: code=%d www-auth=%q", rec.Code, rec.Header().Get("WWW-Authenticate"))
	}
}

func TestLogging(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	h := Logging(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("DELETE", "/api/v1/users/42", nil))

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("log is not JSON: %q", buf.String())
	}
	if entry["method"] != "DELETE" || entry["path"] != "/api/v1/users/42" || entry["status"] != float64(http.StatusTeapot) {
		t.Errorf("log entry = %v", entry)
	}
	if _, ok := entry["duration"]; !ok {
		t.Errorf("log entry has no duration: %v", entry)
	}
}

func TestRecover(t *testing.T) {
	h := Recover(slog.New(slog.DiscardHandler))(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("code = %d, want 500", rec.Code)
	}
}

func TestHandler_InternalErrorIsHidden(t *testing.T) {
	var logs bytes.Buffer
	users := &mocks.UserService{ListFn: func(context.Context) ([]*domain.User, error) {
		return nil, errors.New("connection refused: mongo-secret-host:27017")
	}}
	h := NewHandler(users, &mocks.AuthService{}, slog.New(slog.NewTextHandler(&logs, nil)))

	rec := httptest.NewRecorder()
	h.listUsers(rec, httptest.NewRequest("GET", "/api/v1/users", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("code = %d", rec.Code)
	}
	if bytes.Contains(rec.Body.Bytes(), []byte("mongo-secret-host")) {
		t.Error("internal error details leaked to client")
	}
	if !bytes.Contains(logs.Bytes(), []byte("mongo-secret-host")) {
		t.Error("internal error was not logged")
	}
}
