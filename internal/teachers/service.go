package teachers

import (
	"context"
	"errors"
)

type Service struct {
	store Store
}

func NewService(store Store) *Service { return &Service{store: store} }

func (s *Service) Create(ctx context.Context, req CreateTeacherRequest) (*Teacher, error) {
	t := &Teacher{
		FullName:   req.FullName,
		Department: req.Department,
		Position:   req.Position,
		Contacts:   req.Contacts,
		PhotoURL:   req.PhotoURL,
	}
	if req.RoomID != nil && *req.RoomID > 0 {
		t.Room = &RoomRef{ID: *req.RoomID}
	}
	return s.store.Create(ctx, t)
}

func (s *Service) Get(ctx context.Context, id int64) (*Teacher, error) {
	return s.store.GetByID(ctx, id)
}

func (s *Service) List(ctx context.Context, f ListFilter) (*ListResponse, error) {
	items, total, err := s.store.List(ctx, f)
	if err != nil {
		return nil, err
	}
	limit := f.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	offset := f.Offset
	if offset < 0 {
		offset = 0
	}
	if items == nil {
		items = []Teacher{}
	}
	return &ListResponse{Items: items, Total: total, Limit: limit, Offset: offset}, nil
}

func (s *Service) Update(ctx context.Context, id int64, req UpdateTeacherRequest) (*Teacher, error) {
	cur, err := s.store.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if req.FullName != nil {
		cur.FullName = *req.FullName
	}
	if req.Department != nil {
		cur.Department = *req.Department
	}
	if req.Position != nil {
		cur.Position = *req.Position
	}
	if req.Contacts != nil {
		cur.Contacts = *req.Contacts
	}
	if req.PhotoURL != nil {
		cur.PhotoURL = *req.PhotoURL
	}
	if req.RoomID != nil {
		if *req.RoomID == 0 {
			cur.Room = nil // явный 0 → отвязать
		} else {
			cur.Room = &RoomRef{ID: *req.RoomID}
		}
	}
	return s.store.Update(ctx, cur)
}

func (s *Service) Delete(ctx context.Context, id int64) error {
	return s.store.Delete(ctx, id)
}

// Общие sentinel-ошибки — экспортируем для handler-а.
var _ = errors.New // чтобы не ругался линтер при минимальной версии файла