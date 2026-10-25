package httpapi

import (
	"bytes"
	"log/slog"
	"net/http"

	"github.com/lnwbigbangsm-ops/backend-golang-coding-test/internal/core/port"
)

// NewRouter wires routes and middleware. Auth endpoints and /healthz are
// public; every /api/v1/users route requires a valid JWT. Both create
// endpoints honour an optional Idempotency-Key header.
func NewRouter(users port.UserService, auth port.AuthService, idem port.IdempotencyStore, logger *slog.Logger) http.Handler {
	h := NewHandler(users, auth, logger)
	protect := Authenticate(auth, logger)
	idempotent := Idempotency(idem, logger)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", h.health)
	mux.Handle("POST /api/v1/auth/register", idempotent(http.HandlerFunc(h.register)))
	mux.HandleFunc("POST /api/v1/auth/login", h.login)

	// Authenticate runs before Idempotency so keys are scoped to the caller.
	mux.Handle("POST /api/v1/users", protect(idempotent(http.HandlerFunc(h.createUser))))
	mux.Handle("GET /api/v1/users", protect(http.HandlerFunc(h.listUsers)))
	mux.Handle("GET /api/v1/users/{id}", protect(http.HandlerFunc(h.getUser)))
	mux.Handle("PATCH /api/v1/users/{id}", protect(http.HandlerFunc(h.updateUser)))
	mux.Handle("DELETE /api/v1/users/{id}", protect(http.HandlerFunc(h.deleteUser)))

	return Chain(jsonFallback(mux), Logging(logger), Recover(logger))
}

// jsonFallback makes unmatched routes use the same JSON error format as the
// rest of the API. ServeMux still decides between 404 and 405 (and computes
// the Allow header); only the plain-text body is replaced.
func jsonFallback(mux *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, pattern := mux.Handler(r); pattern != "" {
			mux.ServeHTTP(w, r)
			return
		}

		probe := &probeWriter{header: http.Header{}, status: http.StatusOK}
		mux.ServeHTTP(probe, r)
		switch probe.status {
		case http.StatusNotFound:
			writeErrorCode(w, http.StatusNotFound, "not_found", "route not found")
		case http.StatusMethodNotAllowed:
			w.Header().Set("Allow", probe.header.Get("Allow"))
			writeErrorCode(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed for this route")
		default:
			// Anything else (such as a path-cleaning redirect) passes through.
			for k, v := range probe.header {
				w.Header()[k] = v
			}
			w.WriteHeader(probe.status)
			_, _ = w.Write(probe.body.Bytes())
		}
	})
}

// probeWriter records a response without sending it.
type probeWriter struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (p *probeWriter) Header() http.Header         { return p.header }
func (p *probeWriter) Write(b []byte) (int, error) { return p.body.Write(b) }
func (p *probeWriter) WriteHeader(code int)        { p.status = code }
