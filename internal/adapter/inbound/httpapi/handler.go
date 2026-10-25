package httpapi

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/lnwbigbangsm-ops/backend-golang-coding-test/internal/core/domain"
	"github.com/lnwbigbangsm-ops/backend-golang-coding-test/internal/core/port"
)

// Handler exposes the user and auth use cases over HTTP.
type Handler struct {
	users  port.UserService
	auth   port.AuthService
	logger *slog.Logger
}

// NewHandler builds a Handler.
func NewHandler(users port.UserService, auth port.AuthService, logger *slog.Logger) *Handler {
	return &Handler{users: users, auth: auth, logger: logger}
}

func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	h.createWith(w, r, h.auth.Register)
}

func (h *Handler) createUser(w http.ResponseWriter, r *http.Request) {
	h.createWith(w, r, h.users.Create)
}

func (h *Handler) createWith(w http.ResponseWriter, r *http.Request, create func(context.Context, port.CreateUserInput) (*domain.User, error)) {
	var req createUserRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeErrorCode(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	user, err := create(r.Context(), port.CreateUserInput(req))
	if err != nil {
		writeError(w, r, h.logger, err)
		return
	}
	w.Header().Set("Location", "/api/v1/users/"+user.ID)
	writeJSON(w, http.StatusCreated, toUserResponse(user))
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeErrorCode(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	res, err := h.auth.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		writeError(w, r, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, loginResponse{
		AccessToken: res.AccessToken,
		TokenType:   res.TokenType,
		ExpiresAt:   res.ExpiresAt,
		User:        toUserResponse(res.User),
	})
}

func (h *Handler) getUser(w http.ResponseWriter, r *http.Request) {
	user, err := h.users.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, r, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, toUserResponse(user))
}

func (h *Handler) listUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.users.List(r.Context())
	if err != nil {
		writeError(w, r, h.logger, err)
		return
	}
	resp := listUsersResponse{Data: make([]userResponse, len(users)), Total: len(users)}
	for i, u := range users {
		resp.Data[i] = toUserResponse(u)
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) updateUser(w http.ResponseWriter, r *http.Request) {
	var req updateUserRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeErrorCode(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	user, err := h.users.Update(r.Context(), r.PathValue("id"), domain.UserUpdate(req))
	if err != nil {
		writeError(w, r, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, toUserResponse(user))
}

func (h *Handler) deleteUser(w http.ResponseWriter, r *http.Request) {
	if err := h.users.Delete(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, r, h.logger, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
