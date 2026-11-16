// Package qa — модуль «Вопрос-ответ».
package qa

import "time"

// Well-known status IDs — совпадают с миграцией 0002 + 0008.
// ВАЖНО: StatusNew пересекается с reviews.StatusNew — это ОК, у них разные таблицы.
const (
	StatusNew           int64 = 1
	StatusPendingReview int64 = 6
	StatusPublished     int64 = 7
	StatusNeedsRevision int64 = 8
)

// Transition разрешена? — state machine workflow.
func CanTransition(from, to int64) bool {
	switch from {
	case StatusNew:
		return to == StatusPendingReview
	case StatusPendingReview:
		return to == StatusPublished || to == StatusNeedsRevision
	case StatusNeedsRevision:
		return to == StatusPendingReview
	default:
		return false
	}
}

// QA — доменная модель.
//
// SenderID, ModeratorID, ReviewerID — указатели, потому что:
//   - SenderID nil → вопрос от гостя;
//   - ModeratorID nil → ответа ещё нет;
//   - ReviewerID nil → ревью ещё не было.
type QA struct {
	ID          int64     `json:"id"`
	KeyID       int64     `json:"key_id"`
	SenderID    *int64    `json:"sender_id,omitempty"`
	TypeForumID int64     `json:"type_forum_id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Tags        []string  `json:"tags"`
	StatusID    int64     `json:"status_id"`
	CreatedAt   time.Time `json:"created_at"`

	// Ответ модератора
	ModeratorID *int64     `json:"moderator_id,omitempty"`
	Answer      string     `json:"answer"`
	AnsweredAt  *time.Time `json:"answered_at,omitempty"`

	// Ревью админа
	ReviewerID    *int64     `json:"reviewer_id,omitempty"`
	ReviewComment string     `json:"review_comment"`
	ReviewedAt    *time.Time `json:"reviewed_at,omitempty"`
}

// CreateQARequest — POST /api/v1/qa (публично).
//
// SenderID в теле — только fallback, если запрос без auth.
// Если клиент авторизован — handler берёт sender_id из claims, а это поле игнорируется.
type CreateQARequest struct {
	KeyID       int64    `json:"key_id"        validate:"required,min=1"`
	SenderID    *int64   `json:"sender_id"     validate:"omitempty,min=1"`
	TypeForumID int64    `json:"type_forum_id" validate:"omitempty,min=1"`
	Title       string   `json:"title"         validate:"required,max=500"`
	Description string   `json:"description"   validate:"max=10000"`
	Tags        []string `json:"tags"          validate:"max=20,dive,max=50"`
}

// AnswerQARequest — POST /api/v1/qa/{id}/answer (модератор).
type AnswerQARequest struct {
	Answer string `json:"answer" validate:"required,min=1,max=10000"`
}

// RejectQARequest — POST /api/v1/qa/{id}/reject (админ).
type RejectQARequest struct {
	Comment string `json:"comment" validate:"required,min=1,max=2000"`
}

// ListFilter — фильтры для публичного списка.
type ListFilter struct {
	TypeForumID int64
	Tag         string
	Limit       int
	Offset      int
}

// ModerationFilter — фильтры для очередей админа/модератора.
type ModerationFilter struct {
	StatusID    int64 // например, StatusPendingReview для админа
	ModeratorID int64 // для модератора: «мои needs_revision»
	Limit       int
	Offset      int
}

type ListResponse struct {
	Items  []QA `json:"items"`
	Total  int  `json:"total"`
	Limit  int  `json:"limit"`
	Offset int  `json:"offset"`
}