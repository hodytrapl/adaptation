package middleware

import (
	"errors"
	"net/http"
	"strings"

	"task-manager/internal/authz"
)

type Verifier = authz.Verifier

var (
	WithClaims      = authz.WithClaims
	ClaimsFrom      = authz.ClaimsFrom
	ErrInvalidToken = authz.ErrInvalidToken
)

func AuthMiddleware(v authz.Verifier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw := extractBearer(r)
			if raw == "" {
				WriteError(w, r, http.StatusUnauthorized, "unauthorized",
					"Missing or malformed Authorization header", nil)
				return
			}
			claims, err := v.VerifyAccess(raw)
			if err != nil {
				if errors.Is(err, authz.ErrInvalidToken) {
					WriteError(w, r, http.StatusUnauthorized, "invalid_token",
						"Invalid or expired access token", nil)
					return
				}
				WriteError(w, r, http.StatusInternalServerError, "internal", "Auth check failed", nil)
				return
			}
			next.ServeHTTP(w, r.WithContext(authz.WithClaims(r.Context(), claims)))
		})
	}
}

func OptionalAuthMiddleware(v authz.Verifier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw := extractBearer(r)
			if raw == "" {
				next.ServeHTTP(w, r)
				return
			}
			claims, err := v.VerifyAccess(raw)
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}
			next.ServeHTTP(w, r.WithContext(authz.WithClaims(r.Context(), claims)))
		})
	}
}

func RequireScope(scope string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := authz.ClaimsFrom(r.Context())
			if !ok {
				WriteError(w, r, http.StatusUnauthorized, "unauthorized", "No auth context", nil)
				return
			}
			if !claims.HasScope(scope) {
				WriteError(w, r, http.StatusForbidden, "forbidden",
					"Insufficient scope", map[string]any{"required": scope})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func RequireRole(role string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := authz.ClaimsFrom(r.Context())
			if !ok {
				WriteError(w, r, http.StatusUnauthorized, "unauthorized", "No auth context", nil)
				return
			}
			if claims.Role != role {
				WriteError(w, r, http.StatusForbidden, "forbidden", "Insufficient role",
					map[string]any{"required": role})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func extractBearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if !strings.HasPrefix(h, prefix) {
		return ""
	}
	return strings.TrimSpace(h[len(prefix):])
}