package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"time"

	"task-manager/internal/authz"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// ErrInvalidCredentials — неверный login/password.
var ErrInvalidCredentials = errors.New("auth: invalid credentials")

// ErrInvalidToken — невалидный/просроченный access/refresh.
var ErrInvalidToken = errors.New("auth: invalid token")

// ErrTokenReuse — попытка использовать уже отозванный refresh (признак кражи).
var ErrTokenReuse = errors.New("auth: refresh token reuse detected")

// Config — параметры сервиса. JWTSecret обязателен.
type Config struct {
	JWTSecret    []byte
	Issuer       string
	AccessTTL    time.Duration
	RefreshTTL   time.Duration
}

// Service — бизнес-логика auth.
type Service struct {
	store Store
	cfg   Config
}

func NewService(store Store, cfg Config) *Service {
	if cfg.Issuer == "" {
		cfg.Issuer = "task-manager"
	}
	return &Service{store: store, cfg: cfg}
}

// Login проверяет пароль и выдаёт пару токенов.
func (s *Service) Login(ctx context.Context, req LoginRequest, ua, ip string) (*TokenPair, error) {
	u, err := s.store.FindUserByLogin(ctx, req.Login)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			// Нарочно не различаем "нет юзера" / "неверный пароль" — не даём
			// атакующему определять существующие логины.
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(req.Password)); err != nil {
		return nil, ErrInvalidCredentials
	}
	return s.issuePair(ctx, u, ua, ip)
}

// Refresh ротирует refresh-токен: старый ревокается, выдаётся новый.
// Если приходит уже отозванный токен — трактуем как кражу: ревокаем все сессии юзера.
func (s *Service) Refresh(ctx context.Context, refreshToken, ua, ip string) (*TokenPair, error) {
	hash := hashToken(refreshToken)
	rec, revokedAt, revoked, err := s.store.GetRefreshToken(ctx, hash)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, ErrInvalidToken
		}
		return nil, err
	}

	if revoked {
		// Reuse: кто-то предъявил украденный/уже-использованный токен.
		// Отзываем все активные сессии этого юзера — это лучшая защита.
		if err := s.store.RevokeAllUserTokens(ctx, rec.UserID); err != nil {
			return nil, fmt.Errorf("revoke all after reuse: %w", err)
		}
		_ = revokedAt // оставлено для логов
		return nil, ErrTokenReuse
	}

	if time.Now().After(rec.ExpiresAt) {
		return nil, ErrExpired
	}

	u, err := s.store.FindUserByID(ctx, rec.UserID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, ErrInvalidToken
		}
		return nil, err
	}

	// Ротация: ревокаем старый → выдаём новый.
	if err := s.store.RevokeRefreshToken(ctx, hash); err != nil {
		return nil, err
	}
	return s.issuePair(ctx, u, ua, ip)
}

// Logout ревокает refresh. Access-токен продолжит жить до exp — это ок:
// access короткоживущий (15m), а /auth/me на сервере не проверяет revoked.
// Если нужна жёсткая инвалидация access — вводим blacklist access по jti (Redis).
func (s *Service) Logout(ctx context.Context, refreshToken string) error {
	hash := hashToken(refreshToken)
	err := s.store.RevokeRefreshToken(ctx, hash)
	if err != nil {
		return err
	}
	return nil
}

// Me возвращает актуальные данные юзера. Тянем из БД, чтобы роли/скоупы
// могли измениться без релогина.
func (s *Service) Me(ctx context.Context, userID int64) (*MeResponse, error) {
	u, err := s.store.FindUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &MeResponse{
		ID:     u.ID,
		Login:  u.Login,
		Name:   u.Name,
		Role:   u.Role.Name,
		Scopes: u.EffectiveScopes(),
	}, nil
}

// VerifyAccess парсит access-JWT. Используется middleware.
func (s *Service) VerifyAccess(tokenString string) (*authz.Claims, error) {
	claims := &authz.Claims{}
	tok, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (any, error) {
		if t.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, fmt.Errorf("unexpected signing method: %s", t.Method.Alg())
		}
		return s.cfg.JWTSecret, nil
	})
	if err != nil || !tok.Valid {
		return nil, authz.ErrInvalidToken
	}
	if claims.Type != TypeAccess {
		return nil, authz.ErrInvalidToken
	}
	return claims, nil
}

// issuePair создаёт access (JWT) + refresh (opaque, hash в БД).
func (s *Service) issuePair(ctx context.Context, u *User, ua, ip string) (*TokenPair, error) {
	now := time.Now()
	accessExp := now.Add(s.cfg.AccessTTL)
	claims := &authz.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    s.cfg.Issuer,
			Subject:   strconv.FormatInt(u.ID, 10),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(accessExp),
		},
		UserID: u.ID,
		Role:   u.Role.Name,
		Scopes: u.EffectiveScopes(),
		Type:   TypeAccess,
	}
	access, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.cfg.JWTSecret)
	if err != nil {
		return nil, fmt.Errorf("sign access: %w", err)
	}

	rawRefresh, hash, err := newRefreshToken()
	if err != nil {
		return nil, err
	}
	rec := &RefreshTokenRecord{
		UserID:    u.ID,
		Hash:      hash,
		ExpiresAt: now.Add(s.cfg.RefreshTTL),
	}
	if err := s.store.CreateRefreshToken(ctx, rec, ua, ip); err != nil {
		return nil, err
	}

	return &TokenPair{
		AccessToken:  access,
		RefreshToken: rawRefresh,
		TokenType:    "Bearer",
		ExpiresIn:    int64(time.Until(accessExp).Seconds()),
	}, nil
}

// ---- helpers ----

func newRefreshToken() (raw string, hash []byte, err error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", nil, fmt.Errorf("rand: %w", err)
	}
	raw = base64.RawURLEncoding.EncodeToString(buf)
	h := sha256.Sum256([]byte(raw))
	return raw, h[:], nil
}

func hashToken(raw string) []byte {
	h := sha256.Sum256([]byte(raw))
	return h[:]
}