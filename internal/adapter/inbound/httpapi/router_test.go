package httpapi_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/lnwbigbangsm-ops/backend-golang-coding-test/internal/adapter/inbound/httpapi"
	"github.com/lnwbigbangsm-ops/backend-golang-coding-test/internal/adapter/outbound/security"
	"github.com/lnwbigbangsm-ops/backend-golang-coding-test/internal/core/service"
	"github.com/lnwbigbangsm-ops/backend-golang-coding-test/internal/mocks"
)

// newTestServer wires the real core services and JWT adapter against an
// in-memory repository, so the full HTTP flow is exercised without MongoDB.
func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	repo := mocks.NewMemoryUserRepository()
	hasher := security.NewBcryptHasher(bcrypt.MinCost)
	tokens, err := security.NewJWTService("test-secret-that-is-at-least-32-bytes-long", "test", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	users := service.NewUserService(repo, hasher)
	auth, err := service.NewAuthService(users, repo, hasher, tokens)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(httpapi.NewRouter(users, auth, slog.New(slog.DiscardHandler)))
	t.Cleanup(srv.Close)
	return srv
}

type apiClient struct {
	t     *testing.T
	base  string
	token string
}

func (c *apiClient) do(method, path, body string) (int, map[string]any) {
	c.t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, c.base+path, r)
	if err != nil {
		c.t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	if strings.HasPrefix(resp.Header.Get("Content-Type"), "application/json") && len(bytes.TrimSpace(raw)) > 0 {
		if err := json.Unmarshal(raw, &out); err != nil {
			c.t.Fatalf("%s %s: invalid JSON %q", method, path, raw)
		}
	}
	return resp.StatusCode, out
}

func errCode(body map[string]any) string {
	e, _ := body["error"].(map[string]any)
	s, _ := e["code"].(string)
	return s
}

func TestAPI_FullFlow(t *testing.T) {
	srv := newTestServer(t)
	c := &apiClient{t: t, base: srv.URL}

	// Register.
	status, body := c.do("POST", "/api/v1/auth/register", `{"name":"Alice","email":"Alice@Example.com","password":"password123"}`)
	if status != http.StatusCreated {
		t.Fatalf("register status = %d, body = %v", status, body)
	}
	aliceID, _ := body["id"].(string)
	if aliceID == "" || body["email"] != "alice@example.com" {
		t.Fatalf("register body = %v", body)
	}
	if _, leaked := body["password"]; leaked {
		t.Fatal("password must not be returned")
	}
	if _, leaked := body["password_hash"]; leaked {
		t.Fatal("password hash must not be returned")
	}

	// Duplicate email (case-insensitive).
	if status, body = c.do("POST", "/api/v1/auth/register", `{"name":"A2","email":"ALICE@example.com","password":"password123"}`); status != http.StatusConflict {
		t.Errorf("duplicate register status = %d, body = %v", status, body)
	}

	// Invalid input.
	status, body = c.do("POST", "/api/v1/auth/register", `{"name":"","email":"nope","password":"x"}`)
	if status != http.StatusBadRequest || errCode(body) != "validation_error" {
		t.Errorf("invalid register = %d %v", status, body)
	}

	// Login failures.
	if status, _ = c.do("POST", "/api/v1/auth/login", `{"email":"alice@example.com","password":"wrong-pass"}`); status != http.StatusUnauthorized {
		t.Errorf("wrong password status = %d", status)
	}
	if status, _ = c.do("POST", "/api/v1/auth/login", `{"email":"bob@example.com","password":"password123"}`); status != http.StatusUnauthorized {
		t.Errorf("unknown email status = %d", status)
	}

	// Protected route without / with bad token.
	if status, _ = c.do("GET", "/api/v1/users", ""); status != http.StatusUnauthorized {
		t.Errorf("no token status = %d", status)
	}
	c.token = "not.a.jwt"
	if status, _ = c.do("GET", "/api/v1/users", ""); status != http.StatusUnauthorized {
		t.Errorf("bad token status = %d", status)
	}

	// Login success.
	c.token = ""
	status, body = c.do("POST", "/api/v1/auth/login", `{"email":"alice@example.com","password":"password123"}`)
	if status != http.StatusOK || body["token_type"] != "Bearer" {
		t.Fatalf("login = %d %v", status, body)
	}
	c.token = body["access_token"].(string)

	// Create another user via protected endpoint.
	status, body = c.do("POST", "/api/v1/users", `{"name":"Bob","email":"bob@example.com","password":"password123"}`)
	if status != http.StatusCreated {
		t.Fatalf("create = %d %v", status, body)
	}
	bobID := body["id"].(string)

	// List.
	status, body = c.do("GET", "/api/v1/users", "")
	if status != http.StatusOK || body["total"].(float64) != 2 {
		t.Errorf("list = %d %v", status, body)
	}

	// Get.
	if status, body = c.do("GET", "/api/v1/users/"+bobID, ""); status != http.StatusOK || body["name"] != "Bob" {
		t.Errorf("get = %d %v", status, body)
	}
	if status, _ = c.do("GET", "/api/v1/users/does-not-exist", ""); status != http.StatusNotFound {
		t.Errorf("get missing status = %d", status)
	}

	// Update.
	status, body = c.do("PATCH", "/api/v1/users/"+bobID, `{"name":"Robert","email":"robert@example.com"}`)
	if status != http.StatusOK || body["name"] != "Robert" || body["email"] != "robert@example.com" {
		t.Errorf("update = %d %v", status, body)
	}
	if status, _ = c.do("PATCH", "/api/v1/users/"+bobID, `{"email":"alice@example.com"}`); status != http.StatusConflict {
		t.Errorf("update to taken email status = %d", status)
	}
	if status, body = c.do("PATCH", "/api/v1/users/"+bobID, `{"password":"new-password"}`); status != http.StatusBadRequest {
		t.Errorf("update with unknown field = %d %v", status, body)
	}
	if status, body = c.do("PATCH", "/api/v1/users/"+bobID, `{}`); status != http.StatusBadRequest || errCode(body) != "validation_error" {
		t.Errorf("empty update = %d %v", status, body)
	}

	// Delete.
	if status, _ = c.do("DELETE", "/api/v1/users/"+bobID, ""); status != http.StatusNoContent {
		t.Errorf("delete status = %d", status)
	}
	if status, _ = c.do("DELETE", "/api/v1/users/"+bobID, ""); status != http.StatusNotFound {
		t.Errorf("second delete status = %d", status)
	}

	// Deleting yourself invalidates your token.
	if status, _ = c.do("DELETE", "/api/v1/users/"+aliceID, ""); status != http.StatusNoContent {
		t.Errorf("self delete status = %d", status)
	}
	if status, _ = c.do("GET", "/api/v1/users", ""); status != http.StatusUnauthorized {
		t.Errorf("token of deleted user status = %d", status)
	}
}

func TestAPI_BadRequests(t *testing.T) {
	srv := newTestServer(t)
	c := &apiClient{t: t, base: srv.URL}

	tests := []struct {
		name, method, path, body string
		want                     int
	}{
		{"empty body", "POST", "/api/v1/auth/register", "", http.StatusBadRequest},
		{"malformed JSON", "POST", "/api/v1/auth/register", `{"name":`, http.StatusBadRequest},
		{"two objects", "POST", "/api/v1/auth/login", `{} {}`, http.StatusBadRequest},
		{"too large", "POST", "/api/v1/auth/register", `{"name":"` + strings.Repeat("a", 2<<20) + `"}`, http.StatusBadRequest},
		{"unknown route", "GET", "/nope", "", http.StatusNotFound},
		{"wrong method", "PUT", "/api/v1/auth/login", "", http.StatusMethodNotAllowed},
		{"health", "GET", "/healthz", "", http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if status, body := c.do(tt.method, tt.path, tt.body); status != tt.want {
				t.Errorf("status = %d, want %d (body %v)", status, tt.want, body)
			}
		})
	}
}
