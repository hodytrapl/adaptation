package events

import (
	"context"
)

// Service — бизнес-логика модуля мероприятий. Тонкий слой:
// валидация форматов — в handler/validator, а здесь — сборка и нормализация.
type Service struct {
	store Store
}

func NewService(store Store) *Service {
	return &Service{store: store}
}

func (s *Service) Create(ctx context.Context, req CreateEventRequest) (*Event, error) {
	e := &Event{
		Title:           req.Title,
		Description:     req.Description,
		DateStarted:     req.DateStarted,
		DurationSeconds: req.DurationSeconds,
		Place:           req.Place,
	}
	return s.store.Create(ctx, e)
}

func (s *Service) Get(ctx context.Context, id int64) (*Event, error) {
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
		items = []Event{} // JSON: [] вместо null
	}
	return &ListResponse{Items: items, Total: total, Limit: limit, Offset: offset}, nil
}

func (s *Service) Update(ctx context.Context, id int64, req UpdateEventRequest) (*Event, error) {
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
	if req.DateStarted != nil {
		cur.DateStarted = *req.DateStarted
	}
	if req.DurationSeconds != nil {
		cur.DurationSeconds = *req.DurationSeconds
	}
	if req.Place != nil {
		cur.Place = *req.Place
	}
	return s.store.Update(ctx, cur)
}

func (s *Service) Delete(ctx context.Context, id int64) error {
	return s.store.Delete(ctx, id)
}