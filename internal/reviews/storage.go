package reviews

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"
)

// ErrNotFound — единая ошибка "запись не найдена".
// Сервис и handler маппят её в 404. Раньше в tasks было (Task{}, false, nil) —
// с БД чище вернуть ошибку, чем два возвращаемых значения-«флага».
var ErrNotFound = errors.New("reviews: not found")

// Store реализует интерфейс Store из service.go поверх *sql.DB.
//
// Почему отдельный тип, а не методы на *sql.DB — чтобы было куда класть
// вспомогательные вещи (например, prepared statements) и чтобы не течь
// *sql.DB в бизнес-логику.
type pgStore struct {
	db *sql.DB
}

// NewStore — конструктор, который возвращает Store (интерфейс), а не *pgStore.
// Это позволяет тестам подменить хранилище fake-реализацией.
func NewStore(db *sql.DB) Store {
	return &pgStore{db: db}
}

const reviewColumns = `id, key_id, sender_id, type_forum_id, title, description, tags, status_id, created_at`

func scanReview(row interface {
	Scan(dest ...any) error
}, r *Review) error {
	return row.Scan(
		&r.ID, &r.KeyID, &r.SenderID, &r.TypeForumID,
		&r.Title, &r.Description, pq.Array(&r.Tags), &r.StatusID, &r.CreatedAt,
	)
}

func (s *pgStore) Create(ctx context.Context, r *Review) (*Review, error) {
	const q = `
		INSERT INTO reviews (key_id, sender_id, type_forum_id, title, description, tags, status_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING ` + reviewColumns

	out := *r
	err := scanReview(s.db.QueryRowContext(ctx, q,
		r.KeyID, r.SenderID, r.TypeForumID, r.Title, r.Description,
		pq.Array(r.Tags), r.StatusID,
	), &out)
	if err != nil {
		return nil, fmt.Errorf("create review: %w", err)
	}
	return &out, nil
}

func (s *pgStore) GetByID(ctx context.Context, id int64) (*Review, error) {
	const q = `SELECT ` + reviewColumns + ` FROM reviews WHERE id = $1`
	var r Review
	err := scanReview(s.db.QueryRowContext(ctx, q, id), &r)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get review: %w", err)
	}
	return &r, nil
}

func (s *pgStore) List(ctx context.Context, f ListFilter) ([]Review, int, error) {
	var (
		conds  []string
		args   []any
		argIdx = 1
	)
	add := func(expr string, val any) {
		conds = append(conds, fmt.Sprintf(expr, argIdx))
		args = append(args, val)
		argIdx++
	}

	if f.StatusID > 0 {
		add("status_id = $%d", f.StatusID)
	}
	if f.SenderID > 0 {
		add("sender_id = $%d", f.SenderID)
	}
	if f.TypeForumID > 0 {
		add("type_forum_id = $%d", f.TypeForumID)
	}
	if f.Tag != "" {
		// ANY(tags) — самый простой способ проверить «есть ли тег в массиве».
		add("$%d = ANY(tags)", f.Tag)
	}

	where := ""
	if len(conds) > 0 {
		where = "WHERE " + strings.Join(conds, " AND ")
	}

	// total — для пагинации на фронте (чтобы не дёргать два раза)
	var total int
	countQ := "SELECT count(*) FROM reviews " + where
	if err := s.db.QueryRowContext(ctx, countQ, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count reviews: %w", err)
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
		SELECT `+reviewColumns+`
		FROM reviews %s
		ORDER BY created_at DESC, id DESC
		LIMIT $%d OFFSET $%d
	`, where, argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := s.db.QueryContext(ctx, dataQ, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list reviews: %w", err)
	}
	defer rows.Close()

	var out []Review
	for rows.Next() {
		var r Review
		if err := scanReview(rows, &r); err != nil {
			return nil, 0, fmt.Errorf("scan review: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("rows: %w", err)
	}
	return out, total, nil
}

func (s *pgStore) Update(ctx context.Context, r *Review) (*Review, error) {
	const q = `
		UPDATE reviews
		SET title = $1, description = $2, tags = $3
		WHERE id = $4
		RETURNING ` + reviewColumns

	out := *r
	err := scanReview(s.db.QueryRowContext(ctx, q,
		r.Title, r.Description, pq.Array(r.Tags), r.ID,
	), &out)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("update review: %w", err)
	}
	return &out, nil
}

func (s *pgStore) UpdateStatus(ctx context.Context, id, statusID int64) (*Review, error) {
	const q = `
		UPDATE reviews SET status_id = $1 WHERE id = $2
		RETURNING ` + reviewColumns

	var out Review
	err := scanReview(s.db.QueryRowContext(ctx, q, statusID, id), &out)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("update status: %w", err)
	}
	return &out, nil
}

func (s *pgStore) Delete(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM reviews WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete review: %w", err)
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

// FindStale — кандидаты на автоперевод new → in_progress.
// Используется cron-воркером (RunStaleWatcher в service.go).
//
// Cutoff считаем в Go и передаём как TIMESTAMPTZ — это надёжнее,
// чем собирать интервал строками в SQL.
func (s *pgStore) FindStale(ctx context.Context, threshold time.Duration) ([]Review, error) {
	cutoff := time.Now().Add(-threshold)
	const q = `
		SELECT ` + reviewColumns + `
		FROM reviews
		WHERE status_id = $1 AND created_at < $2
		ORDER BY created_at ASC
	`
	rows, err := s.db.QueryContext(ctx, q, StatusNew, cutoff)
	if err != nil {
		return nil, fmt.Errorf("find stale: %w", err)
	}
	defer rows.Close()

	var out []Review
	for rows.Next() {
		var r Review
		if err := scanReview(rows, &r); err != nil {
			return nil, fmt.Errorf("scan stale: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}