package rooms

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
	mu      sync.Mutex
	nextID  int64
	rooms   map[int64]*Room
	avail   map[string]*Availability // key: "roomID|YYYY-MM-DD"
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		nextID: 1,
		rooms:  map[int64]*Room{},
		avail:  map[string]*Availability{},
	}
}

func (f *fakeStore) Create(_ context.Context, r *Room) (*Room, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := *r
	cp.ID = f.nextID
	cp.CreatedAt = time.Now()
	f.nextID++
	f.rooms[cp.ID] = &cp
	return &cp, nil
}

func (f *fakeStore) GetByID(_ context.Context, id int64) (*Room, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.rooms[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *r
	return &cp, nil
}

func (f *fakeStore) List(_ context.Context, flt ListFilter) ([]Room, int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []Room
	for _, r := range f.rooms {
		if flt.Building != "" && r.Building != flt.Building {
			continue
		}
		if flt.Floor != 0 && r.Floor != flt.Floor {
			continue
		}
		cp := *r
		out = append(out, cp)
	}
	return out, len(out), nil
}

func (f *fakeStore) Update(_ context.Context, r *Room) (*Room, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.rooms[r.ID]; !ok {
		return nil, ErrNotFound
	}
	cp := *r
	f.rooms[r.ID] = &cp
	return &cp, nil
}

func (f *fakeStore) Delete(_ context.Context, id int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.rooms[id]; !ok {
		return ErrNotFound
	}
	delete(f.rooms, id)
	return nil
}

func (f *fakeStore) GetAvailability(_ context.Context, roomID int64, date time.Time) (*Availability, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := availKey(roomID, date.Format("2006-01-02"))
	a, ok := f.avail[key]
	if !ok {
		return nil, ErrAvailabilityNotFound
	}
	cp := *a
	return &cp, nil
}

func (f *fakeStore) SetAvailability(_ context.Context, roomID int64, date time.Time, status AvailabilityStatus, _ int64) (*Availability, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	a := &Availability{
		RoomID:    roomID,
		Date:      date.Format("2006-01-02"),
		Status:    status,
		UpdatedAt: time.Now(),
	}
	f.avail[availKey(roomID, a.Date)] = a
	cp := *a
	return &cp, nil
}

func availKey(roomID int64, d string) string {
	return string(rune(roomID)) + "|" + d
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
	req.Header.Set("Authorization", "Bearer any")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// ---- tests ----

func TestCreateRoom_RequiresScope(t *testing.T) {
	payload := `{"number":"A101","floor":1,"building":"Main","longitude":4.9,"latitude":52.3,"description":"lab"}`

	// без map:write → 403
	router := setupRouter(t, []string{"map:read"})
	rec := doJSON(t, router, http.MethodPost, "/", payload)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rec.Code, rec.Body.String())
	}

	// moderator (map:write) → 201
	router = setupRouter(t, []string{"map:read", "map:write"})
	rec = doJSON(t, router, http.MethodPost, "/", payload)
	if rec.Code != http.StatusCreated {
		t.Fatalf("moderator create: expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	// admin (map:admin покрывает write) → 201
	router = setupRouter(t, []string{"map:admin"})
	rec = doJSON(t, router, http.MethodPost, "/", payload)
	if rec.Code != http.StatusCreated {
		t.Fatalf("admin create: expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCreateRoom_Validation(t *testing.T) {
	router := setupRouter(t, []string{"map:admin"})

	cases := []struct {
		name    string
		payload string
		want    int
	}{
		{
			name:    "success",
			payload: `{"number":"B202","floor":2,"building":"North","longitude":4.8,"latitude":52.4}`,
			want:    http.StatusCreated,
		},
		{
			name:    "missing number",
			payload: `{"floor":2,"building":"North"}`,
			want:    http.StatusBadRequest,
		},
		{
			name:    "missing building",
			payload: `{"number":"B202","floor":2}`,
			want:    http.StatusBadRequest,
		},
		{
			name:    "lon out of range",
			payload: `{"number":"B202","building":"N","longitude":200,"latitude":52}`,
			want:    http.StatusBadRequest,
		},
		{
			name:    "invalid JSON",
			payload: `{"number":`,
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

func TestListRooms_Public(t *testing.T) {
	store := newFakeStore()
	adminRouter := NewHandler(NewService(store), &fakeVerifier{claims: &authz.Claims{UserID: 1, Scopes: []string{"map:admin"}}}).Router()
	publicRouter := NewHandler(NewService(store), &fakeVerifier{}).Router()

	mk := func(number, building string, floor int) {
		payload := `{"number":"` + number + `","floor":` + itoa(int64(floor)) +
			`,"building":"` + building + `","longitude":0,"latitude":0}`
		rec := doJSON(t, adminRouter, http.MethodPost, "/", payload)
		if rec.Code != http.StatusCreated {
			t.Fatalf("setup %s: %d %s", number, rec.Code, rec.Body.String())
		}
	}
	mk("A1", "Main", 1)
	mk("A2", "Main", 2)
	mk("B1", "North", 1)

	rec := doJSON(t, publicRouter, http.MethodGet, "/", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list: %d %s", rec.Code, rec.Body.String())
	}
	var page ListResponse
	if err := json.NewDecoder(rec.Body).Decode(&page); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if page.Total != 3 {
		t.Errorf("total: want 3, got %d", page.Total)
	}

	// ?building=Main
	rec = doJSON(t, publicRouter, http.MethodGet, "/?building=Main", "")
	if err := json.NewDecoder(rec.Body).Decode(&page); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if page.Total != 2 {
		t.Errorf("building filter: want 2, got %d", page.Total)
	}
}

func TestAvailability(t *testing.T) {
	router := setupRouter(t, []string{"map:admin"})

	// создаём комнату
	rec := doJSON(t, router, http.MethodPost, "/",
		`{"number":"A101","floor":1,"building":"Main","longitude":4.9,"latitude":52.3}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("setup: %d %s", rec.Code, rec.Body.String())
	}
	var room Room
	_ = json.NewDecoder(rec.Body).Decode(&room)
	id := itoa(room.ID)

	// GET availability — default free (записи нет)
	rec = doJSON(t, router, http.MethodGet, "/"+id+"/availability?date=2026-06-01", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("get default: %d %s", rec.Code, rec.Body.String())
	}
	var a Availability
	_ = json.NewDecoder(rec.Body).Decode(&a)
	if a.Status != StatusFree {
		t.Errorf("default: want free, got %s", a.Status)
	}

	// PATCH — ставим busy
	rec = doJSON(t, router, http.MethodPatch, "/"+id+"/availability",
		`{"date":"2026-06-01","status":"busy"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch: %d %s", rec.Code, rec.Body.String())
	}
	_ = json.NewDecoder(rec.Body).Decode(&a)
	if a.Status != StatusBusy {
		t.Errorf("patch: want busy, got %s", a.Status)
	}

	// GET — теперь busy
	rec = doJSON(t, router, http.MethodGet, "/"+id+"/availability?date=2026-06-01", "")
	_ = json.NewDecoder(rec.Body).Decode(&a)
	if a.Status != StatusBusy {
		t.Errorf("after patch: want busy, got %s", a.Status)
	}

	// PATCH на другую дату — независимо
	rec = doJSON(t, router, http.MethodPatch, "/"+id+"/availability",
		`{"date":"2026-06-02","status":"busy"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch other date: %d", rec.Code)
	}
	// 06-01 всё ещё busy
	rec = doJSON(t, router, http.MethodGet, "/"+id+"/availability?date=2026-06-01", "")
	_ = json.NewDecoder(rec.Body).Decode(&a)
	if a.Status != StatusBusy {
		t.Errorf("isolation: 06-01 must still be busy, got %s", a.Status)
	}

	// PATCH невалидный статус → 400
	rec = doJSON(t, router, http.MethodPatch, "/"+id+"/availability",
		`{"date":"2026-06-01","status":"maybe"}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("invalid status: want 400, got %d", rec.Code)
	}

	// PATCH невалидная дата → 400
	rec = doJSON(t, router, http.MethodPatch, "/"+id+"/availability",
		`{"date":"01.06.2026","status":"free"}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("invalid date: want 400, got %d", rec.Code)
	}

	// GET на несуществующую комнату → 404
	rec = doJSON(t, router, http.MethodGet, "/99999/availability?date=2026-06-01", "")
	if rec.Code != http.StatusNotFound {
		t.Errorf("unknown room: want 404, got %d", rec.Code)
	}
}

func TestAvailability_RequiresScope(t *testing.T) {
	router := setupRouter(t, []string{"map:read"}) // есть read, нет write
	rec := doJSON(t, router, http.MethodPatch, "/1/availability",
		`{"date":"2026-06-01","status":"busy"}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rec.Code, rec.Body.String())
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