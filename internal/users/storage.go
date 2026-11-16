package users

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrNotFound         = errors.New("users: not found")
	ErrLoginExists      = errors.New("users: login already exists")
	ErrRoleNotFound     = errors.New("users: role not found")
	ErrCannotDeleteSelf = errors.New("users: cannot delete yourself")
	ErrInUse            = errors.New("users: user has dependent data")
)

type Store interface {
	Create(ctx context.Context, u *User, password string) (*User, error)
	GetByID(ctx context.Context, id int64) (*User, error)
	List(ctx context.Context, f ListFilter) ([]User, int, error)
	Update(ctx context.Context, u *User, password *string) (*User, error)
	Delete(ctx context.Context, id int64) error
	ListRoles(ctx context.Context) ([]Role, error)
}

type pgStore struct{ db *sql.DB }

func NewStore(db *sql.DB) Store { return &pgStore{db: db} }

const userSelect = `
	SELECT u.id, u.login, u.name, u.role_id, r.name, COALESCE(u.scopes,'{}'), u.created_at
	FROM users u JOIN roles r ON r.id = u.role_id
`

func scanUser(row interface{ Scan(...any) error }, u *User) error {
	return row.Scan(&u.ID, &u.Login, &u.Name, &u.RoleID, &u.Role, pq.Array(&u.Scopes), &u.CreatedAt)
}

func (s *pgStore) Create(ctx context.Context, u *User, password string) (*User, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}
	if u.Scopes == nil {
		u.Scopes = []string{}
	}

	const q = `
		WITH ins AS (
			INSERT INTO users (login, password, name, role_id, scopes)
			VALUES ($1, $2, $3, $4, $5)
			RETURNING id
		)
		SELECT u.id, u.login, u.name, u.role_id, r.name, COALESCE(u.scopes,'{}'), u.created_at
		FROM ins JOIN users u ON u.id = ins.id JOIN roles r ON r.id = u.role_id
	`
	var out User
	err = scanUser(s.db.QueryRowContext(ctx, q, u.Login, string(hash), u.Name, u.RoleID, pq.Array(u.Scopes)), &out)
	if err != nil {
		if isUniqueViolation(err, "users_login_key") {
			return nil, ErrLoginExists
		}
		if isForeignKeyViolation(err) {
			return nil, ErrRoleNotFound
		}
		return nil, fmt.Errorf("create user: %w", err)
	}
	return &out, nil
}

func (s *pgStore) GetByID(ctx context.Context, id int64) (*User, error) {
	var u User
	err := scanUser(s.db.QueryRowContext(ctx, userSelect+` WHERE u.id = $1`, id), &u)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}
	return &u, nil
}

func (s *pgStore) List(ctx context.Context, f ListFilter) ([]User, int, error) {
	conds := []string{"1=1"}
	args := []any{}
	argIdx := 1

	if f.RoleID > 0 {
		conds = append(conds, fmt.Sprintf("u.role_id = $%d", argIdx))
		args = append(args, f.RoleID)
		argIdx++
	}
	if f.Query != "" {
		conds = append(conds, fmt.Sprintf("(u.login ILIKE '%%'||$%d||'%%' OR u.name ILIKE '%%'||$%d||'%%')", argIdx, argIdx))
		args = append(args, f.Query)
		argIdx++
	}

	where := "WHERE " + strings.Join(conds, " AND ")

	var total int
	if err := s.db.QueryRowContext(ctx,
		"SELECT count(*) FROM users u "+where, args...,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count users: %w", err)
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
		%s %s
		ORDER BY u.id
		LIMIT $%d OFFSET $%d
	`, userSelect, where, argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := s.db.QueryContext(ctx, dataQ, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	out := make([]User, 0, limit)
	for rows.Next() {
		var u User
		if err := scanUser(rows, &u); err != nil {
			return nil, 0, fmt.Errorf("scan user: %w", err)
		}
		out = append(out, u)
	}
	return out, total, rows.Err()
}

func (s *pgStore) Update(ctx context.Context, u *User, password *string) (*User, error) {
	if u.Scopes == nil {
		u.Scopes = []string{}
	}

	var (
		hashArg any
	)
	if password != nil {
		hash, err := bcrypt.GenerateFromPassword([]byte(*password), bcrypt.DefaultCost)
		if err != nil {
			return nil, fmt.Errorf("hash password: %w", err)
		}
		hashArg = string(hash)
	}

	const q = `
		WITH upd AS (
			UPDATE users
			SET name     = $1,
			    role_id  = $2,
			    scopes   = $3,
			    password = COALESCE($4, password)
			WHERE id = $5
			RETURNING id
		)
		SELECT u.id, u.login, u.name, u.role_id, r.name, COALESCE(u.scopes,'{}'), u.created_at
		FROM upd JOIN users u ON u.id = upd.id JOIN roles r ON r.id = u.role_id
	`
	var out User
	err := scanUser(s.db.QueryRowContext(ctx, q, u.Name, u.RoleID, pq.Array(u.Scopes), hashArg, u.ID), &out)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		if isForeignKeyViolation(err) {
			return nil, ErrRoleNotFound
		}
		return nil, fmt.Errorf("update user: %w", err)
	}
	return &out, nil
}

func (s *pgStore) Delete(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, id)
	if err != nil {
		if isForeignKeyViolation(err) {
			return ErrInUse
		}
		return fmt.Errorf("delete user: %w", err)
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

func (s *pgStore) ListRoles(ctx context.Context) ([]Role, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, COALESCE(scopes,'{}') FROM roles ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list roles: %w", err)
	}
	defer rows.Close()
	var out []Role
	for rows.Next() {
		var r Role
		if err := rows.Scan(&r.ID, &r.Name, pq.Array(&r.Scopes)); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Классификация ошибок Postgres по коду SQLSTATE через текст (без импорта pgconn).
// В проде лучше errors.As с *pgconn.PgError — но тогда pgx-зависимость в этом пакете.
func isUniqueViolation(err error, constraint string) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "23505") || strings.Contains(s, constraint) ||
		strings.Contains(strings.ToLower(s), "duplicate key")
}

func isForeignKeyViolation(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "23503") ||
		strings.Contains(strings.ToLower(s), "foreign key")
}