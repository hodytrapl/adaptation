package events

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

var ErrNotFound = errors.New("events: not found")

type Store interface {
	Create(ctx context.Context, e *Event) (*Event, error)
	GetByID(ctx context.Context, id int64) (*Event, error)
	List(ctx context.Context, f ListFilter) ([]Event, int, error)
	Update(ctx context.Context, e *Event) (*Event, error)
	Delete(ctx context.Context, id int64) error
}

type pgStore struct{ db *sql.DB }

func NewStore(db *sql.DB) Store { return &pgStore{db: db} }

const eventColumns = `
	id, title, description, date_started,
	EXTRACT(EPOCH FROM duration)::float8 AS duration_seconds,
	place, created_at
`

func scanEvent(row interface{ Scan(dest ...any) error }, e *Event) error {
	var durSec float64
	if err := row.Scan(
		&e.ID, &e.Title, &e.Description, &e.DateStarted,
		&durSec, &e.Place, &e.CreatedAt,
	); err != nil {
		return err
	}
	e.DurationSeconds = int64(durSec)
	return nil
}

func (s *pgStore) Create(ctx context.Context, e *Event) (*Event, error) {
	const q = `
		INSERT INTO events (title, description, date_started, duration, place)
		VALUES ($1, $2, $3, make_interval(secs => $4), $5)
		RETURNING ` + eventColumns
	out := *e
	if err := scanEvent(s.db.QueryRowContext(ctx, q,
		e.Title, e.Description, e.DateStarted, e.DurationSeconds, e.Place,
	), &out); err != nil {
		return nil, fmt.Errorf("create event: %w", err)
	}
	return &out, nil
}

func (s *pgStore) GetByID(ctx context.Context, id int64) (*Event, error) {
	const q = `SELECT ` + eventColumns + ` FROM events WHERE id = $1`
	var e Event
	err := scanEvent(s.db.QueryRowContext(ctx, q, id), &e)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get event: %w", err)
	}
	return &e, nil
}

func (s *pgStore) List(ctx context.Context, f ListFilter) ([]Event, int, error) {
	var (
		conds  []string
		args   []any
		argIdx = 1
	)
	if !f.From.IsZero() {
		conds = append(conds, fmt.Sprintf("date_started >= $%d", argIdx))
		args = append(args, f.From)
		argIdx++
	}
	if !f.To.IsZero() {
		conds = append(conds, fmt.Sprintf("date_started <= $%d", argIdx))
		args = append(args, f.To)
		argIdx++
	}
	where := ""
	if len(conds) > 0 {
		where = "WHERE " + strings.Join(conds, " AND ")
	}
	var total int
	if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM events "+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count events: %w", err)
	}
	limit := f.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	offset := f.Offset
	if offset < 0 {
		offset = 0
	}
	dataQ := fmt.Sprintf(`
		SELECT `+eventColumns+`
		FROM events %s
		ORDER BY date_started ASC, id ASC
		LIMIT $%d OFFSET $%d
	`, where, argIdx, argIdx+1)
	args = append(args, limit, offset)
	rows, err := s.db.QueryContext(ctx, dataQ, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list events: %w", err)
	}
	defer rows.Close()
	out := make([]Event, 0, limit)
	for rows.Next() {
		var e Event
		if err := scanEvent(rows, &e); err != nil {
			return nil, 0, fmt.Errorf("scan event: %w", err)
		}
		out = append(out, e)
	}
	return out, total, rows.Err()
}

func (s *pgStore) Update(ctx context.Context, e *Event) (*Event, error) {
	const q = `
		UPDATE events
		SET title = $1, description = $2, date_started = $3,
		    duration = make_interval(secs => $4), place = $5
		WHERE id = $6
		RETURNING ` + eventColumns
	out := *e
	err := scanEvent(s.db.QueryRowContext(ctx, q,
		e.Title, e.Description, e.DateStarted, e.DurationSeconds, e.Place, e.ID,
	), &out)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("update event: %w", err)
	}
	return &out, nil
}

func (s *pgStore) Delete(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM events WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete event: %w", err)
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

// keep time imported for potential future use; otherwise compile fails if unused
var _ = time.Now