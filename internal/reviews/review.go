// Package reviews — модуль «Отзывы и предложения».
//
// HTTP-слой (handler), бизнес-логика (service), хранение в PostgreSQL (storage).
package reviews

import "time"

// Well-known status IDs — совпадают с 0002_seed_lookup миграцией.
// Если поменяешь порядок в сиде — обнови и здесь, иначе переходы статусов поедут.
const (
	StatusNew        int64 = 1
	StatusInProgress int64 = 2
	StatusResolved   int64 = 3
	StatusRejected   int64 = 4
	StatusClosed     int64 = 5
)

// StatusTransitions — state machine смены статуса.
// Ключ — текущий статус, значение — допустимые следующие.
//
//	new         → in_progress | rejected
//	in_progress → resolved    | rejected
//	resolved / rejected / closed — терминальные
var StatusTransitions = map[int64][]int64{
	StatusNew:        {StatusInProgress, StatusRejected},
	StatusInProgress: {StatusResolved, StatusRejected},
	StatusResolved:   {},
	StatusRejected:   {},
	StatusClosed:     {},
}

// CanTransition проверяет, разрешён ли переход from → to.
func CanTransition(from, to int64) bool {
	if from == to {
		return false // no-op переходы запрещены: пусть клиент не "пишет в пустоту"
	}
	for _, next := range StatusTransitions[from] {
		if next == to {
			return true
		}
	}
	return false
}

// Review — доменная модель. Один-в-один с таблицей reviews.
type Review struct {
	ID          int64     `json:"id"`
	KeyID       int64     `json:"key_id"`
	SenderID    int64     `json:"sender_id"`
	TypeForumID int64     `json:"type_forum_id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Tags        []string  `json:"tags"`
	StatusID    int64     `json:"status_id"`
	CreatedAt   time.Time `json:"created_at"`
}

// CreateReviewRequest — DTO для POST /api/v1/reviews.
//
// TODO: когда появится auth-слой, sender_id надо брать из JWT
// (r.Context() → claims.sub), а из тела запроса удалить.
// Сейчас оставлен в теле, чтобы можно было тестировать до auth.
type CreateReviewRequest struct {
	KeyID       int64    `json:"key_id"        validate:"required,min=1"`
	SenderID    int64    `json:"sender_id"     validate:"required,min=1"`
	TypeForumID int64    `json:"type_forum_id" validate:"required,min=1"`
	Title       string   `json:"title"         validate:"required,max=500"`
	Description string   `json:"description"   validate:"max=5000"`
	Tags        []string `json:"tags"          validate:"max=20,dive,max=50"`
}

// UpdateReviewRequest — DTO для PATCH /api/v1/reviews/{id}.
// Указатели, чтобы отличать "поле не передано" от "передано пустое".
type UpdateReviewRequest struct {
	Title       *string   `json:"title"       validate:"omitempty,max=500"`
	Description *string   `json:"description" validate:"omitempty,max=5000"`
	Tags        *[]string `json:"tags"        validate:"omitempty,max=20,dive,max=50"`
}

// ChangeStatusRequest — DTO для POST /api/v1/reviews/{id}/status.
type ChangeStatusRequest struct {
	StatusID int64 `json:"status_id" validate:"required,min=1"`
}

// ListFilter — фильтры для GET /api/v1/reviews.
type ListFilter struct {
	StatusID    int64
	SenderID    int64
	TypeForumID int64
	Tag         string
	Limit       int
	Offset      int
}

// ListResponse — обёртка для списочного ответа: items + total + пагинация.
type ListResponse struct {
	Items  []Review `json:"items"`
	Total  int      `json:"total"`
	Limit  int      `json:"limit"`
	Offset int      `json:"offset"`
}