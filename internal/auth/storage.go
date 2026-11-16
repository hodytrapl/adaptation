package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/lib/pq"
)

// ErrNotFound — единая «нет такой записи». Маппится в 401/404 на уровне handler.
var ErrNotFound = errors.New("auth: not found")

// ErrRevoked — refresh-токен уже отозван (logout / reuse-after-rotate).
var ErrRevoked = errors.New("auth: token revoked")

// ErrExpired — refresh-токен истёк.
var ErrExpired = errors.New("auth: token expired")

// Store — контракт хранилища. Отдельным интерфейсом — чтобы в тестах
// подменить на in-memory fake без Postgres.
type Store interface {
	FindUserByLogin(ctx context.Context, login string) (*User, error)
	FindUserByID(ctx context.Context, id int64) (*User, error)

	CreateRefreshToken(ctx context.Context, rt *RefreshTokenRecord, ua, ip string) error
	GetRefreshToken(ctx context.Context, hash []byte) (*RefreshTokenRecord, time.Time, bool, error) // record, revokedAt, revoked
	RevokeRefreshToken(ctx context.Context, hash []byte) error
	RevokeAllUserTokens(ctx context.Context, userID int64) error
}

type pgStore struct{ db *sql.DB }

func NewStore(db *sql.DB) Store { return &pgStore{db: db} }

const userSelect = `
	SELECT u.id, u.login, u.name, u.password,
	       r.id, r.name, COALESCE(r.scopes, ARRAY[]::TEXT[]),
	       COALESCE(u.scopes, ARRAY[]::TEXT[])
	FROM users u
	JOIN roles r ON r.id = u.role_id
`

func scanUser(row interface{ Scan(...any) error }) (*User, error) {
	var (
		u User
		rs int64
		rn string
		rs2 []string
		us  []string
	)
	if err := row.Scan(&u.ID, &u.Login, &u.Name, &u.PasswordHash, &rs, &rn, pq.Array(&rs2), pq.Array(&us)); err != nil {
		return nil, err
	}
	u.Role = Role{ID: rs, Name: rn, Scopes: rs2}
	u.UserScopes = us
	return &u, nil
}

func (s *pgStore) FindUserByLogin(ctx context.Context, login string) (*User, error) {
	u, err := scanUser(s.db.QueryRowContext(ctx, userSelect+` WHERE u.login = $1`, login))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find user by login: %w", err)
	}
	return u, nil
}

func (s *pgStore) FindUserByID(ctx context.Context, id int64) (*User, error) {
	u, err := scanUser(s.db.QueryRowContext(ctx, userSelect+` WHERE u.id = $1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find user by id: %w", err)
	}
	return u, nil
}

func (s *pgStore) CreateRefreshToken(ctx context.Context, rt *RefreshTokenRecord, ua, ip string) error {
	const q = `
		INSERT INTO refresh_tokens (user_id, token_hash, expires_at, user_agent, ip)
		VALUES ($1, $2, $3, NULLIF($4,'')::text, NULLIF($5,'')::inet)
	`
	var ipArg any
	if ip != "" {
		ipArg = ip
	}
	if _, err := s.db.ExecContext(ctx, q, rt.UserID, rt.Hash, rt.ExpiresAt, ua, ipArg); err != nil {
		return fmt.Errorf("create refresh: %w", err)
	}
	return nil
}

// GetRefreshToken возвращает (record, revokedAt, revoked bool, error).
// Если запись не найдена — (nil, zero, false, ErrNotFound).
func (s *pgStore) GetRefreshToken(ctx context.Context, hash []byte) (*RefreshTokenRecord, time.Time, bool, error) {
	const q = `
		SELECT user_id, token_hash, expires_at, revoked_at
		FROM refresh_tokens WHERE token_hash = $1
	`
	var (
		rt        RefreshTokenRecord
		revokedAt sql.NullTime
	)
	err := s.db.QueryRowContext(ctx, q, hash).Scan(&rt.UserID, &rt.Hash, &rt.ExpiresAt, &revokedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, time.Time{}, false, ErrNotFound
	}
	if err != nil {
		return nil, time.Time{}, false, fmt.Errorf("get refresh: %w", err)
	}
	return &rt, revokedAt.Time, revokedAt.Valid, nil
}

func (s *pgStore) RevokeRefreshToken(ctx context.Context, hash []byte) error {
	const q = `UPDATE refresh_tokens SET revoked_at = now() WHERE token_hash = $1 AND revoked_at IS NULL`
	if _, err := s.db.ExecContext(ctx, q, hash); err != nil {
		return fmt.Errorf("revoke refresh: %w", err)
	}
	return nil
}

func (s *pgStore) RevokeAllUserTokens(ctx context.Context, userID int64) error {
	const q = `UPDATE refresh_tokens SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL`
	if _, err := s.db.ExecContext(ctx, q, userID); err != nil {
		return fmt.Errorf("revoke all user tokens: %w", err)
	}
	return nil
}