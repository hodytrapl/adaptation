package users

import (
	"context"
)

type Service struct {
	store Store
}

func NewService(store Store) *Service { return &Service{store: store} }

func (s *Service) Create(ctx context.Context, req CreateUserRequest) (*User, error) {
	u := &User{
		Login:  req.Login,
		Name:   req.Name,
		RoleID: req.RoleID,
		Scopes: req.Scopes,
	}
	return s.store.Create(ctx, u, req.Password)
}

func (s *Service) Get(ctx context.Context, id int64) (*User, error) {
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
		items = []User{}
	}
	return &ListResponse{Items: items, Total: total, Limit: limit, Offset: offset}, nil
}

func (s *Service) Update(ctx context.Context, id int64, req UpdateUserRequest) (*User, error) {
	cur, err := s.store.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if req.Name != nil {
		cur.Name = *req.Name
	}
	if req.RoleID != nil {
		cur.RoleID = *req.RoleID
	}
	if req.Scopes != nil {
		cur.Scopes = *req.Scopes
	}
	return s.store.Update(ctx, cur, req.Password)
}

// Delete — запрет на self-delete: super_admin не должен рубить сук, на котором сидит.
func (s *Service) Delete(ctx context.Context, id, actorID int64) error {
	if id == actorID {
		return ErrCannotDeleteSelf
	}
	return s.store.Delete(ctx, id)
}

func (s *Service) ListRoles(ctx context.Context) ([]Role, error) {
	return s.store.ListRoles(ctx)
}