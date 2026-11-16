package rooms

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

// Router — БЕЗ префикса /api/v1/rooms, монтируется в main:
//
//	r.Mount("/api/v1/rooms", roomsHandler.Router())
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

	// Публичные чтения.
	r.Get("/", h.list)
	r.Get("/{id}", h.get)
	r.Get("/{id}/availability", h.getAvailability)

	// Мутации: admin (map:admin) или moderator (map:write).
	r.Group(func(r chi.Router) {
		r.Use(appMiddleware.AuthMiddleware(h.authVerifier))
		r.Use(appMiddleware.RequireScope("map:write"))

		r.Post("/", h.create)
		r.Patch("/{id}", h.update)
		r.Delete("/{id}", h.delete)
		r.Patch("/{id}/availability", h.setAvailability)
	})

	return r
}

// ---- handlers ----

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query()

	filter := ListFilter{
		Building: q.Get("building"),
	}
	if v := q.Get("floor"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			filter.Floor = n
		}
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

	page, err := h.svc.List(ctx, filter)
	if err != nil {
		if h.handleContextError(w, r, err) {
			return
		}
		log.Printf("request_id=%s list rooms error: %v", appMiddleware.GetRequestID(ctx), err)
		appMiddleware.WriteError(w, r, http.StatusInternalServerError, "internal", "Failed to list rooms", nil)
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
	room, err := h.svc.Get(ctx, id)
	if err != nil {
		if h.handleServiceError(w, r, err) {
			return
		}
		log.Printf("request_id=%s get room error: %v", appMiddleware.GetRequestID(ctx), err)
		appMiddleware.WriteError(w, r, http.StatusInternalServerError, "internal", "Failed to get room", nil)
		return
	}
	_ = json.NewEncoder(w).Encode(room)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req CreateRoomRequest
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
		log.Printf("request_id=%s create room error: %v", appMiddleware.GetRequestID(ctx), err)
		appMiddleware.WriteError(w, r, http.StatusInternalServerError, "internal", "Failed to create room", nil)
		return
	}
	w.Header().Set("Location", fmt.Sprintf("/api/v1/rooms/%d", created.ID))
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(created)
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, ok := h.parseID(w, r)
	if !ok {
		return
	}

	var req UpdateRoomRequest
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
		log.Printf("request_id=%s update room error: %v", appMiddleware.GetRequestID(ctx), err)
		appMiddleware.WriteError(w, r, http.StatusInternalServerError, "internal", "Failed to update room", nil)
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
		log.Printf("request_id=%s delete room error: %v", appMiddleware.GetRequestID(ctx), err)
		appMiddleware.WriteError(w, r, http.StatusInternalServerError, "internal", "Failed to delete room", nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) getAvailability(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, ok := h.parseID(w, r)
	if !ok {
		return
	}

	// ?date=YYYY-MM-DD; если не задан — сегодня (UTC).
	raw := r.URL.Query().Get("date")
	var date time.Time
	if raw == "" {
		now := time.Now().UTC()
		date = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	} else {
		t, err := time.ParseInLocation("2006-01-02", raw, time.UTC)
		if err != nil {
			appMiddleware.WriteError(w, r, http.StatusBadRequest, "bad_request",
				"Invalid 'date' (expected YYYY-MM-DD)", map[string]any{"date": raw})
			return
		}
		date = t
	}

	a, err := h.svc.GetAvailability(ctx, id, date)
	if err != nil {
		if h.handleServiceError(w, r, err) {
			return
		}
		log.Printf("request_id=%s get availability error: %v", appMiddleware.GetRequestID(ctx), err)
		appMiddleware.WriteError(w, r, http.StatusInternalServerError, "internal", "Failed to get availability", nil)
		return
	}
	_ = json.NewEncoder(w).Encode(a)
}

func (h *Handler) setAvailability(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, ok := h.parseID(w, r)
	if !ok {
		return
	}

	var req UpdateAvailabilityRequest
	if err := decodeJSONStrict(r, &req); err != nil {
		h.writeDecodeError(w, r, err)
		return
	}
	if err := h.validate.Struct(req); err != nil {
		appMiddleware.WriteError(w, r, http.StatusBadRequest, "validation_error", "Validation failed",
			validationDetails(err))
		return
	}

	date, err := time.ParseInLocation("2006-01-02", req.Date, time.UTC)
	if err != nil {
		appMiddleware.WriteError(w, r, http.StatusBadRequest, "bad_request",
			"Invalid 'date' (expected YYYY-MM-DD)", map[string]any{"date": req.Date})
		return
	}

	// updated_by — из JWT, если есть (роут под AuthMiddleware).
	var updatedBy int64
	if claims, ok := appMiddleware.ClaimsFrom(ctx); ok && claims != nil {
		updatedBy = claims.UserID
	}

	a, err := h.svc.SetAvailability(ctx, id, date, req.Status, updatedBy)
	if err != nil {
		if h.handleServiceError(w, r, err) {
			return
		}
		log.Printf("request_id=%s set availability error: %v", appMiddleware.GetRequestID(ctx), err)
		appMiddleware.WriteError(w, r, http.StatusInternalServerError, "internal", "Failed to set availability", nil)
		return
	}
	_ = json.NewEncoder(w).Encode(a)
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
	if errors.Is(err, ErrNotFound) {
		appMiddleware.WriteError(w, r, http.StatusNotFound, "not_found", "Room not found", nil)
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