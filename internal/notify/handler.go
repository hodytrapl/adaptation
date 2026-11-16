package notify

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"time"

	appMiddleware "task-manager/internal/middleware"

	"github.com/go-chi/chi/v5"
)

type Handler struct {
	svc          *Service
	authVerifier appMiddleware.Verifier
}

func NewHandler(svc *Service, verifier appMiddleware.Verifier) *Handler {
	return &Handler{svc: svc, authVerifier: verifier}
}

// Router — БЕЗ префикса, монтируется как
//
//	r.Mount("/api/v1/notifications", notifyHandler.Router())
//
// Все роуты требуют auth: уведомления — приватные.
func (h *Handler) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(appMiddleware.JSONHeaderMiddleware)

	r.NotFound(func(w http.ResponseWriter, req *http.Request) {
		appMiddleware.WriteError(w, req, http.StatusNotFound, "not_found", "Route not found", nil)
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, req *http.Request) {
		appMiddleware.WriteError(w, req, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", nil)
	})

	r.Use(appMiddleware.RequestTimeoutMiddleware(2 * time.Second))
	r.Use(appMiddleware.AuthMiddleware(h.authVerifier))

	r.Get("/", h.list)
	r.Patch("/{id}/read", h.markRead)

	return r
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	claims, ok := appMiddleware.ClaimsFrom(ctx)
	if !ok {
		appMiddleware.WriteError(w, r, http.StatusUnauthorized, "unauthorized", "No auth context", nil)
		return
	}

	q := r.URL.Query()
	f := ListFilter{
		UserID: claims.UserID,
		Unread: q.Get("unread") == "true",
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

func (h *Handler) markRead(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	claims, ok := appMiddleware.ClaimsFrom(ctx)
	if !ok {
		appMiddleware.WriteError(w, r, http.StatusUnauthorized, "unauthorized", "No auth context", nil)
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		appMiddleware.WriteError(w, r, http.StatusBadRequest, "bad_request", "Invalid ID",
			map[string]any{"id": idStr})
		return
	}

	n, err := h.svc.MarkRead(ctx, id, claims.UserID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			// Чужое и несуществующее не различаем — защита от IDOR-энумерации.
			appMiddleware.WriteError(w, r, http.StatusNotFound, "not_found", "Notification not found", nil)
			return
		}
		h.failInternal(w, r, "mark_read", err)
		return
	}
	_ = json.NewEncoder(w).Encode(n)
}

func (h *Handler) failInternal(w http.ResponseWriter, r *http.Request, op string, err error) {
	log.Printf("request_id=%s notify %s error: %v", appMiddleware.GetRequestID(r.Context()), op, err)
	appMiddleware.WriteError(w, r, http.StatusInternalServerError, "internal", "Operation failed", nil)
}

// убеждаемся, что context импортирован (для будущих расширений)
var _ = context.Background