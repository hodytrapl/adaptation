package rooms

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrNotFound             = errors.New("rooms: not found")
	ErrAvailabilityNotFound = errors.New("rooms: availability not set")
)

// Store — контракт хранилища (для подмены в тестах).
type Store interface {
	Create(ctx context.Context, r *Room) (*Room, error)
	GetByID(ctx context.Context, id int64) (*Room, error)
	List(ctx context.Context, f ListFilter) ([]Room, int, error)
	Update(ctx context.Context, r *Room) (*Room, error)
	Delete(ctx context.Context, id int64) error

	GetAvailability(ctx context.Context, roomID int64, date time.Time) (*Availability, error)
	SetAvailability(ctx context.Context, roomID int64, date time.Time, status AvailabilityStatus, updatedBy int64) (*Availability, error)
}

type pgStore struct{ db *sql.DB }

func NewStore(db *sql.DB) Store { return &pgStore{db: db} }

// roomColumns: POINT → lon/lat через array-подписи (без PostGIS).
const roomColumns = `
	id, number, floor, building,
	coordinates[0]::float8 AS lon,
	coordinates[1]::float8 AS lat,
	description, created_at
`

func scanRoom(row interface{ Scan(dest ...any) error }, r *Room) error {
	return row.Scan(
		&r.ID, &r.Number, &r.Floor, &r.Building,
		&r.Longitude, &r.Latitude,
		&r.Description, &r.CreatedAt,
	)
}

func (s *pgStore) Create(ctx context.Context, r *Room) (*Room, error) {
	const q = `
		INSERT INTO map_build (number, floor, building, coordinates, description)
		VALUES ($1, $2, $3, point($4, $5), $6)
		RETURNING ` + roomColumns

	out := *r
	if err := scanRoom(s.db.QueryRowContext(ctx, q,
		r.Number, r.Floor, r.Building, r.Longitude, r.Latitude, r.Description,
	), &out); err != nil {
		return nil, fmt.Errorf("create room: %w", err)
	}
	return &out, nil
}

func (s *pgStore) GetByID(ctx context.Context, id int64) (*Room, error) {
	const q = `SELECT ` + roomColumns + ` FROM map_build WHERE id = $1`
	var r Room
	err := scanRoom(s.db.QueryRowContext(ctx, q, id), &r)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get room: %w", err)
	}
	return &r, nil
}

func (s *pgStore) List(ctx context.Context, f ListFilter) ([]Room, int, error) {
	var (
		conds  []string
		args   []any
		argIdx = 1
	)
	if f.Building != "" {
		conds = append(conds, fmt.Sprintf("building = $%d", argIdx))
		args = append(args, f.Building)
		argIdx++
	}
	if f.Floor != 0 {
		conds = append(conds, fmt.Sprintf("floor = $%d", argIdx))
		args = append(args, f.Floor)
		argIdx++
	}

	where := ""
	if len(conds) > 0 {
		where = "WHERE " + strings.Join(conds, " AND ")
	}

	var total int
	if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM map_build "+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count rooms: %w", err)
	}

	limit := f.Limit
	if limit <= 0 || limit > 500 { // карта обычно тянет всё здание разом
		limit = 100
	}
	offset := f.Offset
	if offset < 0 {
		offset = 0
	}

	dataQ := fmt.Sprintf(`
		SELECT `+roomColumns+`
		FROM map_build %s
		ORDER BY building, floor, number, id
		LIMIT $%d OFFSET $%d
	`, where, argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := s.db.QueryContext(ctx, dataQ, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list rooms: %w", err)
	}
	defer rows.Close()

	out := make([]Room, 0, limit)
	for rows.Next() {
		var r Room
		if err := scanRoom(rows, &r); err != nil {
			return nil, 0, fmt.Errorf("scan room: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("rows: %w", err)
	}
	return out, total, nil
}

func (s *pgStore) Update(ctx context.Context, r *Room) (*Room, error) {
	const q = `
		UPDATE map_build
		SET number = $1, floor = $2, building = $3,
		    coordinates = point($4, $5), description = $6
		WHERE id = $7
		RETURNING ` + roomColumns

	out := *r
	err := scanRoom(s.db.QueryRowContext(ctx, q,
		r.Number, r.Floor, r.Building, r.Longitude, r.Latitude, r.Description, r.ID,
	), &out)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("update room: %w", err)
	}
	return &out, nil
}

func (s *pgStore) Delete(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM map_build WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete room: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// GetAvailability: если строки нет — ErrAvailabilityNotFound (сервис превратит в free).
func (s *pgStore) GetAvailability(ctx context.Context, roomID int64, date time.Time) (*Availability, error) {
	const q = `
		SELECT room_id, date, status, updated_at
		FROM room_availability
		WHERE room_id = $1 AND date = $2
	`
	var (
		a  Availability
		d  time.Time
		st string
	)
	err := s.db.QueryRowContext(ctx, q, roomID, date).Scan(&a.RoomID, &d, &st, &a.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrAvailabilityNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get availability: %w", err)
	}
	a.Date = d.Format("2006-01-02")
	a.Status = AvailabilityStatus(st)
	return &a, nil
}

// SetAvailability — upsert. updated_by=0 превращаем в NULL.
func (s *pgStore) SetAvailability(ctx context.Context, roomID int64, date time.Time, status AvailabilityStatus, updatedBy int64) (*Availability, error) {
	const q = `
		INSERT INTO room_availability (room_id, date, status, updated_by)
		VALUES ($1, $2, $3, NULLIF($4, 0))
		ON CONFLICT (room_id, date)
		DO UPDATE SET status     = EXCLUDED.status,
		              updated_by = EXCLUDED.updated_by,
		              updated_at = now()
		RETURNING room_id, date, status, updated_at
	`
	var (
		a  Availability
		d  time.Time
		st string
	)
	err := s.db.QueryRowContext(ctx, q, roomID, date, string(status), updatedBy).
		Scan(&a.RoomID, &d, &st, &a.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("set availability: %w", err)
	}
	a.Date = d.Format("2006-01-02")
	a.Status = AvailabilityStatus(st)
	return &a, nil
}