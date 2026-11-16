// Package notify — уведомления: generic-хранилище + adapter'ы под модули.
package notify

import "time"

// Kind* — типы уведомлений. Строкой, чтобы можно было расширять без миграций.
const (
	KindReviewCreated = "review_created"
	KindReviewStale   = "review_stale"
	KindQAPublished   = "qa_published"
	KindQARejected    = "qa_rejected"
	KindSystem        = "system"
)

// Notification — доменная модель. Один-в-один с notifications.
type Notification struct {
	ID        int64          `json:"id"`
	UserID    int64          `json:"user_id"`
	Kind      string         `json:"kind"`
	Payload   map[string]any `json:"payload,omitempty"`
	ReadAt    *time.Time     `json:"read_at,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
}

// ListFilter — фильтры для GET /notifications.
type ListFilter struct {
	UserID int64
	Unread bool
	Limit  int
	Offset int
}

// ListResponse — единый формат списочных ответов.
type ListResponse struct {
	Items  []Notification `json:"items"`
	Total  int            `json:"total"`
	Limit  int            `json:"limit"`
	Offset int            `json:"offset"`
}