package reviews

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"
)

// ErrInvalidStatusChange — попытка недопустимого перехода статуса.
var ErrInvalidStatusChange = errors.New("reviews: invalid status transition")

// Store — контракт хранилища. Service не знает, Postgres это или что-то ещё.
// Это даёт возможность юнит-тестить сервис без БД.
type Store interface {
	Create(ctx context.Context, r *Review) (*Review, error)
	GetByID(ctx context.Context, id int64) (*Review, error)
	List(ctx context.Context, f ListFilter) ([]Review, int, error)
	Update(ctx context.Context, r *Review) (*Review, error)
	UpdateStatus(ctx context.Context, id, statusID int64) (*Review, error)
	Delete(ctx context.Context, id int64) error
	FindStale(ctx context.Context, threshold time.Duration) ([]Review, error)
}

// Notifier — уведомления. Может быть nil — тогда уведомления отключены.
// Реализации (email/telegram/log) появятся в internal/notify.
type Notifier interface {
	ReviewCreated(ctx context.Context, r *Review) error
	ReviewStale(ctx context.Context, r *Review) error
}

// Service — бизнес-логика модуля отзывов.
type Service struct {
	store    Store
	notifier Notifier
}

func NewService(store Store, notifier Notifier) *Service {
	return &Service{store: store, notifier: notifier}
}

func (s *Service) Create(ctx context.Context, req CreateReviewRequest) (*Review, error) {
	tags := req.Tags
	if tags == nil {
		tags = []string{}
	}

	r := &Review{
		KeyID:       req.KeyID,
		SenderID:    req.SenderID,
		TypeForumID: req.TypeForumID,
		Title:       req.Title,
		Description: req.Description,
		Tags:        tags,
		StatusID:    StatusNew,
	}
	created, err := s.store.Create(ctx, r)
	if err != nil {
		return nil, err
	}
	if s.notifier != nil {
		if err := s.notifier.ReviewCreated(ctx, created); err != nil {
			log.Printf("notify review_created id=%d: %v", created.ID, err)
		}
	}
	return created, nil
}

func (s *Service) Get(ctx context.Context, id int64) (*Review, error) {
	return s.store.GetByID(ctx, id)
}

func (s *Service) List(ctx context.Context, f ListFilter) (*ListResponse, error) {
	items, total, err := s.store.List(ctx, f)
	if err != nil {
		return nil, err
	}
	limit := f.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	offset := f.Offset
	if offset < 0 {
		offset = 0
	}
	if items == nil {
		items = []Review{} // чтобы JSON был [], а не null
	}
	return &ListResponse{Items: items, Total: total, Limit: limit, Offset: offset}, nil
}

func (s *Service) Update(ctx context.Context, id int64, req UpdateReviewRequest) (*Review, error) {
	cur, err := s.store.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if req.Title != nil {
		cur.Title = *req.Title
	}
	if req.Description != nil {
		cur.Description = *req.Description
	}
	if req.Tags != nil {
		cur.Tags = *req.Tags
	}
	return s.store.Update(ctx, cur)
}

// ChangeStatus — единственный легальный способ менять статус.
// Проверяет переход по StatusTransitions.
func (s *Service) ChangeStatus(ctx context.Context, id, newStatusID int64) (*Review, error) {
	cur, err := s.store.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !CanTransition(cur.StatusID, newStatusID) {
		return nil, fmt.Errorf("%w: %d -> %d", ErrInvalidStatusChange, cur.StatusID, newStatusID)
	}
	return s.store.UpdateStatus(ctx, id, newStatusID)
}

func (s *Service) Delete(ctx context.Context, id int64) error {
	return s.store.Delete(ctx, id)
}

// RunStaleWatcher — фоновый воркер: раз в interval переводит "new"-отзывы
// старше threshold в "in_progress" и шлёт уведомление.
//
// Блокирует горутину до отмены ctx. Запускается из main:
//
//	go svc.RunStaleWatcher(appCtx, time.Hour, 5*24*time.Hour)
//
// Первый прогон — сразу (чтобы не ждать первый тик).
func (s *Service) RunStaleWatcher(ctx context.Context, interval, threshold time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	s.processStale(ctx, threshold)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.processStale(ctx, threshold)
		}
	}
}

func (s *Service) processStale(ctx context.Context, threshold time.Duration) {
	stale, err := s.store.FindStale(ctx, threshold)
	if err != nil {
		log.Printf("stale watcher: find: %v", err)
		return
	}
	for _, r := range stale {
		updated, err := s.store.UpdateStatus(ctx, r.ID, StatusInProgress)
		if err != nil {
			log.Printf("stale watcher: update id=%d: %v", r.ID, err)
			continue
		}
		if s.notifier != nil {
			if err := s.notifier.ReviewStale(ctx, updated); err != nil {
				log.Printf("stale watcher: notify id=%d: %v", updated.ID, err)
			}
		}
	}
}