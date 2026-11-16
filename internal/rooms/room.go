// Package rooms — модуль «Карта аудиторий» (таблица map_build).
package rooms

import "time"

// AvailabilityStatus — значение статуса.
type AvailabilityStatus string

const (
	StatusFree AvailabilityStatus = "free"
	StatusBusy AvailabilityStatus = "busy"
)

// Room — доменная модель. Один-в-один с map_build.
//
// coordinates POINT в БД разворачиваем в Longitude/Latitude.
// Формат JSON — WGS84: {lon, lat} → конвенция GeoJSON / leaflet.
type Room struct {
	ID          int64     `json:"id"`
	Number      string    `json:"number"`
	Floor       int       `json:"floor"`
	Building    string    `json:"building"`
	Longitude   float64   `json:"longitude"`
	Latitude    float64   `json:"latitude"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
}

// Availability — статус аудитории на конкретную дату.
// Date — строка YYYY-MM-DD (не time.Time), чтобы JSON-контракт был однозначным.
type Availability struct {
	RoomID    int64              `json:"room_id"`
	Date      string             `json:"date"`   // YYYY-MM-DD
	Status    AvailabilityStatus `json:"status"` // free | busy
	UpdatedAt time.Time          `json:"updated_at"`
}

// CreateRoomRequest — DTO POST /api/v1/rooms.
//
// Longitude/Latitude НЕ помечены required, потому что 0 — валидная координата
// по validator. Ограничиваем диапазоном. Проверку «это точно не пустышка»
// оставляем на уровне UI/соглашения (0,0 — Атлантика, для аудитории невозможно).
type CreateRoomRequest struct {
	Number      string  `json:"number"      validate:"required,max=50"`
	Floor       int     `json:"floor"       validate:"min=0,max=200"`
	Building    string  `json:"building"    validate:"required,max=100"`
	Longitude   float64 `json:"longitude"   validate:"min=-180,max=180"`
	Latitude    float64 `json:"latitude"    validate:"min=-90,max=90"`
	Description string  `json:"description" validate:"max=5000"`
}

// UpdateRoomRequest — PATCH /api/v1/rooms/{id}. Указатели — «не передано vs 0».
type UpdateRoomRequest struct {
	Number      *string  `json:"number"      validate:"omitempty,max=50"`
	Floor       *int     `json:"floor"       validate:"omitempty,min=0,max=200"`
	Building    *string  `json:"building"    validate:"omitempty,max=100"`
	Longitude   *float64 `json:"longitude"   validate:"omitempty,min=-180,max=180"`
	Latitude    *float64 `json:"latitude"    validate:"omitempty,min=-90,max=90"`
	Description *string  `json:"description" validate:"omitempty,max=5000"`
}

// UpdateAvailabilityRequest — PATCH /api/v1/rooms/{id}/availability.
// Date — YYYY-MM-DD, парсится в handler.
type UpdateAvailabilityRequest struct {
	Date   string             `json:"date"   validate:"required"`
	Status AvailabilityStatus `json:"status" validate:"required,oneof=free busy"`
}

// ListFilter — фильтры для GET /api/v1/rooms.
type ListFilter struct {
	Building string
	Floor    *int
	Limit    int
	Offset   int
}

type ListResponse struct {
	Items  []Room `json:"items"`
	Total  int    `json:"total"`
	Limit  int    `json:"limit"`
	Offset int    `json:"offset"`
}
