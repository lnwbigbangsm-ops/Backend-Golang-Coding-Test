package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/lnwbigbangsm-ops/backend-golang-coding-test/internal/core/port"
)

const (
	idempotencyHeader = "Idempotency-Key"
	maxIdempotencyKey = 255
	// idempotencyLease is how long an in-progress claim blocks duplicates
	// before a retry may take it over. Taking over is safe here because the
	// only guarded effect, creating a user, is also protected by the unique
	// email index; a takeover can at worst produce 409 email_exists.
	idempotencyLease = 30 * time.Second
)

// Idempotency honours an optional Idempotency-Key header on POST requests.
//
//   - First request with a key: claimed atomically, executed, and its
//     response stored.
//   - Retry with the same key and body: the stored response is replayed with
//     an "Idempotent-Replayed: true" header.
//   - Same key with a different body: 422, because that is a client bug.
//   - Same key while the first request is still running: 409.
//   - A 5xx response is not stored, so the client can retry.
//
// Keys are scoped to the method, path and caller, so two users cannot collide.
func Idempotency(store port.IdempotencyStore, logger *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := r.Header.Get(idempotencyHeader)
			if key == "" {
				next.ServeHTTP(w, r)
				return
			}
			if len(key) > maxIdempotencyKey {
				writeErrorCode(w, http.StatusBadRequest, "bad_request", "Idempotency-Key must be at most 255 characters")
				return
			}

			// Buffer the body so it can be hashed and then read by the handler.
			body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
			if err != nil {
				writeError(w, r, logger, toBodyError(err))
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(body))
			sum := sha256.Sum256(body)

			caller := actorID(r)
			if caller == "" {
				caller = "anonymous"
			}
			rec := port.IdempotencyRecord{
				Scope:       r.Method + " " + r.URL.Path + " " + caller,
				Key:         key,
				RequestHash: hex.EncodeToString(sum[:]),
				ClaimedAt:   time.Now(),
			}

			existing, claimed, err := store.Claim(r.Context(), rec, rec.ClaimedAt.Add(-idempotencyLease))
			if err != nil {
				writeError(w, r, logger, err)
				return
			}
			if !claimed {
				switch {
				case existing.RequestHash != rec.RequestHash:
					writeErrorCode(w, http.StatusUnprocessableEntity, "idempotency_key_reused",
						"Idempotency-Key was already used with a different request body")
				case !existing.Completed:
					writeErrorCode(w, http.StatusConflict, "request_in_progress",
						"a request with this Idempotency-Key is still being processed")
				default:
					replay(w, existing)
				}
				return
			}

			cw := &captureWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(cw, r)

			// Finish bookkeeping even if the client has gone away.
			ctx := context.WithoutCancel(r.Context())
			if cw.status >= 500 {
				err = store.Release(ctx, rec.Scope, rec.Key)
			} else {
				err = store.Complete(ctx, rec.Scope, rec.Key, cw.status, cw.Header().Get("Location"), cw.body.Bytes())
			}
			if err != nil {
				logger.ErrorContext(ctx, "idempotency bookkeeping failed", "error", err)
			}
		})
	}
}

func replay(w http.ResponseWriter, rec *port.IdempotencyRecord) {
	if rec.Location != "" {
		w.Header().Set("Location", rec.Location)
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Idempotent-Replayed", "true")
	w.WriteHeader(rec.StatusCode)
	_, _ = w.Write(rec.Body)
}

// captureWriter passes the response through while keeping a copy of the
// status and body for storage.
type captureWriter struct {
	http.ResponseWriter
	status int
	body   bytes.Buffer
}

func (c *captureWriter) WriteHeader(code int) {
	c.status = code
	c.ResponseWriter.WriteHeader(code)
}

func (c *captureWriter) Write(p []byte) (int, error) {
	c.body.Write(p)
	return c.ResponseWriter.Write(p)
}

func (c *captureWriter) Unwrap() http.ResponseWriter { return c.ResponseWriter }
