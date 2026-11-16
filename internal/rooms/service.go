package rooms

import (
	"context"
	"errors"
	"fmt"
	"time"
)

type Service struct {
	store Store
}

func NewService(store Store) *Service { return &Service{store: store} }

func (s *Service) Create(ctx context.Context, req CreateRoomRequest) (*Room, error) {
	r := &Room{
		Number:      req.Number,
		Floor:       req.Floor,
		Building:    req.Building,
		Longitude:   req.Longitude,
		Latitude:    req.Latitude,
		Description: req.Description,
	}
	return s.store.Create(ctx, r)
}

func (s *Service) Get(ctx context.Context, id int64) (*Room, error) {
	return s.store.GetByID(ctx, id)
}

func (s *Service) List(ctx context.Context, f ListFilter) (*ListResponse, error) {
	items, total, err := s.store.List(ctx, f)
	if err != nil {
		return nil, err
	}
	limit := f.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	offset := f.Offset
	if offset < 0 {
		offset = 0
	}
	if items == nil {
		items = []Room{}
	}
	return &ListResponse{Items: items, Total: total, Limit: limit, Offset: offset}, nil
}

func (s *Service) Update(ctx context.Context, id int64, req UpdateRoomRequest) (*Room, error) {
	cur, err := s.store.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if req.Number != nil {
		cur.Number = *req.Number
	}
	if req.Floor != nil {
		cur.Floor = *req.Floor
	}
	if req.Building != nil {
		cur.Building = *req.Building
	}
	if req.Longitude != nil {
		cur.Longitude = *req.Longitude
	}
	if req.Latitude != nil {
		cur.Latitude = *req.Latitude
	}
	if req.Description != nil {
		cur.Description = *req.Description
	}
	return s.store.Update(ctx, cur)
}

func (s *Service) Delete(ctx context.Context, id int64) error {
	return s.store.Delete(ctx, id)
}

// GetAvailability — если записи нет, возвращает free (default).
// Это важно для публичного API: клиенту не нужно отдельно обрабатывать «нет данных».
func (s *Service) GetAvailability(ctx context.Context, roomID int64, date time.Time) (*Availability, error) {
	// Проверяем, что комната существует (иначе 404, а не «free для несуществующей»).
	if _, err := s.store.GetByID(ctx, roomID); err != nil {
		return nil, err
	}
	a, err := s.store.GetAvailability(ctx, roomID, date)
	if err != nil {
		if errors.Is(err, ErrAvailabilityNotFound) {
			return &Availability{
				RoomID: roomID,
				Date:   date.Format("2006-01-02"),
				Status: StatusFree,
			}, nil
		}
		return nil, err
	}
	return a, nil
}

// SetAvailability — upsert. updatedBy = 0 → NULL в БД.
func (s *Service) SetAvailability(ctx context.Context, roomID int64, date time.Time, status AvailabilityStatus, updatedBy int64) (*Availability, error) {
	if status != StatusFree && status != StatusBusy {
		return nil, fmt.Errorf("invalid status %q", status)
	}
	// Проверка существования комнаты — до INSERT, чтобы 404 вместо FK violation.
	if _, err := s.store.GetByID(ctx, roomID); err != nil {
		return nil, err
	}
	return s.store.SetAvailability(ctx, roomID, date, status, updatedBy)
}