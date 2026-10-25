package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"

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
		writeError(w, r, h.logger, err)
		return
	}
	// actorID is "" on the public register route and the caller on POST /users,
	// which makes the caller the account's creator.
	user, err := create(r.Context(), port.CreateUserInput{
		Name: req.Name, Email: req.Email, Password: req.Password, CreatedBy: actorID(r),
	})
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
		writeError(w, r, h.logger, err)
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

// listUsers serves GET /users?page=1&page_size=20.
func (h *Handler) listUsers(w http.ResponseWriter, r *http.Request) {
	page, err := parsePage(r)
	if err != nil {
		writeError(w, r, h.logger, err)
		return
	}
	res, err := h.users.List(r.Context(), page)
	if err != nil {
		writeError(w, r, h.logger, err)
		return
	}
	resp := listUsersResponse{
		Data: make([]userResponse, len(res.Users)),
		Pagination: pagination{
			Page:       res.Page.Number,
			PageSize:   res.Page.Size,
			TotalItems: res.TotalItems,
			TotalPages: res.Page.TotalPages(res.TotalItems),
		},
	}
	for i, u := range res.Users {
		resp.Data[i] = toUserResponse(u)
	}
	writeJSON(w, http.StatusOK, resp)
}

// parsePage reads the paging query parameters, applying defaults when they
// are absent. Range checks are left to the domain.
func parsePage(r *http.Request) (domain.Page, error) {
	page := domain.Page{Number: 1, Size: domain.DefaultPageSize}
	verr := &domain.ValidationError{}
	q := r.URL.Query()
	if v := q.Get("page"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			verr.Add("page", "must be an integer")
		}
		page.Number = n
	}
	if v := q.Get("page_size"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			verr.Add("page_size", "must be an integer")
		}
		page.Size = n
	}
	return page, verr.OrNil()
}

func (h *Handler) updateUser(w http.ResponseWriter, r *http.Request) {
	var req updateUserRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, h.logger, err)
		return
	}
	user, err := h.users.Update(r.Context(), actorID(r), r.PathValue("id"), domain.UserUpdate(req))
	if err != nil {
		writeError(w, r, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, toUserResponse(user))
}

func (h *Handler) deleteUser(w http.ResponseWriter, r *http.Request) {
	if err := h.users.Delete(r.Context(), actorID(r), r.PathValue("id")); err != nil {
		writeError(w, r, h.logger, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// actorID is the authenticated caller's user ID, or "" if unauthenticated.
func actorID(r *http.Request) string {
	if c, ok := ClaimsFromContext(r.Context()); ok {
		return c.UserID
	}
	return ""
}
