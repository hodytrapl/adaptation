// Package users — управление пользователями (super_admin only).
package users

import "time"

// User — доменная модель. Один-в-один с users + JOIN roles.name.
// password_hash никогда не сериализуется.
type User struct {
	ID        int64     `json:"id"`
	Login     string    `json:"login"`
	Name      string    `json:"name"`
	RoleID    int64     `json:"role_id"`
	Role      string    `json:"role"`
	Scopes    []string  `json:"scopes"` // users.scopes — поверх role.scopes
	CreatedAt time.Time `json:"created_at"`
}

// Role — элемент справочника roles (для UI назначения).
type Role struct {
	ID     int64    `json:"id"`
	Name   string   `json:"name"`
	Scopes []string `json:"scopes"`
}

// CreateUserRequest — POST /api/v1/users.
type CreateUserRequest struct {
	Login    string   `json:"login"    validate:"required,min=3,max=100"`
	Password string   `json:"password" validate:"required,min=8,max=200"`
	Name     string   `json:"name"     validate:"required,max=255"`
	RoleID   int64    `json:"role_id"  validate:"required,min=1"`
	Scopes   []string `json:"scopes"   validate:"max=50,dive,max=100"`
}

// UpdateUserRequest — PATCH /api/v1/users/{id}.
// login не меняется — это идентификатор. password — перехэшируется.
type UpdateUserRequest struct {
	Password *string   `json:"password" validate:"omitempty,min=8,max=200"`
	Name     *string   `json:"name"     validate:"omitempty,max=255"`
	RoleID   *int64    `json:"role_id"  validate:"omitempty,min=1"`
	Scopes   *[]string `json:"scopes"   validate:"omitempty,max=50,dive,max=100"`
}

type ListFilter struct {
	RoleID int64
	Query  string // ILIKE по login и name
	Limit  int
	Offset int
}

type ListResponse struct {
	Items  []User `json:"items"`
	Total  int    `json:"total"`
	Limit  int    `json:"limit"`
	Offset int    `json:"offset"`
}