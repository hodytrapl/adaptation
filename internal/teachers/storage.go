package teachers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

var ErrNotFound = errors.New("teachers: not found")

// Store — контракт хранилища (fake-реализация в тестах).
type Store interface {
	Create(ctx context.Context, t *Teacher) (*Teacher, error)
	GetByID(ctx context.Context, id int64) (*Teacher, error)
	List(ctx context.Context, f ListFilter) ([]Teacher, int, error)
	Update(ctx context.Context, t *Teacher) (*Teacher, error)
	Delete(ctx context.Context, id int64) error
}

type pgStore struct{ db *sql.DB }

func NewStore(db *sql.DB) Store { return &pgStore{db: db} }

// teacherSelect: LEFT JOIN на map_build, чтобы отдать room одним запросом.
const teacherSelect = `
	SELECT
		t.id, t.user_id, t.full_name, t.department, t.position,
		t.contacts, COALESCE(t.photo_url, ''), t.created_at,
		m.id, m.number, m.building
	FROM teachers t
	LEFT JOIN map_build m ON m.id = t.room_id
`

func scanTeacher(row interface{ Scan(dest ...any) error }, tch *Teacher) error {
	var (
		userID      sql.NullInt64
		contactsRaw []byte
		roomID      sql.NullInt64
		roomNumber  sql.NullString
		roomBuild   sql.NullString
	)
	if err := row.Scan(
		&tch.ID, &userID, &tch.FullName, &tch.Department, &tch.Position,
		&contactsRaw, &tch.PhotoURL, &tch.CreatedAt,
		&roomID, &roomNumber, &roomBuild,
	); err != nil {
		return err
	}
	if userID.Valid {
		v := userID.Int64
		tch.UserID = &v
	} else {
		tch.UserID = nil
	}
	if len(contactsRaw) > 0 {
		if err := json.Unmarshal(contactsRaw, &tch.Contacts); err != nil {
			return fmt.Errorf("decode contacts: %w", err)
		}
	}
	if roomID.Valid {
		tch.Room = &RoomRef{
			ID:       roomID.Int64,
			Number:   roomNumber.String,
			Building: roomBuild.String,
		}
	} else {
		tch.Room = nil
	}
	return nil
}

func (s *pgStore) Create(ctx context.Context, t *Teacher) (*Teacher, error) {
	contactsJSON, err := json.Marshal(t.Contacts)
	if err != nil {
		return nil, fmt.Errorf("encode contacts: %w", err)
	}

	const q = `
		WITH ins AS (
			INSERT INTO teachers (full_name, department, position, contacts, photo_url, room_id)
			VALUES ($1, $2, $3, $4, $5, $6)
			RETURNING id
		)
		SELECT
			t.id, t.full_name, t.department, t.position, t.contacts, t.photo_url,
			t.created_at,
			m.id, m.number, m.building
		FROM ins
		JOIN teachers t ON t.id = ins.id
		LEFT JOIN map_build m ON m.id = t.room_id
	`
	var (
		roomIDArg any
	)
	if t.Room != nil && t.Room.ID != 0 {
		roomIDArg = t.Room.ID
	}

	var out Teacher
	if err := scanTeacher(s.db.QueryRowContext(ctx, q,
		t.FullName, t.Department, t.Position, contactsJSON, nullIfEmpty(t.PhotoURL), roomIDArg,
	), &out); err != nil {
		return nil, fmt.Errorf("create teacher: %w", err)
	}
	return &out, nil
}

func (s *pgStore) GetByID(ctx context.Context, id int64) (*Teacher, error) {
	var tch Teacher
	err := scanTeacher(s.db.QueryRowContext(ctx, teacherSelect+` WHERE t.id = $1`, id), &tch)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get teacher: %w", err)
	}
	return &tch, nil
}

func (s *pgStore) List(ctx context.Context, f ListFilter) ([]Teacher, int, error) {
	var (
		conds  []string
		args   []any
		argIdx = 1
	)
	if f.Department != "" {
		conds = append(conds, fmt.Sprintf("t.department = $%d", argIdx))
		args = append(args, f.Department)
		argIdx++
	}
	if f.RoomID > 0 {
		conds = append(conds, fmt.Sprintf("t.room_id = $%d", argIdx))
		args = append(args, f.RoomID)
		argIdx++
	}
	if f.Query != "" {
		conds = append(conds, fmt.Sprintf("t.full_name ILIKE '%%' || $%d || '%%'", argIdx))
		args = append(args, f.Query)
		argIdx++
	}

	where := ""
	if len(conds) > 0 {
		where = "WHERE " + strings.Join(conds, " AND ")
	}

	// count — без join’а, чтобы не платить за LEFT JOIN.
	var total int
	if err := s.db.QueryRowContext(ctx,
		"SELECT count(*) FROM teachers t "+where, args...,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count teachers: %w", err)
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
		%s
		%s
		ORDER BY t.full_name, t.id
		LIMIT $%d OFFSET $%d
	`, teacherSelect, where, argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := s.db.QueryContext(ctx, dataQ, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list teachers: %w", err)
	}
	defer rows.Close()

	out := make([]Teacher, 0, limit)
	for rows.Next() {
		var tch Teacher
		if err := scanTeacher(rows, &tch); err != nil {
			return nil, 0, fmt.Errorf("scan teacher: %w", err)
		}
		out = append(out, tch)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("rows: %w", err)
	}
	return out, total, nil
}

func (s *pgStore) Update(ctx context.Context, t *Teacher) (*Teacher, error) {
	contactsJSON, err := json.Marshal(t.Contacts)
	if err != nil {
		return nil, fmt.Errorf("encode contacts: %w", err)
	}

	var roomIDArg any
	if t.Room != nil && t.Room.ID != 0 {
		roomIDArg = t.Room.ID
	}

	const q = `
		WITH upd AS (
			UPDATE teachers
			SET full_name  = $1,
			    department = $2,
			    position   = $3,
			    contacts   = $4,
			    photo_url  = $5,
			    room_id    = $6
			WHERE id = $7
			RETURNING id
		)
		SELECT
			t.id, t.full_name, t.department, t.position, t.contacts, t.photo_url,
			t.created_at,
			m.id, m.number, m.building
		FROM upd
		JOIN teachers t ON t.id = upd.id
		LEFT JOIN map_build m ON m.id = t.room_id
	`
	var out Teacher
	err = scanTeacher(s.db.QueryRowContext(ctx, q,
		t.FullName, t.Department, t.Position, contactsJSON, nullIfEmpty(t.PhotoURL), roomIDArg, t.ID,
	), &out)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("update teacher: %w", err)
	}
	return &out, nil
}

func (s *pgStore) Delete(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM teachers WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete teacher: %w", err)
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

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
