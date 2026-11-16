package qa

import (
	"context"
	"errors"
	"log"
	"time"
)

// ErrInvalidStatus — попытка действия не из того статуса.
var ErrInvalidStatus = errors.New("qa: invalid status for this operation")

// Notifier — уведомления. Уведомляем ТОЛЬКО модератора
// (по ТЗ: «опубликовано» / «отклонено, комментарий»).
type Notifier interface {
	QAPublished(ctx context.Context, q *QA) error
	QARejected(ctx context.Context, q *QA) error
}

type Service struct {
	store    Store
	notifier Notifier
}

func NewService(store Store, notifier Notifier) *Service {
	return &Service{store: store, notifier: notifier}
}

// Create — публичное создание. senderID берётся из handler'а:
// если клиент авторизован — claims.UserID, иначе из тела (или nil).
func (s *Service) Create(ctx context.Context, req CreateQARequest) (*QA, error) {
	q := &QA{
		KeyID:       req.KeyID,
		SenderID:    req.SenderID,
		TypeForumID: req.TypeForumID,
		Title:       req.Title,
		Description: req.Description,
		Tags:        req.Tags,
	}
	return s.store.Create(ctx, q)
}

// GetPublic — для публичного GET. Возвращает ErrNotFound, если статус != published
// (не 403 — не раскрываем существование неопубликованных).
func (s *Service) GetPublic(ctx context.Context, id int64) (*QA, error) {
	q, err := s.store.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if q.StatusID != StatusPublished {
		return nil, ErrNotFound
	}
	return q, nil
}

// GetForModeration — для админа/модератора, любой статус.
func (s *Service) GetForModeration(ctx context.Context, id int64) (*QA, error) {
	return s.store.GetByID(ctx, id)
}

func (s *Service) PublicList(ctx context.Context, f ListFilter) (*ListResponse, error) {
	items, total, err := s.store.PublicList(ctx, f)
	if err != nil {
		return nil, err
	}
	return normalizePage(items, total, f.Limit, f.Offset), nil
}

func (s *Service) ModerationList(ctx context.Context, f ModerationFilter) (*ListResponse, error) {
	items, total, err := s.store.ListByStatus(ctx, f)
	if err != nil {
		return nil, err
	}
	return normalizePage(items, total, f.Limit, f.Offset), nil
}

// Answer — модератор даёт/обновляет ответ. Статус → pending_review.
func (s *Service) Answer(ctx context.Context, id, moderatorID int64, answer string) (*QA, error) {
	if answer == "" {
		return nil, errors.New("qa: empty answer")
	}
	q, err := s.store.Answer(ctx, id, moderatorID, answer, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	return q, nil
}

// Approve — админ публикует. Уведомляем модератора.
func (s *Service) Approve(ctx context.Context, id, reviewerID int64) (*QA, error) {
	q, err := s.store.Publish(ctx, id, reviewerID, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	if s.notifier != nil {
		if err := s.notifier.QAPublished(ctx, q); err != nil {
			log.Printf("notify qa_published id=%d: %v", q.ID, err)
		}
	}
	return q, nil
}

// Reject — админ отклоняет с комментарием. Уведомляем модератора с комментарием.
func (s *Service) Reject(ctx context.Context, id, reviewerID int64, comment string) (*QA, error) {
	if comment == "" {
		return nil, errors.New("qa: empty comment")
	}
	q, err := s.store.Reject(ctx, id, reviewerID, comment, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	if s.notifier != nil {
		if err := s.notifier.QARejected(ctx, q); err != nil {
			log.Printf("notify qa_rejected id=%d: %v", q.ID, err)
		}
	}
	return q, nil
}

func normalizePage(items []QA, total, limit, offset int) *ListResponse {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	if items == nil {
		items = []QA{}
	}
	return &ListResponse{Items: items, Total: total, Limit: limit, Offset: offset}
}