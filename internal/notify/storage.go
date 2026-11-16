package notify

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

var ErrNotFound = errors.New("notify: not found")

type Store interface {
	Create(ctx context.Context, n *Notification) (*Notification, error)
	List(ctx context.Context, f ListFilter) ([]Notification, int, error)
	MarkRead(ctx context.Context, id, userID int64) (*Notification, error)
}

type pgStore struct{ db *sql.DB }

func NewStore(db *sql.DB) Store { return &pgStore{db: db} }

const notifColumns = `id, user_id, kind, payload, read_at, created_at`

func scanNotif(row interface{ Scan(dest ...any) error }, n *Notification) error {
	var (
		payloadRaw []byte
		readAt     sql.NullTime
	)
	if err := row.Scan(&n.ID, &n.UserID, &n.Kind, &payloadRaw, &readAt, &n.CreatedAt); err != nil {
		return err
	}
	if len(payloadRaw) > 0 {
		if err := json.Unmarshal(payloadRaw, &n.Payload); err != nil {
			return fmt.Errorf("decode payload: %w", err)
		}
	}
	if readAt.Valid {
		v := readAt.Time
		n.ReadAt = &v
	}
	return nil
}

func (s *pgStore) Create(ctx context.Context, n *Notification) (*Notification, error) {
	payload := n.Payload
	if payload == nil {
		payload = map[string]any{}
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode payload: %w", err)
	}

	const q = `
		INSERT INTO notifications (user_id, kind, payload)
		VALUES ($1, $2, $3)
		RETURNING ` + notifColumns
	out := *n
	if err := scanNotif(s.db.QueryRowContext(ctx, q, n.UserID, n.Kind, raw), &out); err != nil {
		return nil, fmt.Errorf("create notification: %w", err)
	}
	return &out, nil
}

func (s *pgStore) List(ctx context.Context, f ListFilter) ([]Notification, int, error) {
	conds := []string{"user_id = $1"}
	args := []any{f.UserID}
	argIdx := 2
	if f.Unread {
		conds = append(conds, "read_at IS NULL")
	}

	where := "WHERE " + conds[0]
	for i := 1; i < len(conds); i++ {
		where += " AND " + conds[i]
	}

	var total int
	if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM notifications "+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count notifications: %w", err)
	}

	limit := f.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	offset := f.Offset
	if offset < 0 {
		offset = 0
	}

	dataQ := fmt.Sprintf(`
		SELECT %s FROM notifications %s
		ORDER BY created_at DESC, id DESC
		LIMIT $%d OFFSET $%d
	`, notifColumns, where, argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := s.db.QueryContext(ctx, dataQ, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list notifications: %w", err)
	}
	defer rows.Close()

	out := make([]Notification, 0, limit)
	for rows.Next() {
		var n Notification
		if err := scanNotif(rows, &n); err != nil {
			return nil, 0, fmt.Errorf("scan notification: %w", err)
		}
		out = append(out, n)
	}
	return out, total, rows.Err()
}

// MarkRead — идемпотентно. WHERE user_id = $2 — защита от IDOR:
// чужое уведомление вернёт ErrNotFound, а не «успех».
func (s *pgStore) MarkRead(ctx context.Context, id, userID int64) (*Notification, error) {
	const q = `
		UPDATE notifications
		SET read_at = COALESCE(read_at, now())
		WHERE id = $1 AND user_id = $2
		RETURNING ` + notifColumns
	var n Notification
	err := scanNotif(s.db.QueryRowContext(ctx, q, id, userID), &n)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("mark read: %w", err)
	}
	return &n, nil
}