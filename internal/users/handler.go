package users

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"

	appMiddleware "task-manager/internal/middleware"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
)

type Handler struct {
	svc          *Service
	validate     *validator.Validate
	authVerifier appMiddleware.Verifier
}

func NewHandler(svc *Service, verifier appMiddleware.Verifier) *Handler {
	return &Handler{
		svc:          svc,
		validate:     validator.New(),
		authVerifier: verifier,
	}
}

// Router — БЕЗ префикса, монтируется как
//
//	r.Mount("/api/v1/users", usersHandler.Router())
//
// Весь модуль закрыт: AuthMiddleware + RequireRole("super_admin").
func (h *Handler) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(appMiddleware.JSONHeaderMiddleware)

	r.NotFound(func(w http.ResponseWriter, req *http.Request) {
		appMiddleware.WriteError(w, req, http.StatusNotFound, "not_found", "Route not found", nil)
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, req *http.Request) {
		appMiddleware.WriteError(w, req, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", nil)
	})

	r.Use(appMiddleware.RequestTimeoutMiddleware(3 * time.Second))
	r.Use(appMiddleware.BodyLimitMiddleware(1 << 20))

	r.Use(appMiddleware.AuthMiddleware(h.authVerifier))
	r.Use(appMiddleware.RequireRole("super_admin"))

	r.Get("/", h.list)
	r.Post("/", h.create)
	r.Get("/roles", h.listRoles) // справочник для UI назначения
	r.Get("/{id}", h.get)
	r.Patch("/{id}", h.update)
	r.Delete("/{id}", h.delete)

	return r
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query()

	f := ListFilter{Query: q.Get("q")}
	if v := q.Get("role_id"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			f.RoleID = n
		}
	}
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			f.Limit = n
		}
	}
	if v := q.Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			f.Offset = n
		}
	}

	page, err := h.svc.List(ctx, f)
	if err != nil {
		h.failInternal(w, r, "list", err)
		return
	}
	_ = json.NewEncoder(w).Encode(page)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, ok := h.parseID(w, r)
	if !ok {
		return
	}
	u, err := h.svc.Get(ctx, id)
	if err != nil {
		if h.handleServiceError(w, r, err) {
			return
		}
		h.failInternal(w, r, "get", err)
		return
	}
	_ = json.NewEncoder(w).Encode(u)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req CreateUserRequest
	if err := decodeJSONStrict(r, &req); err != nil {
		h.writeDecodeError(w, r, err)
		return
	}
	if err := h.validate.Struct(req); err != nil {
		appMiddleware.WriteError(w, r, http.StatusBadRequest, "validation_error", "Validation failed",
			validationDetails(err))
		return
	}

	created, err := h.svc.Create(ctx, req)
	if err != nil {
		if h.handleServiceError(w, r, err) {
			return
		}
		h.failInternal(w, r, "create", err)
		return
	}
	w.Header().Set("Location", fmt.Sprintf("/api/v1/users/%d", created.ID))
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(created)
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, ok := h.parseID(w, r)
	if !ok {
		return
	}

	var req UpdateUserRequest
	if err := decodeJSONStrict(r, &req); err != nil {
		h.writeDecodeError(w, r, err)
		return
	}
	if err := h.validate.Struct(req); err != nil {
		appMiddleware.WriteError(w, r, http.StatusBadRequest, "validation_error", "Validation failed",
			validationDetails(err))
		return
	}

	updated, err := h.svc.Update(ctx, id, req)
	if err != nil {
		if h.handleServiceError(w, r, err) {
			return
		}
		h.failInternal(w, r, "update", err)
		return
	}
	_ = json.NewEncoder(w).Encode(updated)
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, ok := h.parseID(w, r)
	if !ok {
		return
	}

	claims, _ := appMiddleware.ClaimsFrom(ctx)
	err := h.svc.Delete(ctx, id, claims.UserID)
	if err != nil {
		if h.handleServiceError(w, r, err) {
			return
		}
		h.failInternal(w, r, "delete", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) listRoles(w http.ResponseWriter, r *http.Request) {
	roles, err := h.svc.ListRoles(r.Context())
	if err != nil {
		h.failInternal(w, r, "list_roles", err)
		return
	}
	if roles == nil {
		roles = []Role{}
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"items": roles})
}

// ---- helpers ----

func (h *Handler) parseID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		appMiddleware.WriteError(w, r, http.StatusBadRequest, "bad_request", "Invalid ID",
			map[string]any{"id": idStr})
		return 0, false
	}
	return id, true
}

func (h *Handler) handleServiceError(w http.ResponseWriter, r *http.Request, err error) bool {
	switch {
	case errors.Is(err, ErrNotFound):
		appMiddleware.WriteError(w, r, http.StatusNotFound, "not_found", "User not found", nil)
	case errors.Is(err, ErrLoginExists):
		appMiddleware.WriteError(w, r, http.StatusConflict, "login_exists", "Login already exists", nil)
	case errors.Is(err, ErrRoleNotFound):
		appMiddleware.WriteError(w, r, http.StatusBadRequest, "role_not_found", "Role not found", nil)
	case errors.Is(err, ErrCannotDeleteSelf):
		appMiddleware.WriteError(w, r, http.StatusForbidden, "cannot_delete_self",
			"Cannot delete your own account", nil)
	case errors.Is(err, ErrInUse):
		appMiddleware.WriteError(w, r, http.StatusConflict, "user_in_use",
			"User has dependent data and cannot be deleted", nil)
	default:
		return false
	}
	return true
}

func (h *Handler) failInternal(w http.ResponseWriter, r *http.Request, op string, err error) {
	log.Printf("request_id=%s users %s error: %v", appMiddleware.GetRequestID(r.Context()), op, err)
	appMiddleware.WriteError(w, r, http.StatusInternalServerError, "internal", "Operation failed", nil)
}

func decodeJSONStrict(r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return errors.New("request body must contain a single JSON object")
	}
	return nil
}

func (h *Handler) writeDecodeError(w http.ResponseWriter, r *http.Request, err error) {
	var maxErr *http.MaxBytesError
	switch {
	case errors.As(err, &maxErr):
		appMiddleware.WriteError(w, r, http.StatusRequestEntityTooLarge, "payload_too_large",
			"Request body is too large", map[string]any{"limit_bytes": maxErr.Limit})
	case errors.Is(err, io.EOF):
		appMiddleware.WriteError(w, r, http.StatusBadRequest, "bad_request", "Empty request body", nil)
	default:
		appMiddleware.WriteError(w, r, http.StatusBadRequest, "bad_request", "Invalid JSON",
			map[string]any{"error": err.Error()})
	}
}

func validationDetails(err error) any {
	var verrs validator.ValidationErrors
	if !errors.As(err, &verrs) {
		return map[string]any{"error": err.Error()}
	}
	type item struct {
		Field string `json:"field"`
		Rule  string `json:"rule"`
	}
	out := make([]item, 0, len(verrs))
	for _, fe := range verrs {
		out = append(out, item{Field: fe.Field(), Rule: fe.Tag()})
	}
	return out
}

// context нужен только для сигнатуры failInternal — на случай расширения.
var _ = context.Background