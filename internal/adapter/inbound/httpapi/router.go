package httpapi

import (
	"log/slog"
	"net/http"

	"backend-golang-coding-test/internal/core/port"
)

// NewRouter wires routes and middleware. Auth endpoints and /healthz are
// public; every /api/v1/users route requires a valid JWT.
func NewRouter(users port.UserService, auth port.AuthService, logger *slog.Logger) http.Handler {
	h := NewHandler(users, auth, logger)
	protect := Authenticate(auth, logger)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", h.health)
	mux.HandleFunc("POST /api/v1/auth/register", h.register)
	mux.HandleFunc("POST /api/v1/auth/login", h.login)

	mux.Handle("POST /api/v1/users", protect(http.HandlerFunc(h.createUser)))
	mux.Handle("GET /api/v1/users", protect(http.HandlerFunc(h.listUsers)))
	mux.Handle("GET /api/v1/users/{id}", protect(http.HandlerFunc(h.getUser)))
	mux.Handle("PATCH /api/v1/users/{id}", protect(http.HandlerFunc(h.updateUser)))
	mux.Handle("DELETE /api/v1/users/{id}", protect(http.HandlerFunc(h.deleteUser)))
	// Unmatched routes fall through to ServeMux, which answers 404, or 405
	// with an Allow header when the path exists under another method.

	return Chain(mux, Logging(logger), Recover(logger))
}
