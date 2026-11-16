package qa

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
//	r.Mount("/api/v1/qa", qaHandler.Router())
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

	// Публичные: список опубликованных + создание вопроса.
	// OptionalAuth: если клиент прислал Bearer — sender_id берём из claims, иначе из тела/nil.
	r.With(appMiddleware.OptionalAuthMiddleware(h.authVerifier)).Post("/", h.create)
	r.Get("/", h.list)
	r.Get("/{id}", h.getPublic)

	// Модератор: ответ → pending_review.
	r.Group(func(r chi.Router) {
		r.Use(appMiddleware.AuthMiddleware(h.authVerifier))
		r.Use(appMiddleware.RequireScope("qa:write"))

		r.Get("/moderation/mine", h.listMine)
		r.Post("/{id}/answer", h.answer)
	})

	// Админ: approve / reject + очередь pending_review.
	r.Group(func(r chi.Router) {
		r.Use(appMiddleware.AuthMiddleware(h.authVerifier))
		r.Use(appMiddleware.RequireScope("qa:admin"))

		r.Get("/moderation/pending", h.listPending)
		r.Post("/{id}/approve", h.approve)
		r.Post("/{id}/reject", h.reject)
	})

	return r
}

// ---- public ----

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req CreateQARequest
	if err := decodeJSONStrict(r, &req); err != nil {
		h.writeDecodeError(w, r, err)
		return
	}
	if err := h.validate.Struct(req); err != nil {
		appMiddleware.WriteError(w, r, http.StatusBadRequest, "validation_error", "Validation failed",
			validationDetails(err))
		return
	}

	// Если клиент авторизован — sender_id берём из JWT, тело игнорируем.
	if claims, ok := appMiddleware.ClaimsFrom(ctx); ok && claims != nil {
		uid := claims.UserID
		req.SenderID = &uid
	}

	created, err := h.svc.Create(ctx, req)
	if err != nil {
		if h.handleContextError(w, r, err) {
			return
		}
		log.Printf("request_id=%s create qa error: %v", appMiddleware.GetRequestID(ctx), err)
		appMiddleware.WriteError(w, r, http.StatusInternalServerError, "internal", "Failed to create question", nil)
		return
	}
	w.Header().Set("Location", fmt.Sprintf("/api/v1/qa/%d", created.ID))
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(created)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query()

	f := ListFilter{Tag: q.Get("tag")}
	if v := q.Get("type_forum_id"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			f.TypeForumID = n
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

	page, err := h.svc.PublicList(ctx, f)
	if err != nil {
		if h.handleContextError(w, r, err) {
			return
		}
		log.Printf("request_id=%s list qa error: %v", appMiddleware.GetRequestID(ctx), err)
		appMiddleware.WriteError(w, r, http.StatusInternalServerError, "internal", "Failed to list questions", nil)
		return
	}
	_ = json.NewEncoder(w).Encode(page)
}

func (h *Handler) getPublic(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, ok := h.parseID(w, r)
	if !ok {
		return
	}
	q, err := h.svc.GetPublic(ctx, id)
	if err != nil {
		if h.handleServiceError(w, r, err) {
			return
		}
		log.Printf("request_id=%s get qa error: %v", appMiddleware.GetRequestID(ctx), err)
		appMiddleware.WriteError(w, r, http.StatusInternalServerError, "internal", "Failed to get question", nil)
		return
	}
	_ = json.NewEncoder(w).Encode(q)
}

// ---- moderator ----

func (h *Handler) listMine(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	claims, _ := appMiddleware.ClaimsFrom(ctx)

	// Очередь "новых" — без ответа, любая, кто первый возьмёт.
	queuedPage, err := h.svc.ModerationList(ctx, ModerationFilter{
		StatusID: StatusNew,
		Limit:    50,
	})
	if err != nil {
		h.failInternal(w, r, "list new queue", err)
		return
	}

	// Свои "needs_revision" — после reject админом.
	minePage, err := h.svc.ModerationList(ctx, ModerationFilter{
		StatusID:    StatusNeedsRevision,
		ModeratorID: claims.UserID,
		Limit:       50,
	})
	if err != nil {
		h.failInternal(w, r, "list my revisions", err)
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]any{
		"new":            queuedPage.Items,
		"new_total":      queuedPage.Total,
		"needs_revision": minePage.Items,
		"revision_total": minePage.Total,
	})
}

func (h *Handler) answer(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, ok := h.parseID(w, r)
	if !ok {
		return
	}

	var req AnswerQARequest
	if err := decodeJSONStrict(r, &req); err != nil {
		h.writeDecodeError(w, r, err)
		return
	}
	if err := h.validate.Struct(req); err != nil {
		appMiddleware.WriteError(w, r, http.StatusBadRequest, "validation_error", "Validation failed",
			validationDetails(err))
		return
	}

	claims, _ := appMiddleware.ClaimsFrom(ctx)
	updated, err := h.svc.Answer(ctx, id, claims.UserID, req.Answer)
	if err != nil {
		if errors.Is(err, ErrInvalidStatus) {
			appMiddleware.WriteError(w, r, http.StatusConflict, "invalid_status",
				"Question is not in a state that can be answered", nil)
			return
		}
		if h.handleServiceError(w, r, err) {
			return
		}
		h.failInternal(w, r, "answer", err)
		return
	}
	_ = json.NewEncoder(w).Encode(updated)
}

// ---- admin ----

func (h *Handler) listPending(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	limit, offset := h.pageParams(r)
	page, err := h.svc.ModerationList(ctx, ModerationFilter{
		StatusID: StatusPendingReview,
		Limit:    limit,
		Offset:   offset,
	})
	if err != nil {
		h.failInternal(w, r, "list pending", err)
		return
	}
	_ = json.NewEncoder(w).Encode(page)
}

func (h *Handler) approve(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, ok := h.parseID(w, r)
	if !ok {
		return
	}

	claims, _ := appMiddleware.ClaimsFrom(ctx)
	updated, err := h.svc.Approve(ctx, id, claims.UserID)
	if err != nil {
		if errors.Is(err, ErrInvalidStatus) {
			appMiddleware.WriteError(w, r, http.StatusConflict, "invalid_status",
				"Only pending_review questions can be published", nil)
			return
		}
		if h.handleServiceError(w, r, err) {
			return
		}
		h.failInternal(w, r, "approve", err)
		return
	}
	_ = json.NewEncoder(w).Encode(updated)
}

func (h *Handler) reject(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, ok := h.parseID(w, r)
	if !ok {
		return
	}

	var req RejectQARequest
	if err := decodeJSONStrict(r, &req); err != nil {
		h.writeDecodeError(w, r, err)
		return
	}
	if err := h.validate.Struct(req); err != nil {
		appMiddleware.WriteError(w, r, http.StatusBadRequest, "validation_error", "Validation failed",
			validationDetails(err))
		return
	}

	claims, _ := appMiddleware.ClaimsFrom(ctx)
	updated, err := h.svc.Reject(ctx, id, claims.UserID, req.Comment)
	if err != nil {
		if errors.Is(err, ErrInvalidStatus) {
			appMiddleware.WriteError(w, r, http.StatusConflict, "invalid_status",
				"Only pending_review questions can be rejected", nil)
			return
		}
		if h.handleServiceError(w, r, err) {
			return
		}
		h.failInternal(w, r, "reject", err)
		return
	}
	_ = json.NewEncoder(w).Encode(updated)
}

// ---- helpers ----

func (h *Handler) pageParams(r *http.Request) (limit, offset int) {
	q := r.URL.Query()
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	if v := q.Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			offset = n
		}
	}
	return
}

func (h *Handler) failInternal(w http.ResponseWriter, r *http.Request, op string, err error) {
	log.Printf("request_id=%s qa %s error: %v", appMiddleware.GetRequestID(r.Context()), op, err)
	appMiddleware.WriteError(w, r, http.StatusInternalServerError, "internal", "Operation failed", nil)
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
		appMiddleware.WriteError(w, r, http.StatusNotFound, "not_found", "Question not found", nil)
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