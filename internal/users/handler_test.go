package users

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"task-manager/internal/auth"
)

type fakeStore struct {
	mu        sync.Mutex
	nextID    int64
	users     map[int64]*User
	logins    map[string]bool
	passwords map[int64]string // храним как есть для проверки — в реальном было бы bcrypt
	roles     []Role
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		nextID:    1,
		users:     map[int64]*User{},
		logins:    map[string]bool{},
		passwords: map[int64]string{},
		roles: []Role{
			{ID: 1, Name: "super_admin", Scopes: []string{"*:*"}},
			{ID: 2, Name: "admin", Scopes: []string{"feedback:admin"}},
			{ID: 5, Name: "client", Scopes: []string{"feedback:write"}},
		},
	}
}

func (f *fakeStore) roleByID(id int64) (Role, bool) {
	for _, r := range f.roles {
		if r.ID == id {
			return r, true
		}
	}
	return Role{}, false
}

func (f *fakeStore) Create(_ context.Context, u *User, password string) (*User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.logins[u.Login] {
		return nil, ErrLoginExists
	}
	role, ok := f.roleByID(u.RoleID)
	if !ok {
		return nil, ErrRoleNotFound
	}
	cp := *u
	cp.ID = f.nextID
	cp.Role = role.Name
	cp.CreatedAt = time.Now()
	if cp.Scopes == nil {
		cp.Scopes = []string{}
	}
	f.nextID++
	f.users[cp.ID] = &cp
	f.logins[cp.Login] = true
	f.passwords[cp.ID] = password
	return &cp, nil
}

func (f *fakeStore) GetByID(_ context.Context, id int64) (*User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	u, ok := f.users[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *u
	return &cp, nil
}

func (f *fakeStore) List(_ context.Context, flt ListFilter) ([]User, int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []User
	for _, u := range f.users {
		if flt.RoleID > 0 && u.RoleID != flt.RoleID {
			continue
		}
		out = append(out, *u)
	}
	return out, len(out), nil
}

func (f *fakeStore) Update(_ context.Context, u *User, password *string) (*User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cur, ok := f.users[u.ID]
	if !ok {
		return nil, ErrNotFound
	}
	role, ok := f.roleByID(u.RoleID)
	if !ok {
		return nil, ErrRoleNotFound
	}
	cp := *cur
	cp.Name = u.Name
	cp.RoleID = u.RoleID
	cp.Role = role.Name
	cp.Scopes = u.Scopes
	f.users[u.ID] = &cp
	if password != nil {
		f.passwords[u.ID] = *password
	}
	return &cp, nil
}

func (f *fakeStore) Delete(_ context.Context, id int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	u, ok := f.users[id]
	if !ok {
		return ErrNotFound
	}
	delete(f.users, id)
	delete(f.logins, u.Login)
	return nil
}

func (f *fakeStore) ListRoles(_ context.Context) ([]Role, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.roles, nil
}

type fakeVerifier struct{ claims *auth.Claims }

func (v *fakeVerifier) VerifyAccess(string) (*auth.Claims, error) {
	if v.claims == nil {
		return nil, auth.ErrInvalidToken
	}
	return v.claims, nil
}

func setup(t *testing.T, role string) (http.Handler, *fakeStore) {
	t.Helper()
	store := newFakeStore()
	svc := NewService(store)
	verifier := &fakeVerifier{claims: &auth.Claims{UserID: 1, Role: role}}
	return NewHandler(svc, verifier).Router(), store
}

func doJSON(t *testing.T, router http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer any")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestRequireSuperAdmin(t *testing.T) {
	// moderator пытается создать пользователя → 403
	router, _ := setup(t, "moderator")
	rec := doJSON(t, router, http.MethodPost, "/",
		`{"login":"x","password":"12345678","name":"X","role_id":5}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("moderator: want 403, got %d: %s", rec.Code, rec.Body.String())
	}

	// super_admin — 201
	router, _ = setup(t, "super_admin")
	rec = doJSON(t, router, http.MethodPost, "/",
		`{"login":"ivan","password":"12345678","name":"Ivan","role_id":5}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("super_admin: %d %s", rec.Code, rec.Body.String())
	}
}

func TestCreateUser_Validation(t *testing.T) {
	router, _ := setup(t, "super_admin")

	cases := []struct {
		name    string
		payload string
		want    int
	}{
		{
			name:    "success",
			payload: `{"login":"user_ok","password":"password1","name":"OK","role_id":5}`,
			want:    http.StatusCreated,
		},
		{
			name:    "short login",
			payload: `{"login":"a","password":"password1","name":"X","role_id":5}`,
			want:    http.StatusBadRequest,
		},
		{
			name:    "short password",
			payload: `{"login":"user_pw","password":"123","name":"X","role_id":5}`,
			want:    http.StatusBadRequest,
		},
		{
			name:    "unknown role",
			payload: `{"login":"user_badrole","password":"password1","name":"X","role_id":9999}`,
			want:    http.StatusBadRequest,
		},
		{
			name:    "duplicate login",
			payload: `{"login":"user_ok","password":"password1","name":"X","role_id":5}`,
			want:    http.StatusConflict,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := doJSON(t, router, http.MethodPost, "/", c.payload)
			if rec.Code != c.want {
				t.Fatalf("expected %d, got %d: %s", c.want, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestDeleteSelf(t *testing.T) {
	router, _ := setup(t, "super_admin")
	// создаём себя же под id=1
	rec := doJSON(t, router, http.MethodPost, "/",
		`{"login":"root","password":"password1","name":"Root","role_id":1}`)
	var u User
	_ = json.NewDecoder(rec.Body).Decode(&u)

	// удаление себя → 403
	rec = doJSON(t, router, http.MethodDelete, "/"+itoa(u.ID), "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("delete self: want 403, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestListRoles(t *testing.T) {
	router, _ := setup(t, "super_admin")
	rec := doJSON(t, router, http.MethodGet, "/roles", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list roles: %d %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Items []Role `json:"items"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if len(resp.Items) == 0 {
		t.Errorf("expected non-empty roles list")
	}
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}