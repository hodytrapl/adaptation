package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"golang.org/x/crypto/bcrypt"
)

// fakeStore — in-memory реализация Store.
type fakeStore struct {
	mu     sync.Mutex
	users  map[string]*User
	tokens map[string]*fakeToken // key: string(hash)
}

type fakeToken struct {
	rec       *RefreshTokenRecord
	revokedAt time.Time
	revoked   bool
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		users:  map[string]*User{},
		tokens: map[string]*fakeToken{},
	}
}

func (f *fakeStore) addUser(u *User) { f.users[u.Login] = u }

func (f *fakeStore) FindUserByLogin(_ context.Context, login string) (*User, error) {
	if u, ok := f.users[login]; ok {
		return u, nil
	}
	return nil, ErrNotFound
}

func (f *fakeStore) FindUserByID(_ context.Context, id int64) (*User, error) {
	for _, u := range f.users {
		if u.ID == id {
			return u, nil
		}
	}
	return nil, ErrNotFound
}

func (f *fakeStore) CreateRefreshToken(_ context.Context, rt *RefreshTokenRecord, _, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tokens[string(rt.Hash)] = &fakeToken{rec: rt}
	return nil
}

func (f *fakeStore) GetRefreshToken(_ context.Context, hash []byte) (*RefreshTokenRecord, time.Time, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.tokens[string(hash)]
	if !ok {
		return nil, time.Time{}, false, ErrNotFound
	}
	return t.rec, t.revokedAt, t.revoked, nil
}

func (f *fakeStore) RevokeRefreshToken(_ context.Context, hash []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if t, ok := f.tokens[string(hash)]; ok {
		t.revoked = true
		t.revokedAt = time.Now()
	}
	return nil
}

func (f *fakeStore) RevokeAllUserTokens(_ context.Context, userID int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, t := range f.tokens {
		if t.rec.UserID == userID {
			t.revoked = true
			t.revokedAt = time.Now()
		}
	}
	return nil
}

// ---- setup ----

func setupAuth(t *testing.T) (http.Handler, *fakeStore, *Service) {
	t.Helper()

	fs := newFakeStore()
	hash, err := bcrypt.GenerateFromPassword([]byte("admin123"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	fs.addUser(&User{
		ID:           1,
		Login:        "admin",
		Name:         "Administrator",
		PasswordHash: string(hash),
		Role:         Role{ID: 1, Name: "super_admin", Scopes: []string{"*:*"}},
	})

	svc := NewService(fs, Config{
		JWTSecret:  []byte("test-secret-32-bytes-long-padding!"),
		Issuer:     "test",
		AccessTTL:  15 * time.Minute,
		RefreshTTL: 24 * time.Hour,
	})

	h := NewHandler(svc)

	// Собираем root-роутер так же, как в main: /login и т.д. под /api/v1/auth.
	root := chi.NewRouter()
	root.Route("/api/v1/auth", func(r chi.Router) {
		r.Post("/login", h.LoginHandler())
		r.Post("/refresh", h.RefreshHandler())
		r.Post("/logout", h.LogoutHandler())
		r.Get("/me", h.MeHandler())
	})
	return root, fs, svc
}

func doJSON(t *testing.T, router http.Handler, method, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// ---- tests ----

func TestLoginAndMe(t *testing.T) {
	router, _, svc := setupAuth(t)

	// 1) login success
	rec := doJSON(t, router, http.MethodPost, "/api/v1/auth/login",
		`{"login":"admin","password":"admin123"}`, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("login: %d %s", rec.Code, rec.Body.String())
	}
	var pair TokenPair
	if err := json.NewDecoder(rec.Body).Decode(&pair); err != nil {
		t.Fatal(err)
	}
	if pair.AccessToken == "" || pair.RefreshToken == "" {
		t.Fatal("expected both tokens")
	}

	// 2) verify access через сервис (эмулируем middleware)
	claims, err := svc.VerifyAccess(pair.AccessToken)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if claims.UserID != 1 {
		t.Errorf("expected uid=1, got %d", claims.UserID)
	}
	if !claims.HasScope("feedback:write") {
		t.Errorf("super_admin должен иметь все scopes, has=%v", claims.Scopes)
	}

	// 3) login wrong password → 401
	rec = doJSON(t, router, http.MethodPost, "/api/v1/auth/login",
		`{"login":"admin","password":"nope"}`, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong password: expected 401, got %d", rec.Code)
	}

	// 4) login unknown user → 401 (без раскрытия)
	rec = doJSON(t, router, http.MethodPost, "/api/v1/auth/login",
		`{"login":"ghost","password":"whatever"}`, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unknown user: expected 401, got %d", rec.Code)
	}
}

func TestRefreshRotationAndReuse(t *testing.T) {
	router, _, _ := setupAuth(t)

	// login
	rec := doJSON(t, router, http.MethodPost, "/api/v1/auth/login",
		`{"login":"admin","password":"admin123"}`, nil)
	var first TokenPair
	_ = json.NewDecoder(rec.Body).Decode(&first)

	// refresh — ротация
	rec = doJSON(t, router, http.MethodPost, "/api/v1/auth/refresh",
		`{"refresh_token":"`+first.RefreshToken+`"}`, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("refresh: %d %s", rec.Code, rec.Body.String())
	}
	var second TokenPair
	_ = json.NewDecoder(rec.Body).Decode(&second)
	if second.RefreshToken == first.RefreshToken {
		t.Fatal("expected rotated refresh token")
	}

	// reuse старого → 401 token_reuse
	rec = doJSON(t, router, http.MethodPost, "/api/v1/auth/refresh",
		`{"refresh_token":"`+first.RefreshToken+`"}`, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("reuse: expected 401, got %d: %s", rec.Code, rec.Body.String())
	}

	// и после reuse-detected все токены отозваны — второй тоже не работает
	rec = doJSON(t, router, http.MethodPost, "/api/v1/auth/refresh",
		`{"refresh_token":"`+second.RefreshToken+`"}`, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("after theft: expected 401, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestLogout(t *testing.T) {
	router, _, _ := setupAuth(t)

	rec := doJSON(t, router, http.MethodPost, "/api/v1/auth/login",
		`{"login":"admin","password":"admin123"}`, nil)
	var pair TokenPair
	_ = json.NewDecoder(rec.Body).Decode(&pair)

	rec = doJSON(t, router, http.MethodPost, "/api/v1/auth/logout",
		`{"refresh_token":"`+pair.RefreshToken+`"}`, nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("logout: %d %s", rec.Code, rec.Body.String())
	}

	// refresh после logout → 401
	rec = doJSON(t, router, http.MethodPost, "/api/v1/auth/refresh",
		`{"refresh_token":"`+pair.RefreshToken+`"}`, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("refresh after logout: expected 401, got %d", rec.Code)
	}
}

func TestRequireScope_Unit(t *testing.T) {
	// проверяем логику HasScope через фиктивные claims — не нужно поднимать HTTP
	mod := &Claims{Scopes: []string{
		"feedback:read", "feedback:write",
		"map:read", "map:write",
		"teachers:read", "teachers:write",
		"events:read", "events:write",
		"qa:read", "qa:write",
	}}
	if !mod.HasScope("feedback:write") {
		t.Error("moderator must have feedback:write")
	}
	if mod.HasScope("users:admin") {
		t.Error("moderator must NOT have users:admin")
	}
	if mod.HasScope("feedback:admin") {
		t.Error("moderator must NOT have feedback:admin")
	}

	admin := &Claims{Scopes: []string{"feedback:admin"}}
	if !admin.HasScope("feedback:read") || !admin.HasScope("feedback:write") {
		t.Error("module:admin must imply read+write")
	}

	root := &Claims{Scopes: []string{"*:*"}}
	if !root.HasScope("anything:goes") {
		t.Error("*:* must allow everything")
	}
}