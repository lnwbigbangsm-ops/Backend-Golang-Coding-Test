package httpapi_test

import (
	"bytes"
	"encoding/json"
	"fmt"
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

// newTestServer wires the real core services and JWT adapter against
// in-memory stores, so the full HTTP flow is exercised without MongoDB.
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
	router := httpapi.NewRouter(users, auth, mocks.NewMemoryIdempotencyStore(), slog.New(slog.DiscardHandler))
	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)
	return srv
}

type apiClient struct {
	t     *testing.T
	base  string
	token string
}

type response struct {
	status int
	header http.Header
	body   map[string]any
	raw    string
}

func (c *apiClient) do(method, path, body string, headers ...string) response {
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
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	out := response{status: resp.StatusCode, header: resp.Header, raw: string(raw)}
	if len(bytes.TrimSpace(raw)) > 0 {
		if !strings.HasPrefix(resp.Header.Get("Content-Type"), "application/json") {
			c.t.Fatalf("%s %s: non-JSON response %q", method, path, raw)
		}
		if err := json.Unmarshal(raw, &out.body); err != nil {
			c.t.Fatalf("%s %s: invalid JSON %q", method, path, raw)
		}
	}
	return out
}

func (r response) errCode() string {
	e, _ := r.body["error"].(map[string]any)
	s, _ := e["code"].(string)
	return s
}

func (c *apiClient) register(name, email string) string {
	c.t.Helper()
	res := c.do("POST", "/api/v1/auth/register", fmt.Sprintf(`{"name":%q,"email":%q,"password":"password123"}`, name, email))
	if res.status != http.StatusCreated {
		c.t.Fatalf("register %s = %d %v", email, res.status, res.body)
	}
	return res.body["id"].(string)
}

func (c *apiClient) login(email string) {
	c.t.Helper()
	c.token = ""
	res := c.do("POST", "/api/v1/auth/login", fmt.Sprintf(`{"email":%q,"password":"password123"}`, email))
	if res.status != http.StatusOK || res.body["token_type"] != "Bearer" {
		c.t.Fatalf("login %s = %d %v", email, res.status, res.body)
	}
	c.token = res.body["access_token"].(string)
}

func TestAPI_FullFlow(t *testing.T) {
	srv := newTestServer(t)
	c := &apiClient{t: t, base: srv.URL}

	// Register.
	res := c.do("POST", "/api/v1/auth/register", `{"name":"Alice","email":"Alice@Example.com","password":"password123"}`)
	if res.status != http.StatusCreated || res.body["email"] != "alice@example.com" {
		t.Fatalf("register = %d %v", res.status, res.body)
	}
	aliceID := res.body["id"].(string)
	if res.header.Get("Location") != "/api/v1/users/"+aliceID {
		t.Errorf("Location = %q", res.header.Get("Location"))
	}
	for _, leaked := range []string{"password", "password_hash"} {
		if _, ok := res.body[leaked]; ok {
			t.Fatalf("%s must not be returned", leaked)
		}
	}

	// Duplicate email (case-insensitive) and invalid input.
	if res = c.do("POST", "/api/v1/auth/register", `{"name":"A2","email":"ALICE@example.com","password":"password123"}`); res.status != http.StatusConflict {
		t.Errorf("duplicate register = %d %v", res.status, res.body)
	}
	if res = c.do("POST", "/api/v1/auth/register", `{"name":"","email":"nope","password":"x"}`); res.status != http.StatusUnprocessableEntity || res.errCode() != "validation_error" {
		t.Errorf("invalid register = %d %v", res.status, res.body)
	}

	// Login failures.
	if res = c.do("POST", "/api/v1/auth/login", `{"email":"alice@example.com","password":"wrong-pass"}`); res.status != http.StatusUnauthorized {
		t.Errorf("wrong password = %d", res.status)
	}
	if res = c.do("POST", "/api/v1/auth/login", `{"email":"bob@example.com","password":"password123"}`); res.status != http.StatusUnauthorized {
		t.Errorf("unknown email = %d", res.status)
	}

	// Protected route without / with a bad token.
	if res = c.do("GET", "/api/v1/users", ""); res.status != http.StatusUnauthorized {
		t.Errorf("no token = %d", res.status)
	}
	c.token = "not.a.jwt"
	if res = c.do("GET", "/api/v1/users", ""); res.status != http.StatusUnauthorized {
		t.Errorf("bad token = %d", res.status)
	}

	// Alice creates Bob through the protected endpoint.
	c.login("alice@example.com")
	res = c.do("POST", "/api/v1/users", `{"name":"Bob","email":"bob@example.com","password":"password123"}`)
	if res.status != http.StatusCreated {
		t.Fatalf("create = %d %v", res.status, res.body)
	}
	bobID := res.body["id"].(string)

	// List and get.
	res = c.do("GET", "/api/v1/users", "")
	if p, _ := res.body["pagination"].(map[string]any); res.status != http.StatusOK || p["total_items"] != float64(2) {
		t.Errorf("list = %d %v", res.status, res.body)
	}
	if res = c.do("GET", "/api/v1/users/"+bobID, ""); res.status != http.StatusOK || res.body["name"] != "Bob" {
		t.Errorf("get = %d %v", res.status, res.body)
	}
	if res = c.do("GET", "/api/v1/users/does-not-exist", ""); res.status != http.StatusNotFound {
		t.Errorf("get missing = %d", res.status)
	}

	// Alice created Bob, so she may update him.
	if res = c.do("PATCH", "/api/v1/users/"+bobID, `{"name":"Bobby"}`); res.status != http.StatusOK || res.body["name"] != "Bobby" {
		t.Errorf("creator PATCH = %d %v", res.status, res.body)
	}

	// Carol registered herself: Alice may not modify or delete her, and a
	// missing ID gets the same 403 so IDs cannot be probed.
	carolID := c.register("Carol", "carol@example.com")
	for _, id := range []string{carolID, "does-not-exist"} {
		if res = c.do("PATCH", "/api/v1/users/"+id, `{"name":"Hacked"}`); res.status != http.StatusForbidden || res.errCode() != "forbidden" {
			t.Errorf("PATCH other (%s) = %d %v", id, res.status, res.body)
		}
		if res = c.do("DELETE", "/api/v1/users/"+id, ""); res.status != http.StatusForbidden {
			t.Errorf("DELETE other (%s) = %d %v", id, res.status, res.body)
		}
	}
	if res = c.do("GET", "/api/v1/users/"+carolID, ""); res.body["name"] != "Carol" {
		t.Errorf("Carol was modified by Alice: %v", res.body)
	}

	// Bob updates himself; Bob did not create Alice, so he cannot touch her.
	c.login("bob@example.com")
	res = c.do("PATCH", "/api/v1/users/"+bobID, `{"name":"Robert","email":"robert@example.com"}`)
	if res.status != http.StatusOK || res.body["name"] != "Robert" || res.body["email"] != "robert@example.com" {
		t.Errorf("update = %d %v", res.status, res.body)
	}
	if res = c.do("DELETE", "/api/v1/users/"+aliceID, ""); res.status != http.StatusForbidden {
		t.Errorf("created user deleting its creator = %d", res.status)
	}
	if res = c.do("PATCH", "/api/v1/users/"+bobID, `{"email":"alice@example.com"}`); res.status != http.StatusConflict {
		t.Errorf("update to taken email = %d", res.status)
	}
	res = c.do("PATCH", "/api/v1/users/"+bobID, `{"password":"new-password"}`)
	if f, _ := res.body["error"].(map[string]any)["fields"].(map[string]any); res.status != http.StatusBadRequest || f["password"] != "is not allowed" {
		t.Errorf("update with unknown field = %d %v", res.status, res.body)
	}
	if res = c.do("PATCH", "/api/v1/users/"+bobID, `{}`); res.status != http.StatusUnprocessableEntity || res.errCode() != "validation_error" {
		t.Errorf("empty update = %d %v", res.status, res.body)
	}

	// Bob deletes himself; his token stops working immediately.
	if res = c.do("DELETE", "/api/v1/users/"+bobID, ""); res.status != http.StatusNoContent {
		t.Errorf("self delete = %d", res.status)
	}
	if res = c.do("DELETE", "/api/v1/users/"+bobID, ""); res.status != http.StatusUnauthorized {
		t.Errorf("delete retry with deleted user's token = %d", res.status)
	}

	// The creator can delete an account they created; a retry then gets 403
	// (the target no longer exists, which is reported like any other target
	// the caller may not touch).
	c.login("alice@example.com")
	res = c.do("POST", "/api/v1/users", `{"name":"Dan","email":"dan@example.com","password":"password123"}`)
	danID := res.body["id"].(string)
	if res = c.do("DELETE", "/api/v1/users/"+danID, ""); res.status != http.StatusNoContent {
		t.Errorf("creator DELETE = %d %v", res.status, res.body)
	}
	if res = c.do("GET", "/api/v1/users/"+danID, ""); res.status != http.StatusNotFound {
		t.Errorf("deleted user still readable: %d", res.status)
	}
	if res = c.do("DELETE", "/api/v1/users/"+danID, ""); res.status != http.StatusForbidden {
		t.Errorf("creator DELETE retry = %d", res.status)
	}
}

func TestAPI_Pagination(t *testing.T) {
	srv := newTestServer(t)
	c := &apiClient{t: t, base: srv.URL}
	for i := range 5 {
		c.register(fmt.Sprintf("User %d", i), fmt.Sprintf("user%d@example.com", i))
	}
	c.login("user0@example.com")

	res := c.do("GET", "/api/v1/users?page=2&page_size=2", "")
	data, _ := res.body["data"].([]any)
	p, _ := res.body["pagination"].(map[string]any)
	if res.status != http.StatusOK || len(data) != 2 {
		t.Fatalf("page 2 = %d %v", res.status, res.body)
	}
	if first := data[0].(map[string]any); first["email"] != "user2@example.com" {
		t.Errorf("page 2 starts with %v, want user2 (oldest first)", first["email"])
	}
	if p["page"] != float64(2) || p["page_size"] != float64(2) || p["total_items"] != float64(5) || p["total_pages"] != float64(3) {
		t.Errorf("pagination = %v", p)
	}

	// Defaults.
	res = c.do("GET", "/api/v1/users", "")
	if p, _ := res.body["pagination"].(map[string]any); p["page"] != float64(1) || p["page_size"] != float64(20) {
		t.Errorf("default pagination = %v", res.body["pagination"])
	}
	// Past the end.
	if res = c.do("GET", "/api/v1/users?page=9", ""); res.status != http.StatusOK || len(res.body["data"].([]any)) != 0 {
		t.Errorf("past end = %d %v", res.status, res.body)
	}

	for _, q := range []string{"page=0", "page=abc", "page_size=0", "page_size=101", "page_size=x"} {
		if res = c.do("GET", "/api/v1/users?"+q, ""); res.status != http.StatusUnprocessableEntity || res.errCode() != "validation_error" {
			t.Errorf("?%s = %d %v", q, res.status, res.body)
		}
	}
}

func TestAPI_IdempotentCreate(t *testing.T) {
	srv := newTestServer(t)
	c := &apiClient{t: t, base: srv.URL}
	body := `{"name":"Carol","email":"carol@example.com","password":"password123"}`

	first := c.do("POST", "/api/v1/auth/register", body, "Idempotency-Key", "reg-1")
	retry := c.do("POST", "/api/v1/auth/register", body, "Idempotency-Key", "reg-1")
	if first.status != http.StatusCreated || retry.status != http.StatusCreated {
		t.Fatalf("first = %d, retry = %d %v", first.status, retry.status, retry.body)
	}
	if retry.body["id"] != first.body["id"] || retry.header.Get("Idempotent-Replayed") != "true" {
		t.Errorf("retry did not replay the original response: %v %v", retry.body, retry.header)
	}
	if retry.header.Get("Location") != first.header.Get("Location") {
		t.Errorf("replayed Location = %q", retry.header.Get("Location"))
	}

	// Without a key the retry is a new request and hits the unique email.
	if res := c.do("POST", "/api/v1/auth/register", body); res.status != http.StatusConflict {
		t.Errorf("retry without key = %d", res.status)
	}
	// Same key, different body.
	if res := c.do("POST", "/api/v1/auth/register", `{"name":"Dave","email":"dave@example.com","password":"password123"}`, "Idempotency-Key", "reg-1"); res.status != http.StatusUnprocessableEntity || res.errCode() != "idempotency_key_reused" {
		t.Errorf("reused key = %d %v", res.status, res.body)
	}

	// Keys on POST /users are scoped per caller: the same key from two users
	// creates two users.
	c.login("carol@example.com")
	a := c.do("POST", "/api/v1/users", `{"name":"E","email":"e@example.com","password":"password123"}`, "Idempotency-Key", "same")
	c.register("Frank", "frank@example.com")
	c.login("frank@example.com")
	b := c.do("POST", "/api/v1/users", `{"name":"E","email":"e@example.com","password":"password123"}`, "Idempotency-Key", "same")
	if a.status != http.StatusCreated || b.status != http.StatusConflict || b.header.Get("Idempotent-Replayed") != "" {
		t.Errorf("per-caller scoping: a=%d b=%d replayed=%q", a.status, b.status, b.header.Get("Idempotent-Replayed"))
	}
}

func TestAPI_ErrorsAreAlwaysJSON(t *testing.T) {
	srv := newTestServer(t)
	c := &apiClient{t: t, base: srv.URL}

	tests := []struct {
		name, method, path, body string
		want                     int
		code                     string
	}{
		{"empty body", "POST", "/api/v1/auth/register", "", http.StatusBadRequest, "bad_request"},
		{"malformed JSON", "POST", "/api/v1/auth/register", `{"name":`, http.StatusBadRequest, "bad_request"},
		{"wrong type", "POST", "/api/v1/auth/register", `{"name":1}`, http.StatusBadRequest, "bad_request"},
		{"two objects", "POST", "/api/v1/auth/login", `{} {}`, http.StatusBadRequest, "bad_request"},
		{"too large", "POST", "/api/v1/auth/register", `{"name":"` + strings.Repeat("a", 2<<20) + `"}`, http.StatusBadRequest, "bad_request"},
		{"unknown route", "GET", "/nope", "", http.StatusNotFound, "not_found"},
		{"wrong method", "PUT", "/api/v1/auth/login", "", http.StatusMethodNotAllowed, "method_not_allowed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := c.do(tt.method, tt.path, tt.body)
			if res.status != tt.want || res.errCode() != tt.code {
				t.Errorf("got %d %q, want %d %q (body %s)", res.status, res.errCode(), tt.want, tt.code, res.raw)
			}
			if strings.Contains(res.raw, "json:") {
				t.Errorf("response leaks encoding/json text: %s", res.raw)
			}
		})
	}

	if res := c.do("PUT", "/api/v1/auth/login", ""); res.header.Get("Allow") != "POST" {
		t.Errorf("405 Allow = %q, want POST", res.header.Get("Allow"))
	}
	if res := c.do("GET", "/healthz", ""); res.status != http.StatusOK || res.body["status"] != "ok" {
		t.Errorf("health = %d %v", res.status, res.body)
	}
}
