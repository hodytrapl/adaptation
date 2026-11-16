package auth

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"task-manager/internal/authz"

	appMiddleware "task-manager/internal/middleware"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
)

type Handler struct {
	svc      *Service
	validate *validator.Validate
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc, validate: validator.New()}
}

// Router возвращает chi-роутер для /auth.
//
//	POST /auth/login
//	POST /auth/refresh
//	POST /auth/logout
//	GET  /auth/me      (требует AuthMiddleware — навешивается в main)
func (h *Handler) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(appMiddleware.JSONHeaderMiddleware)

	r.NotFound(func(w http.ResponseWriter, req *http.Request) {
		appMiddleware.WriteError(w, req, http.StatusNotFound, "not_found", "Route not found", nil)
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, req *http.Request) {
		appMiddleware.WriteError(w, req, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", nil)
	})

	r.Use(appMiddleware.RequestTimeoutMiddleware(5 * time.Second))
	r.Use(appMiddleware.BodyLimitMiddleware(1 << 16))

	r.Post("/login", h.login)
	r.Post("/refresh", h.refresh)
	r.Post("/logout", h.logout)

	// Используем h.svc напрямую, так как он уже есть в структуре Handler
	r.With(appMiddleware.AuthMiddleware(h.svc)).Get("/me", h.me)

	return r
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req LoginRequest
	if err := decodeJSONStrict(r, &req); err != nil {
		h.writeDecodeError(w, r, err)
		return
	}
	if err := h.validate.Struct(req); err != nil {
		appMiddleware.WriteError(w, r, http.StatusBadRequest, "validation_error", "Validation failed",
			validationDetails(err))
		return
	}

	pair, err := h.svc.Login(ctx, req, r.UserAgent(), clientIP(r))
	if err != nil {
		if errors.Is(err, ErrInvalidCredentials) {
			// 401 без подробностей — не даём оракула "логин существует?"
			appMiddleware.WriteError(w, r, http.StatusUnauthorized, "invalid_credentials",
				"Invalid login or password", nil)
			return
		}
		log.Printf("request_id=%s login error: %v", appMiddleware.GetRequestID(ctx), err)
		appMiddleware.WriteError(w, r, http.StatusInternalServerError, "internal", "Login failed", nil)
		return
	}
	_ = json.NewEncoder(w).Encode(pair)
}

func (h *Handler) refresh(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req RefreshRequest
	if err := decodeJSONStrict(r, &req); err != nil {
		h.writeDecodeError(w, r, err)
		return
	}
	if err := h.validate.Struct(req); err != nil {
		appMiddleware.WriteError(w, r, http.StatusBadRequest, "validation_error", "Validation failed",
			validationDetails(err))
		return
	}

	pair, err := h.svc.Refresh(ctx, req.RefreshToken, r.UserAgent(), clientIP(r))
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidToken), errors.Is(err, ErrExpired):
			appMiddleware.WriteError(w, r, http.StatusUnauthorized, "invalid_token", "Invalid or expired refresh token", nil)
		case errors.Is(err, ErrTokenReuse):
			appMiddleware.WriteError(w, r, http.StatusUnauthorized, "token_reuse",
				"Refresh token reuse detected, all sessions revoked", nil)
		default:
			log.Printf("request_id=%s refresh error: %v", appMiddleware.GetRequestID(ctx), err)
			appMiddleware.WriteError(w, r, http.StatusInternalServerError, "internal", "Refresh failed", nil)
		}
		return
	}
	_ = json.NewEncoder(w).Encode(pair)
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req LogoutRequest
	if err := decodeJSONStrict(r, &req); err != nil {
		h.writeDecodeError(w, r, err)
		return
	}
	if err := h.validate.Struct(req); err != nil {
		appMiddleware.WriteError(w, r, http.StatusBadRequest, "validation_error", "Validation failed",
			validationDetails(err))
		return
	}

	// Идемпотентно: даже если токен уже отозван/не существует — 204.
	// Клиенту не нужно различать "уже вышел" и "вышел сейчас".
	if err := h.svc.Logout(ctx, req.RefreshToken); err != nil {
		log.Printf("request_id=%s logout error: %v", appMiddleware.GetRequestID(ctx), err)
		appMiddleware.WriteError(w, r, http.StatusInternalServerError, "internal", "Logout failed", nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	claims, ok := authz.ClaimsFrom(ctx)
	if !ok {
		// Сюда не должны попасть, если AuthMiddleware стоит на /me.
		appMiddleware.WriteError(w, r, http.StatusUnauthorized, "unauthorized", "No auth context", nil)
		return
	}

	me, err := h.svc.Me(ctx, claims.UserID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			appMiddleware.WriteError(w, r, http.StatusUnauthorized, "unauthorized", "User not found", nil)
			return
		}
		log.Printf("request_id=%s me error: %v", appMiddleware.GetRequestID(ctx), err)
		appMiddleware.WriteError(w, r, http.StatusInternalServerError, "internal", "Failed to load profile", nil)
		return
	}
	_ = json.NewEncoder(w).Encode(me)
}

// ---- helpers (те же, что в reviews) ----

func clientIP(r *http.Request) string {
	var rawIP string
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.Index(xff, ","); i >= 0 {
			rawIP = strings.TrimSpace(xff[:i])
		} else {
			rawIP = strings.TrimSpace(xff)
		}
	} else {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			rawIP = r.RemoteAddr
		} else {
			rawIP = host
		}
	}

	// Проверяем, что это валидный IPv4/IPv6 для типа ::inet в PostgreSQL
	if net.ParseIP(rawIP) == nil {
		return ""
	}
	return rawIP
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

func (h *Handler) LoginHandler() http.HandlerFunc   { return h.login }
func (h *Handler) RefreshHandler() http.HandlerFunc { return h.refresh }
func (h *Handler) LogoutHandler() http.HandlerFunc  { return h.logout }
func (h *Handler) MeHandler() http.HandlerFunc      { return h.me }
