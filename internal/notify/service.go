package notify

import (
	"context"
)

// Service — фасад уведомлений.
type Service struct {
	store Store
}

func NewService(store Store) *Service { return &Service{store: store} }

// Notify — единая точка входа для других модулей.
// Возвращает ошибку, но вызывающие её логируют best-effort.
func (s *Service) Notify(ctx context.Context, userID int64, kind string, payload map[string]any) error {
	_, err := s.store.Create(ctx, &Notification{
		UserID:  userID,
		Kind:    kind,
		Payload: payload,
	})
	return err
}

func (s *Service) List(ctx context.Context, f ListFilter) (*ListResponse, error) {
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 50
	}
	if f.Offset < 0 {
		f.Offset = 0
	}

	items, total, err := s.store.List(ctx, f)
	if err != nil {
		return nil, err
	}
	if items == nil {
		items = []Notification{}
	}
	return &ListResponse{
		Items:  items,
		Total:  total,
		Limit:  f.Limit,
		Offset: f.Offset,
	}, nil
}

func (s *Service) MarkRead(ctx context.Context, id, userID int64) (*Notification, error) {
	return s.store.MarkRead(ctx, id, userID)
}