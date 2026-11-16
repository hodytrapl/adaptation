// Package teachers — модуль «Преподаватели».
package teachers

import (
	"time"
)

// Contacts — контакты преподавателя. Хранятся в JSONB.
// Все поля опциональны; валидация — на уровне тегов.
type Contacts struct {
	Phone       string `json:"phone,omitempty"        validate:"omitempty,max=50"`
	Email       string `json:"email,omitempty"        validate:"omitempty,email,max=200"`
	Telegram    string `json:"telegram,omitempty"     validate:"omitempty,max=100"`
	OfficeHours string `json:"office_hours,omitempty" validate:"omitempty,max=500"`
}

// RoomRef — вложенная ссылка на аудиторию. Заполняется LEFT JOIN-ом
// в List/GetByID — фронту не нужно делать второй запрос.
type RoomRef struct {
	ID       int64  `json:"id"`
	Number   string `json:"number"`
	Building string `json:"building"`
}

// Teacher — доменная модель. Один-в-один с teachers.
//
// Room == nil → преподаватель без привязки к кабинету.
type Teacher struct {
	ID         int64     `json:"id"`
	UserID *int64 `json:"user_id,omitempty"`
	FullName   string    `json:"full_name"`
	Department string    `json:"department"`
	Position   string    `json:"position"`
	Contacts   Contacts  `json:"contacts"`
	PhotoURL   string    `json:"photo_url,omitempty"`
	Room       *RoomRef  `json:"room,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

// CreateTeacherRequest — DTO POST /api/v1/teachers.
//
// RoomID — указатель, потому что 0 = «не привязан» (в БД NULL).
type CreateTeacherRequest struct {
	UserID *int64 `json:"user_id" validate:"omitempty,min=1"`
	FullName   string   `json:"full_name"  validate:"required,max=255"`
	Department string   `json:"department" validate:"required,max=255"`
	Position   string   `json:"position"   validate:"required,max=255"`
	Contacts   Contacts `json:"contacts"`
	PhotoURL   string   `json:"photo_url"  validate:"omitempty,url,max=1000"`
	RoomID     *int64   `json:"room_id"    validate:"omitempty,min=1"`
}

// UpdateTeacherRequest — PATCH /api/v1/teachers/{id}.
// Указатели — «не передано» vs «передано пустое». RoomID: явный 0 = «отвязать».
type UpdateTeacherRequest struct {
	FullName   *string   `json:"full_name"  validate:"omitempty,max=255"`
	Department *string   `json:"department" validate:"omitempty,max=255"`
	Position   *string   `json:"position"   validate:"omitempty,max=255"`
	Contacts   *Contacts `json:"contacts"`
	PhotoURL   *string   `json:"photo_url"  validate:"omitempty,url,max=1000"`
	RoomID     *int64    `json:"room_id"` // 0 → отвязать
}

// ListFilter — фильтры для GET /api/v1/teachers.
type ListFilter struct {
	Department string
	RoomID     int64
	Query      string // поиск по full_name (ILIKE)
	Limit      int
	Offset     int
}

type ListResponse struct {
	Items  []Teacher `json:"items"`
	Total  int       `json:"total"`
	Limit  int       `json:"limit"`
	Offset int       `json:"offset"`
}