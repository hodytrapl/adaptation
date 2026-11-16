// Package auth — аутентификация и авторизация: JWT access + opaque refresh.
package auth

import (
	"task-manager/internal/authz"
	"time"
)

// TokenType* — значения claim "typ".
const (
	TypeAccess  = "access"
	TypeRefresh = "refresh"
)

// Role — роль пользователя (из таблицы roles).
type Role struct {
	ID     int64
	Name   string
	Scopes []string
}

// User — пользователь + присоединённая роль.
//
// PasswordHash — bcrypt-хэш из users.password. В JSON НЕ сериализуется
// (нет json-тега → не попадёт в ответ). Мы вообще его наружу не отдаём.
type User struct {
	ID           int64
	Login        string
	Name         string
	PasswordHash string
	Role         Role
	UserScopes   []string // users.scopes (доп. к роли)
}
type Claims = authz.Claims

// EffectiveScopes — union(role.scopes, user.scopes) с дедупликацией.
// Именно это кладём в JWT и в /auth/me.
func (u *User) EffectiveScopes() []string {
	set := make(map[string]struct{}, len(u.Role.Scopes)+len(u.UserScopes))
	for _, s := range u.Role.Scopes {
		if s != "" {
			set[s] = struct{}{}
		}
	}
	for _, s := range u.UserScopes {
		if s != "" {
			set[s] = struct{}{}
		}
	}
	out := make([]string, 0, len(set))
	for s := range set {
		out = append(out, s)
	}
	return out
}

// ---- DTO (HTTP-контракт) ----

type LoginRequest struct {
	Login    string `json:"login"    validate:"required,min=3,max=100"`
	Password string `json:"password" validate:"required,min=1,max=200"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

type LogoutRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

// TokenPair — ответ на login/refresh. Refresh — opaque строка (не JWT).
type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"` // "Bearer"
	ExpiresIn    int64  `json:"expires_in"` // секунды до истечения access
}

// MeResponse — GET /auth/me. Скоупы — эффективные (роль ∪ user).
type MeResponse struct {
	ID        int64    `json:"id"`
	Login     string   `json:"login"`
	Name      string   `json:"name"`
	Role      string   `json:"role"`
	Scopes    []string `json:"scopes"`
	TeacherID *int64   `json:"teacher_id,omitempty"`
}

// RefreshTokenRecord — запись в БД (используется только в storage/service).
type RefreshTokenRecord struct {
	UserID    int64
	Hash      []byte
	ExpiresAt time.Time
}
