package events

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"task-manager/internal/authz"
	"task-manager/internal/auth"
)

// ---- fake store ----

type fakeStore struct {
	mu     sync.Mutex
	nextID int64
	items  map[int64]*Event
}

func newFakeStore() *fakeStore {
	return &fakeStore{nextID: 1, items: map[int64]*Event{}}
}

func (f *fakeStore) Create(_ context.Context, e *Event) (*Event, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := *e
	cp.ID = f.nextID
	cp.CreatedAt = time.Now()
	f.nextID++
	f.items[cp.ID] = &cp
	return &cp, nil
}

func (f *fakeStore) GetByID(_ context.Context, id int64) (*Event, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	e, ok := f.items[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *e
	return &cp, nil
}

func (f *fakeStore) List(_ context.Context, flt ListFilter) ([]Event, int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []Event
	for _, e := range f.items {
		if !flt.From.IsZero() && e.DateStarted.Before(flt.From) {
			continue
		}
		if !flt.To.IsZero() && e.DateStarted.After(flt.To) {
			continue
		}
		cp := *e
		out = append(out, cp)
	}
	// стабильный порядок для тестов
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].DateStarted.Before(out[i].DateStarted) {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out, len(out), nil
}

func (f *fakeStore) Update(_ context.Context, e *Event) (*Event, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.items[e.ID]; !ok {
		return nil, ErrNotFound
	}
	cp := *e
	f.items[e.ID] = &cp
	return &cp, nil
}

func (f *fakeStore) Delete(_ context.Context, id int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.items[id]; !ok {
		return ErrNotFound
	}
	delete(f.items, id)
	return nil
}

// ---- fake verifier ----

type fakeVerifier struct {
	claims *auth.Claims
	err    error
}

func (v *fakeVerifier) VerifyAccess(string) (*auth.Claims, error) {
	if v.err != nil {
		return nil, v.err
	}
	return v.claims, nil
}

// ---- helpers ----

// setupRouter возвращает router с заданным набором скоупов у «пользователя».
// scopes=nil → любой защищённый роут вернёт 403.
func setupRouter(t *testing.T, scopes []string) http.Handler {
	t.Helper()
	store := newFakeStore()
	svc := NewService(store)
	verifier := &fakeVerifier{claims: &authz.Claims{UserID: 1, Role: "test", Scopes: scopes}}
	return NewHandler(svc, verifier).Router()
}

func doJSON(t *testing.T, router http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	// Авторизация: middleware читает Bearer, fakeVerifier игнорирует значение.
	req.Header.Set("Authorization", "Bearer any")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// ---- tests ----

func TestCreateEvent_RequiresScope(t *testing.T) {
	payload := `{"title":"Go Meetup","description":"talks","date_started":"2026-05-01T18:00:00Z","duration_seconds":7200,"place":"Amsterdam"}`

	// без events:write → 403
	router := setupRouter(t, []string{"events:read"})
	rec := doJSON(t, router, http.MethodPost, "/", payload)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rec.Code, rec.Body.String())
	}

	// moderator (events:write) → 201
	router = setupRouter(t, []string{"events:read", "events:write"})
	rec = doJSON(t, router, http.MethodPost, "/", payload)
	if rec.Code != http.StatusCreated {
		t.Fatalf("moderator create: expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	// admin (events:admin покрывает write) → 201
	router = setupRouter(t, []string{"events:admin"})
	rec = doJSON(t, router, http.MethodPost, "/", payload)
	if rec.Code != http.StatusCreated {
		t.Fatalf("admin create: expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCreateEvent_Validation(t *testing.T) {
	router := setupRouter(t, []string{"events:write"})

	cases := []struct {
		name    string
		payload string
		want    int
	}{
		{
			name:    "success",
			payload: `{"title":"Conf","date_started":"2026-06-01T09:00:00Z","duration_seconds":3600,"place":"Berlin"}`,
			want:    http.StatusCreated,
		},
		{
			name:    "missing title",
			payload: `{"date_started":"2026-06-01T09:00:00Z"}`,
			want:    http.StatusBadRequest,
		},
		{
			name:    "missing date",
			payload: `{"title":"Conf"}`,
			want:    http.StatusBadRequest,
		},
		{
			name:    "negative duration",
			payload: `{"title":"Conf","date_started":"2026-06-01T09:00:00Z","duration_seconds":-1}`,
			want:    http.StatusBadRequest,
		},
		{
			name:    "invalid JSON",
			payload: `{"title":`,
			want:    http.StatusBadRequest,
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

func TestList_PublicWithDateFilter(t *testing.T) {
	// публичный роутер: даже без auth middleware / скоупов
	store := newFakeStore()
	adminVerifier := &fakeVerifier{claims: &authz.Claims{UserID: 1, Scopes: []string{"events:admin"}}}
	adminRouter := NewHandler(NewService(store), adminVerifier).Router()

	// публичный — тот же store, пустой verifier (не нужен)
	publicRouter := NewHandler(NewService(store), &fakeVerifier{}).Router()

	mk := func(title, date string) {
		payload := `{"title":"` + title + `","date_started":"` + date + `","duration_seconds":0,"place":""}`
		rec := doJSON(t, adminRouter, http.MethodPost, "/", payload)
		if rec.Code != http.StatusCreated {
			t.Fatalf("setup %s: %d %s", title, rec.Code, rec.Body.String())
		}
	}
	mk("Jan", "2026-01-15T10:00:00Z")
	mk("Mar", "2026-03-15T10:00:00Z")
	mk("May", "2026-05-15T10:00:00Z")

	// дальше — как было, но через publicRouter
	rec := doJSON(t, publicRouter, http.MethodGet, "/", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list: %d %s", rec.Code, rec.Body.String())
	}
	var page ListResponse
	_ = json.NewDecoder(rec.Body).Decode(&page)
	if page.Total != 3 {
		t.Errorf("total: want 3, got %d", page.Total)
	}

	// from=2026-02-01 — Jan отфильтрован
	rec = doJSON(t, publicRouter, http.MethodGet, "/?from=2026-02-01", "")
	_ = json.NewDecoder(rec.Body).Decode(&page)
	if page.Total != 2 {
		t.Errorf("from filter: want 2, got %d", page.Total)
	}

	// to=2026-04-01 — May отфильтрован
	rec = doJSON(t, publicRouter, http.MethodGet, "/?to=2026-04-01", "")
	_ = json.NewDecoder(rec.Body).Decode(&page)
	if page.Total != 2 {
		t.Errorf("to filter: want 2, got %d", page.Total)
	}

	// from+to — только Mar
	rec = doJSON(t, publicRouter, http.MethodGet, "/?from=2026-02-01&to=2026-04-01", "")
	_ = json.NewDecoder(rec.Body).Decode(&page)
	if page.Total != 1 {
		t.Errorf("range filter: want 1, got %d", page.Total)
	}
	if len(page.Items) > 0 && page.Items[0].Title != "Mar" {
		t.Errorf("range filter: want 'Mar', got %q", page.Items[0].Title)
	}

	// from>to → 400
	rec = doJSON(t, publicRouter, http.MethodGet, "/?from=2026-05-01&to=2026-01-01", "")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("invalid range: want 400, got %d", rec.Code)
	}
}

func TestUpdateDelete(t *testing.T) {
	router := setupRouter(t, []string{"events:admin"})

	rec := doJSON(t, router, http.MethodPost, "/",
		`{"title":"Old","date_started":"2026-07-01T09:00:00Z","duration_seconds":0,"place":"X"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("setup create: %d %s", rec.Code, rec.Body.String())
	}
	var created Event
	_ = json.NewDecoder(rec.Body).Decode(&created)

	// PATCH — частичное обновление
	rec = doJSON(t, router, http.MethodPatch, "/"+itoa(created.ID), `{"title":"New","place":"Y"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("update: %d %s", rec.Code, rec.Body.String())
	}
	var updated Event
	_ = json.NewDecoder(rec.Body).Decode(&updated)
	if updated.Title != "New" || updated.Place != "Y" {
		t.Errorf("update fields: %+v", updated)
	}

	// DELETE
	rec = doJSON(t, router, http.MethodDelete, "/"+itoa(created.ID), "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body.String())
	}

	// GET после DELETE → 404
	rec = doJSON(t, router, http.MethodGet, "/"+itoa(created.ID), "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("get after delete: want 404, got %d", rec.Code)
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