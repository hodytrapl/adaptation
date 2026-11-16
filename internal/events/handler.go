package events

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
	authVerifier appMiddleware.Verifier // *auth.Service
}

// NewHandler принимает верификатор — как в reviews.NewHandler.
func NewHandler(svc *Service, verifier appMiddleware.Verifier) *Handler {
	return &Handler{
		svc:          svc,
		validate:     validator.New(),
		authVerifier: verifier,
	}
}

// Router возвращает chi-роутер БЕЗ префикса /api/v1/events.
// Монтируется в main:
//
//	r.Mount("/api/v1/events", eventsHandler.Router())
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
	r.Use(appMiddleware.BodyLimitMiddleware(1 << 20))

	// Публичное чтение: список с фильтром по дате и детальная карточка.
	r.Get("/", h.list)
	r.Get("/{id}", h.get)

	// Мутации: admin (events:admin) или moderator (events:write).
	// RequireScope("events:write") пропускает оба — module:admin покрывает write.
	r.Group(func(r chi.Router) {
		r.Use(appMiddleware.AuthMiddleware(h.authVerifier))
		r.Use(appMiddleware.RequireScope("events:write"))

		r.Post("/", h.create)
		r.Patch("/{id}", h.update)
		r.Delete("/{id}", h.delete)
	})

	return r
}

// ---- handlers ----

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query()

	var filter ListFilter

	if v := q.Get("from"); v != "" {
		t, err := parseDateParam(v)
		if err != nil {
			appMiddleware.WriteError(w, r, http.StatusBadRequest, "bad_request",
				"Invalid 'from' (use RFC3339 or YYYY-MM-DD)", map[string]any{"from": v})
			return
		}
		filter.From = t
	}
	if v := q.Get("to"); v != "" {
		t, err := parseDateParam(v)
		if err != nil {
			appMiddleware.WriteError(w, r, http.StatusBadRequest, "bad_request",
				"Invalid 'to' (use RFC3339 or YYYY-MM-DD)", map[string]any{"to": v})
			return
		}
		filter.To = t
	}
	// ?upcoming=true — shorthand для from=now (если from явно не задан).
	if q.Get("upcoming") == "true" && filter.From.IsZero() {
		filter.From = time.Now().UTC()
	}
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			filter.Limit = n
		}
	}
	if v := q.Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			filter.Offset = n
		}
	}

	if !filter.From.IsZero() && !filter.To.IsZero() && filter.To.Before(filter.From) {
		appMiddleware.WriteError(w, r, http.StatusBadRequest, "bad_request",
			"'to' must be greater than or equal to 'from'", nil)
		return
	}

	page, err := h.svc.List(ctx, filter)
	if err != nil {
		if h.handleContextError(w, r, err) {
			return
		}
		log.Printf("request_id=%s list events error: %v", appMiddleware.GetRequestID(ctx), err)
		appMiddleware.WriteError(w, r, http.StatusInternalServerError, "internal", "Failed to list events", nil)
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

	e, err := h.svc.Get(ctx, id)
	if err != nil {
		if h.handleServiceError(w, r, err) {
			return
		}
		log.Printf("request_id=%s get event error: %v", appMiddleware.GetRequestID(ctx), err)
		appMiddleware.WriteError(w, r, http.StatusInternalServerError, "internal", "Failed to get event", nil)
		return
	}
	_ = json.NewEncoder(w).Encode(e)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req CreateEventRequest
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
		if h.handleContextError(w, r, err) {
			return
		}
		log.Printf("request_id=%s create event error: %v", appMiddleware.GetRequestID(ctx), err)
		appMiddleware.WriteError(w, r, http.StatusInternalServerError, "internal", "Failed to create event", nil)
		return
	}

	w.Header().Set("Location", fmt.Sprintf("/api/v1/events/%d", created.ID))
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(created)
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	id, ok := h.parseID(w, r)
	if !ok {
		return
	}

	var req UpdateEventRequest
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
		log.Printf("request_id=%s update event error: %v", appMiddleware.GetRequestID(ctx), err)
		appMiddleware.WriteError(w, r, http.StatusInternalServerError, "internal", "Failed to update event", nil)
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

	if err := h.svc.Delete(ctx, id); err != nil {
		if h.handleServiceError(w, r, err) {
			return
		}
		log.Printf("request_id=%s delete event error: %v", appMiddleware.GetRequestID(ctx), err)
		appMiddleware.WriteError(w, r, http.StatusInternalServerError, "internal", "Failed to delete event", nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- helpers ----

// parseDateParam принимает и RFC3339 (2026-01-01T10:00:00Z),
// и «голую» дату (2026-01-01 → полночь UTC). Второе удобно для UI.
func parseDateParam(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t.UTC(), nil
	}
	return time.Time{}, errors.New("invalid date format")
}

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
	if errors.Is(err, ErrNotFound) {
		appMiddleware.WriteError(w, r, http.StatusNotFound, "not_found", "Event not found", nil)
		return true
	}
	return false
}

func (h *Handler) handleContextError(w http.ResponseWriter, r *http.Request, err error) bool {
	switch {
	case errors.Is(err, context.Canceled):
		return true
	case errors.Is(err, context.DeadlineExceeded):
		appMiddleware.WriteError(w, r, http.StatusRequestTimeout, "timeout", "Request timeout", nil)
		return true
	default:
		return false
	}
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