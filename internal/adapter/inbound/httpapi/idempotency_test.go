package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lnwbigbangsm-ops/backend-golang-coding-test/internal/core/port"
	"github.com/lnwbigbangsm-ops/backend-golang-coding-test/internal/mocks"
)

// countingHandler records how many times the guarded effect ran.
type countingHandler struct {
	calls  atomic.Int32
	status int
}

func (h *countingHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	n := h.calls.Add(1)
	body, _ := io.ReadAll(r.Body)
	w.Header().Set("Location", "/things/1")
	writeJSON(w, h.status, map[string]any{"call": n, "echo": string(body)})
}

func postWithKey(h http.Handler, key, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", "/things", strings.NewReader(body))
	if key != "" {
		req.Header.Set(idempotencyHeader, key)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func newIdempotent(status int) (http.Handler, *countingHandler, *mocks.MemoryIdempotencyStore) {
	store := mocks.NewMemoryIdempotencyStore()
	inner := &countingHandler{status: status}
	return Idempotency(store, slog.New(slog.DiscardHandler))(inner), inner, store
}

func TestIdempotency_ReplaysSameRequest(t *testing.T) {
	h, inner, _ := newIdempotent(http.StatusCreated)

	first := postWithKey(h, "k1", `{"a":1}`)
	second := postWithKey(h, "k1", `{"a":1}`)

	if inner.calls.Load() != 1 {
		t.Fatalf("effect ran %d times, want 1", inner.calls.Load())
	}
	if second.Code != http.StatusCreated || second.Body.String() != first.Body.String() {
		t.Errorf("replay = %d %q, want %d %q", second.Code, second.Body, first.Code, first.Body)
	}
	if second.Header().Get("Idempotent-Replayed") != "true" || second.Header().Get("Location") != "/things/1" {
		t.Errorf("replay headers = %v", second.Header())
	}
	if first.Header().Get("Idempotent-Replayed") != "" {
		t.Error("first response must not be marked as replayed")
	}
}

func TestIdempotency_NoKeyIsNotGuarded(t *testing.T) {
	h, inner, _ := newIdempotent(http.StatusCreated)
	postWithKey(h, "", `{}`)
	postWithKey(h, "", `{}`)
	if inner.calls.Load() != 2 {
		t.Errorf("effect ran %d times, want 2", inner.calls.Load())
	}
}

func TestIdempotency_DifferentBodyIsRejected(t *testing.T) {
	h, inner, _ := newIdempotent(http.StatusCreated)
	postWithKey(h, "k1", `{"a":1}`)
	rec := postWithKey(h, "k1", `{"a":2}`)

	if rec.Code != http.StatusUnprocessableEntity || errorCodeOf(t, rec) != "idempotency_key_reused" {
		t.Errorf("got %d %s", rec.Code, rec.Body)
	}
	if inner.calls.Load() != 1 {
		t.Errorf("effect ran %d times, want 1", inner.calls.Load())
	}
}

func TestIdempotency_InFlightDuplicateGets409(t *testing.T) {
	h, inner, store := newIdempotent(http.StatusCreated)
	// Simulate a first request that is still running.
	hash := sha256Hex(`{"a":1}`)
	_, _, _ = store.Claim(context.Background(), port.IdempotencyRecord{
		Scope: "POST /things anonymous", Key: "k1", RequestHash: hash, ClaimedAt: time.Now(),
	}, time.Time{})

	rec := postWithKey(h, "k1", `{"a":1}`)
	if rec.Code != http.StatusConflict || errorCodeOf(t, rec) != "request_in_progress" {
		t.Errorf("got %d %s", rec.Code, rec.Body)
	}
	if inner.calls.Load() != 0 {
		t.Error("in-flight duplicate must not run the effect")
	}
}

func TestIdempotency_StaleClaimIsTakenOver(t *testing.T) {
	h, inner, store := newIdempotent(http.StatusCreated)
	hash := sha256Hex(`{"a":1}`)
	_, _, _ = store.Claim(context.Background(), port.IdempotencyRecord{
		Scope: "POST /things anonymous", Key: "k1", RequestHash: hash, ClaimedAt: time.Now().Add(-time.Hour),
	}, time.Time{})

	if rec := postWithKey(h, "k1", `{"a":1}`); rec.Code != http.StatusCreated || inner.calls.Load() != 1 {
		t.Errorf("got %d after %d calls, want takeover", rec.Code, inner.calls.Load())
	}
}

func TestIdempotency_ServerErrorIsNotStored(t *testing.T) {
	h, inner, _ := newIdempotent(http.StatusInternalServerError)
	postWithKey(h, "k1", `{}`)
	postWithKey(h, "k1", `{}`)
	if inner.calls.Load() != 2 {
		t.Errorf("effect ran %d times, want 2 (5xx must allow retry)", inner.calls.Load())
	}
}

func TestIdempotency_KeyTooLong(t *testing.T) {
	h, inner, _ := newIdempotent(http.StatusCreated)
	if rec := postWithKey(h, strings.Repeat("k", maxIdempotencyKey+1), `{}`); rec.Code != http.StatusBadRequest {
		t.Errorf("code = %d, want 400", rec.Code)
	}
	if inner.calls.Load() != 0 {
		t.Error("effect must not run")
	}
}

func TestToBodyError_StableMessages(t *testing.T) {
	type target struct {
		Name string `json:"name"`
	}
	tests := []struct {
		body, msg, field string
	}{
		{``, "request body is empty", ""},
		{`{"name":`, "request body is not valid JSON", ""},
		{`{"name":1}`, "request body has a field of the wrong type", "name"},
		{`{"password":"x"}`, "request body contains an unknown field", "password"},
		{`[]`, "request body must be a JSON object", ""},
	}
	for _, tt := range tests {
		req := httptest.NewRequest("POST", "/", strings.NewReader(tt.body))
		var dst target
		err := decodeJSON(httptest.NewRecorder(), req, &dst)
		berr, ok := err.(*bodyError)
		if !ok || berr.msg != tt.msg || (tt.field != "" && berr.fields[tt.field] == "") {
			t.Errorf("body %q: got %#v, want msg %q field %q", tt.body, err, tt.msg, tt.field)
		}
		if ok && strings.Contains(berr.msg, "json:") {
			t.Errorf("body %q: message leaks encoding/json text: %q", tt.body, berr.msg)
		}
	}
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func errorCodeOf(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("not a JSON error body: %q", rec.Body)
	}
	return body.Error.Code
}
