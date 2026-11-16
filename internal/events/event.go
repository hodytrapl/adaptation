// Package events — модуль «Мероприятия».
package events

import "time"

// Event — доменная модель. Один-в-один с таблицей events.
//
// DurationSeconds — секунды. В БД колонка INTERVAL, но наружу и в Go
// «сколько длится» удобнее числом секунд: не нужен pgtype.Interval,
// не нужен custom JSON-marshal, легко тестируется.
type Event struct {
	ID              int64     `json:"id"`
	Title           string    `json:"title"`
	Description     string    `json:"description"`
	DateStarted     time.Time `json:"date_started"`
	DurationSeconds int64     `json:"duration_seconds"`
	Place           string    `json:"place"`
	CreatedAt       time.Time `json:"created_at"`
}

// CreateEventRequest — DTO для POST /api/v1/events.
type CreateEventRequest struct {
	Title           string    `json:"title"            validate:"required,max=500"`
	Description     string    `json:"description"      validate:"max=5000"`
	DateStarted     time.Time `json:"date_started"     validate:"required"`
	DurationSeconds int64     `json:"duration_seconds" validate:"min=0,max=31536000"` // до 1 года
	Place           string    `json:"place"            validate:"max=500"`
}

// UpdateEventRequest — DTO для PATCH /api/v1/events/{id}.
// Указатели, чтобы отличать «поле не передано» от «передано пустое/ноль».
type UpdateEventRequest struct {
	Title           *string    `json:"title"            validate:"omitempty,max=500"`
	Description     *string    `json:"description"      validate:"omitempty,max=5000"`
	DateStarted     *time.Time `json:"date_started"     validate:"omitempty"`
	DurationSeconds *int64     `json:"duration_seconds" validate:"omitempty,min=0,max=31536000"`
	Place           *string    `json:"place"            validate:"omitempty,max=500"`
}

// ListFilter — фильтры для GET /api/v1/events.
//   From/To — по date_started (включительно); нулевое значение = без ограничения.
type ListFilter struct {
	From   time.Time
	To     time.Time
	Limit  int
	Offset int
}

// ListResponse — единый формат списочного ответа (items/total/пагинация).
type ListResponse struct {
	Items  []Event `json:"items"`
	Total  int     `json:"total"`
	Limit  int     `json:"limit"`
	Offset int     `json:"offset"`
}