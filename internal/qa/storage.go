package qa

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"
)

var ErrNotFound = errors.New("qa: not found")

type Store interface {
	Create(ctx context.Context, q *QA) (*QA, error)
	GetByID(ctx context.Context, id int64) (*QA, error)

	// PublicList — только status=published. Вызывается из публичного handler'а.
	PublicList(ctx context.Context, f ListFilter) ([]QA, int, error)

	// ListByStatus — для очередей модерации (админ/модератор).
	ListByStatus(ctx context.Context, f ModerationFilter) ([]QA, int, error)

	// Answer — записывает ответ модератора + переход в pending_review.
	Answer(ctx context.Context, id int64, moderatorID int64, answer string, now time.Time) (*QA, error)

	// Publish / Reject — финальные действия админа.
	Publish(ctx context.Context, id int64, reviewerID int64, now time.Time) (*QA, error)
	Reject(ctx context.Context, id int64, reviewerID int64, comment string, now time.Time) (*QA, error)
}

type pgStore struct{ db *sql.DB }

func NewStore(db *sql.DB) Store { return &pgStore{db: db} }

const qaColumns = `
	id, key_id, sender_id, type_forum_id, title, description, tags, status_id, created_at,
	moderator_id, answer, answered_at, reviewer_id, review_comment, reviewed_at
`

func scanQA(row interface{ Scan(dest ...any) error }, q *QA) error {
	var (
		senderID, modID, revID sql.NullInt64
		answeredAt, reviewedAt sql.NullTime
	)
	if err := row.Scan(
		&q.ID, &q.KeyID, &senderID, &q.TypeForumID, &q.Title, &q.Description,
		pq.Array(&q.Tags), &q.StatusID, &q.CreatedAt,
		&modID, &q.Answer, &answeredAt, &revID, &q.ReviewComment, &reviewedAt,
	); err != nil {
		return err
	}
	if senderID.Valid {
		v := senderID.Int64
		q.SenderID = &v
	}
	if modID.Valid {
		v := modID.Int64
		q.ModeratorID = &v
	}
	if revID.Valid {
		v := revID.Int64
		q.ReviewerID = &v
	}
	if answeredAt.Valid {
		v := answeredAt.Time
		q.AnsweredAt = &v
	}
	if reviewedAt.Valid {
		v := reviewedAt.Time
		q.ReviewedAt = &v
	}
	return nil
}

func (s *pgStore) Create(ctx context.Context, q *QA) (*QA, error) {
	// type_forum_id по умолчанию = 2 (question). Если клиент не передал — 0 → NULL → DEFAULT.
	var tfArg any
	if q.TypeForumID > 0 {
		tfArg = q.TypeForumID
	}
	var senderArg any
	if q.SenderID != nil {
		senderArg = *q.SenderID
	}

	const qsql = `
		INSERT INTO qa_a (key_id, sender_id, type_forum_id, title, description, tags, status_id)
		VALUES ($1, $2, COALESCE($3, 2), $4, $5, $6, $7)
		RETURNING ` + qaColumns

	out := *q
	if err := scanQA(s.db.QueryRowContext(ctx, qsql,
		q.KeyID, senderArg, tfArg, q.Title, q.Description,
		pq.Array(q.Tags), StatusNew,
	), &out); err != nil {
		return nil, fmt.Errorf("create qa: %w", err)
	}
	return &out, nil
}

func (s *pgStore) GetByID(ctx context.Context, id int64) (*QA, error) {
	const qsql = `SELECT ` + qaColumns + ` FROM qa_a WHERE id = $1`
	var q QA
	err := scanQA(s.db.QueryRowContext(ctx, qsql, id), &q)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get qa: %w", err)
	}
	return &q, nil
}

func (s *pgStore) PublicList(ctx context.Context, f ListFilter) ([]QA, int, error) {
	args := []any{StatusPublished}
	conds := []string{"status_id = $1"}
	argIdx := 2

	if f.TypeForumID > 0 {
		conds = append(conds, fmt.Sprintf("type_forum_id = $%d", argIdx))
		args = append(args, f.TypeForumID)
		argIdx++
	}
	if f.Tag != "" {
		conds = append(conds, fmt.Sprintf("$%d = ANY(tags)", argIdx))
		args = append(args, f.Tag)
		argIdx++
	}
	return s.list(ctx, conds, args, argIdx, f.Limit, f.Offset)
}

func (s *pgStore) ListByStatus(ctx context.Context, f ModerationFilter) ([]QA, int, error) {
	conds := []string{"1=1"}
	args := []any{}
	argIdx := 1

	if f.StatusID > 0 {
		conds = append(conds, fmt.Sprintf("status_id = $%d", argIdx))
		args = append(args, f.StatusID)
		argIdx++
	}
	if f.ModeratorID > 0 {
		conds = append(conds, fmt.Sprintf("moderator_id = $%d", argIdx))
		args = append(args, f.ModeratorID)
		argIdx++
	}
	return s.list(ctx, conds, args, argIdx, f.Limit, f.Offset)
}

func (s *pgStore) list(ctx context.Context, conds []string, args []any, argIdx, limit, offset int) ([]QA, int, error) {
	where := "WHERE " + strings.Join(conds, " AND ")

	var total int
	if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM qa_a "+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count qa: %w", err)
	}

	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	qsql := fmt.Sprintf(`
		SELECT %s FROM qa_a %s
		ORDER BY created_at DESC, id DESC
		LIMIT $%d OFFSET $%d
	`, qaColumns, where, argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := s.db.QueryContext(ctx, qsql, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list qa: %w", err)
	}
	defer rows.Close()

	out := make([]QA, 0, limit)
	for rows.Next() {
		var q QA
		if err := scanQA(rows, &q); err != nil {
			return nil, 0, fmt.Errorf("scan qa: %w", err)
		}
		out = append(out, q)
	}
	return out, total, rows.Err()
}

// Answer — атомарный переход new | needs_revision → pending_review.
// WHERE status_id IN (...) гарантирует, что модератор не перезапишет чужой ответ.
func (s *pgStore) Answer(ctx context.Context, id int64, moderatorID int64, answer string, now time.Time) (*QA, error) {
	const qsql = `
		UPDATE qa_a
		SET moderator_id = $1,
		    answer       = $2,
		    answered_at  = $3,
		    status_id    = $4,
		    review_comment = '',
		    reviewer_id  = NULL,
		    reviewed_at  = NULL
		WHERE id = $5 AND status_id IN ($6, $7)
		RETURNING ` + qaColumns

	var q QA
	err := scanQA(s.db.QueryRowContext(ctx, qsql,
		moderatorID, answer, now, StatusPendingReview,
		id, StatusNew, StatusNeedsRevision,
	), &q)
	if errors.Is(err, sql.ErrNoRows) {
		// Либо id нет, либо статус не тот — различаем по повторному SELECT.
		if _, getErr := s.GetByID(ctx, id); getErr == nil {
			return nil, ErrInvalidStatus
		}
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("answer qa: %w", err)
	}
	return &q, nil
}

func (s *pgStore) Publish(ctx context.Context, id int64, reviewerID int64, now time.Time) (*QA, error) {
	const qsql = `
		UPDATE qa_a
		SET status_id = $1, reviewer_id = $2, reviewed_at = $3, review_comment = ''
		WHERE id = $4 AND status_id = $5
		RETURNING ` + qaColumns

	var q QA
	err := scanQA(s.db.QueryRowContext(ctx, qsql,
		StatusPublished, reviewerID, now, id, StatusPendingReview,
	), &q)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, s.classifyUpdateMiss(ctx, id)
	}
	if err != nil {
		return nil, fmt.Errorf("publish qa: %w", err)
	}
	return &q, nil
}

func (s *pgStore) Reject(ctx context.Context, id int64, reviewerID int64, comment string, now time.Time) (*QA, error) {
	const qsql = `
		UPDATE qa_a
		SET status_id = $1, reviewer_id = $2, reviewed_at = $3, review_comment = $4
		WHERE id = $5 AND status_id = $6
		RETURNING ` + qaColumns

	var q QA
	err := scanQA(s.db.QueryRowContext(ctx, qsql,
		StatusNeedsRevision, reviewerID, now, comment, id, StatusPendingReview,
	), &q)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, s.classifyUpdateMiss(ctx, id)
	}
	if err != nil {
		return nil, fmt.Errorf("reject qa: %w", err)
	}
	return &q, nil
}

// classifyUpdateMiss — «запись есть, но статус не тот» vs «записи нет».
func (s *pgStore) classifyUpdateMiss(ctx context.Context, id int64) error {
	if _, err := s.GetByID(ctx, id); errors.Is(err, ErrNotFound) {
		return ErrNotFound
	}
	return ErrInvalidStatus
}