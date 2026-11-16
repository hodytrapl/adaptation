package notify

import (
	"context"

	"task-manager/internal/qa"
	"task-manager/internal/reviews"
)

// QAAdapter реализует qa.Notifier поверх Service.
//
// Уведомляем ТОЛЬКО модератора, который давал ответ
// (q.ModeratorID). Если ответа не было (например, reject до answer) — тихо пропускаем.
type QAAdapter struct{ svc *Service }

func NewQAAdapter(svc *Service) *QAAdapter { return &QAAdapter{svc: svc} }

func (a *QAAdapter) QAPublished(ctx context.Context, q *qa.QA) error {
	if q == nil || q.ModeratorID == nil {
		return nil
	}
	return a.svc.Notify(ctx, *q.ModeratorID, KindQAPublished, map[string]any{
		"qa_id":  q.ID,
		"title":  q.Title,
		"status": "published",
	})
}

func (a *QAAdapter) QARejected(ctx context.Context, q *qa.QA) error {
	if q == nil || q.ModeratorID == nil {
		return nil
	}
	return a.svc.Notify(ctx, *q.ModeratorID, KindQARejected, map[string]any{
		"qa_id":   q.ID,
		"title":   q.Title,
		"comment": q.ReviewComment,
		"status":  "needs_revision",
	})
}

// ReviewsAdapter реализует reviews.Notifier поверх Service.
//
// ReviewCreated — уведомляем админов/модераторов, но у нас нет списка
// «кто именно». Решение: шлём системное уведомление одному «дежурному» —
// в учебном проекте это sender_id отзыва не нужен. Для реальной задачи
// нужна таблица role_notification_targets или Redis-подписка.
//
// Поэтому здесь — только уведомление автору о просрочке (ReviewStale),
// адресное. ReviewCreated — no-op, потому что политика «кому» — вне scope
// модуля. Легко расширить, добавив SELECT user_id FROM users WHERE role_id=...
type ReviewsAdapter struct{ svc *Service }

func NewReviewsAdapter(svc *Service) *ReviewsAdapter { return &ReviewsAdapter{svc: svc} }

func (a *ReviewsAdapter) ReviewCreated(_ context.Context, _ *reviews.Review) error {
	// no-op: адресат не определён. См. комментарий выше.
	return nil
}

func (a *ReviewsAdapter) ReviewStale(ctx context.Context, rv *reviews.Review) error {
	if rv == nil || rv.SenderID == 0 {
		return nil
	}
	return a.svc.Notify(ctx, rv.SenderID, KindReviewStale, map[string]any{
		"review_id":  rv.ID,
		"title":      rv.Title,
		"created_at": rv.CreatedAt,
	})
}