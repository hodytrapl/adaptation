package teachers

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

// ---- fake store ----

type fakeStore struct {
	mu     sync.Mutex
	nextID int64
	items  map[int64]*Teacher
	rooms  map[int64]*RoomRef
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		nextID: 1,
		items:  map[int64]*Teacher{},
		rooms:  map[int64]*RoomRef{},
	}
}

func (f *fakeStore) enrich(t *Teacher) *Teacher {
	cp := *t
	if cp.Room != nil && cp.Room.ID != 0 {
		if r, ok := f.rooms[cp.Room.ID]; ok {
			cp.Room = r
		} else {
			cp.Room = nil
		}
	}
	return &cp
}

func (f *fakeStore) Create(_ context.Context, t *Teacher) (*Teacher, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := *t
	cp.ID = f.nextID
	cp.CreatedAt = time.Now()
	f.nextID++
	f.items[cp.ID] = &cp
	return f.enrich(&cp), nil
}

func (f *fakeStore) GetByID(_ context.Context, id int64) (*Teacher, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.items[id]
	if !ok {
		return nil, ErrNotFound
	}
	return f.enrich(t), nil
}

func (f *fakeStore) List(_ context.Context, flt ListFilter) ([]Teacher, int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []Teacher
	for _, t := range f.items {
		if flt.Department != "" && t.Department != flt.Department {
			continue
		}
		if flt.RoomID > 0 && (t.Room == nil || t.Room.ID != flt.RoomID) {
			continue
		}
		if flt.Query != "" {
			if !containsFold(t.FullName, flt.Query) {
				continue
			}
		}
		out = append(out, *f.enrich(t))
	}
	return out, len(out), nil
}

func (f *fakeStore) Update(_ context.Context, t *Teacher) (*Teacher, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.items[t.ID]; !ok {
		return nil, ErrNotFound
	}
	cp := *t
	f.items[t.ID] = &cp
	return f.enrich(&cp), nil
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

func containsFold(s, sub string) bool {
	ls, lsub := toLower(s), toLower(sub)
	return len(lsub) == 0 || indexOf(ls, lsub) >= 0
}

func toLower(s string) string {
	b := []byte(s)
	for i := range b {
		if b[i] >= 'A' && b[i] <= 'Z' {
			b[i] += 'a' - 'A'
		}
	}
	return string(b)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
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

func setupRouter(t *testing.T, scopes []string) (http.Handler, *fakeStore) {
	t.Helper()
	store := newFakeStore()
	store.rooms[1] = &RoomRef{ID: 1, Number: "A101", Building: "Main"}
	store.rooms[2] = &RoomRef{ID: 2, Number: "B202", Building: "North"}

	svc := NewService(store)
	verifier := &fakeVerifier{claims: &auth.Claims{
		UserID: 7,
		Role:   "moderator",
		Scopes: scopes,
	}}
	return NewHandler(svc, verifier).Router(), store
}

func doJSON(t *testing.T, router http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	t.Logf("--> REQUEST: %s %s | Body: %s", method, path, body)

	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer any")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	t.Logf("<-- RESPONSE: Status %d | Body: %s", rec.Code, rec.Body.String())
	return rec
}

// ---- tests ----

func TestCreateTeacher_RequiresScope(t *testing.T) {
	payload := `{"full_name":"Ivanov I.I.","department":"CS","position":"Professor","contacts":{"email":"i@x.io"}}`

	cases := []struct {
		name   string
		scopes []string
		want   int
	}{
		{"read-only user (forbidden)", []string{"teachers:read"}, http.StatusForbidden},
		{"moderator with write scope (created)", []string{"teachers:read", "teachers:write"}, http.StatusCreated},
		{"admin user (created)", []string{"teachers:admin"}, http.StatusCreated},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Logf("Checking access with scopes: %v", tc.scopes)
			router, _ := setupRouter(t, tc.scopes)
			rec := doJSON(t, router, http.MethodPost, "/", payload)

			if rec.Code != tc.want {
				t.Fatalf("Expected status %d, got %d", tc.want, rec.Code)
			}
			t.Logf("OK: status %d matched expected", rec.Code)
		})
	}
}

func TestCreateTeacher_Validation(t *testing.T) {
	router, _ := setupRouter(t, []string{"teachers:write"})

	cases := []struct {
		name    string
		payload string
		want    int
	}{
		{
			name:    "success",
			payload: `{"full_name":"Petrov P.P.","department":"Math","position":"Docent"}`,
			want:    http.StatusCreated,
		},
		{
			name:    "missing full_name",
			payload: `{"department":"Math","position":"Docent"}`,
			want:    http.StatusBadRequest,
		},
		{
			name:    "missing department",
			payload: `{"full_name":"X","position":"Docent"}`,
			want:    http.StatusBadRequest,
		},
		{
			name:    "bad email",
			payload: `{"full_name":"X","department":"Y","position":"Z","contacts":{"email":"not-an-email"}}`,
			want:    http.StatusBadRequest,
		},
		{
			name:    "bad photo_url",
			payload: `{"full_name":"X","department":"Y","position":"Z","photo_url":"not-a-url"}`,
			want:    http.StatusBadRequest,
		},
		{
			name:    "invalid JSON",
			payload: `{"full_name":`,
			want:    http.StatusBadRequest,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := doJSON(t, router, http.MethodPost, "/", c.payload)
			if rec.Code != c.want {
				t.Fatalf("Expected status %d, got %d", c.want, rec.Code)
			}
			t.Logf("OK: case '%s' validated with status %d", c.name, rec.Code)
		})
	}
}

func TestTeacher_RoomRef(t *testing.T) {
	router, _ := setupRouter(t, []string{"teachers:write"})

	t.Log("Step 1: Creating teacher linked to room ID=1")
	rec := doJSON(t, router, http.MethodPost, "/",
		`{"full_name":"Ivanov I.I.","department":"CS","position":"Professor","room_id":1}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	var created Teacher
	_ = json.NewDecoder(rec.Body).Decode(&created)

	if created.Room == nil {
		t.Fatal("Expected embedded Room struct, got nil")
	}
	if created.Room.Number != "A101" || created.Room.Building != "Main" {
		t.Errorf("Room mismatch: %+v", created.Room)
	}
	t.Logf("OK: Room attached: %s (%s)", created.Room.Number, created.Room.Building)

	t.Logf("Step 2: Unlinking room via PATCH /%d with room_id=0", created.ID)
	rec = doJSON(t, router, http.MethodPatch, "/"+itoa(created.ID), `{"room_id":0}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("unlink: %d %s", rec.Code, rec.Body.String())
	}
	var updated Teacher
	_ = json.NewDecoder(rec.Body).Decode(&updated)
	if updated.Room != nil {
		t.Errorf("Expected room=nil after unlink, got: %+v", updated.Room)
	}
	t.Log("OK: Room unlinked (room=nil)")
}

func TestListTeachers_Public(t *testing.T) {
	adminRouter, store := setupRouter(t, []string{"teachers:admin"})
	pub := NewHandler(NewService(store), &fakeVerifier{}).Router()

	t.Log("Seeding test data (3 teachers)...")
	mk := func(name, dept string, roomID int64) {
		payload := `{"full_name":"` + name + `","department":"` + dept + `","position":"P","room_id":` + itoa(roomID) + `}`
		rec := doJSON(t, adminRouter, http.MethodPost, "/", payload)
		if rec.Code != http.StatusCreated {
			t.Fatalf("setup %s: %d %s", name, rec.Code, rec.Body.String())
		}
	}
	mk("Ivanov I.I.", "CS", 1)
	mk("Petrov P.P.", "CS", 2)
	mk("Sidorov S.S.", "Math", 1)

	t.Run("no filter", func(t *testing.T) {
		rec := doJSON(t, pub, http.MethodGet, "/", "")
		var page ListResponse
		_ = json.NewDecoder(rec.Body).Decode(&page)
		t.Logf("Found records without filter: %d (expected 3)", page.Total)
		if page.Total != 3 {
			t.Errorf("total: want 3, got %d", page.Total)
		}
	})

	t.Run("filter by department=CS", func(t *testing.T) {
		rec := doJSON(t, pub, http.MethodGet, "/?department=CS", "")
		var page ListResponse
		_ = json.NewDecoder(rec.Body).Decode(&page)
		t.Logf("Found records for department=CS: %d (expected 2)", page.Total)
		if page.Total != 2 {
			t.Errorf("department filter: want 2, got %d", page.Total)
		}
	})

	t.Run("filter by room_id=1", func(t *testing.T) {
		rec := doJSON(t, pub, http.MethodGet, "/?room_id=1", "")
		var page ListResponse
		_ = json.NewDecoder(rec.Body).Decode(&page)
		t.Logf("Found records for room_id=1: %d (expected 2)", page.Total)
		if page.Total != 2 {
			t.Errorf("room filter: want 2, got %d", page.Total)
		}
	})

	t.Run("search by q=petr", func(t *testing.T) {
		rec := doJSON(t, pub, http.MethodGet, "/?q=petr", "")
		var page ListResponse
		_ = json.NewDecoder(rec.Body).Decode(&page)
		t.Logf("Found records for search 'petr': %d (expected 1)", page.Total)
		if page.Total != 1 {
			t.Errorf("search: want 1, got %d", page.Total)
		}
	})
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
