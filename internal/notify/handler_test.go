package notify

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
	mu     sync.Mutex
	nextID int64
	items  map[int64]*Notification
}

func newFakeStore() *fakeStore {
	return &fakeStore{nextID: 1, items: map[int64]*Notification{}}
}

func (f *fakeStore) Create(_ context.Context, n *Notification) (*Notification, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := *n
	cp.ID = f.nextID
	cp.CreatedAt = time.Now()
	f.nextID++
	f.items[cp.ID] = &cp
	return &cp, nil
}

func (f *fakeStore) List(_ context.Context, flt ListFilter) ([]Notification, int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []Notification
	for _, n := range f.items {
		if n.UserID != flt.UserID {
			continue
		}
		if flt.Unread && n.ReadAt != nil {
			continue
		}
		out = append(out, *n)
	}
	return out, len(out), nil
}

func (f *fakeStore) MarkRead(_ context.Context, id, userID int64) (*Notification, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	n, ok := f.items[id]
	if !ok || n.UserID != userID {
		return nil, ErrNotFound
	}
	cp := *n
	now := time.Now()
	if cp.ReadAt == nil {
		cp.ReadAt = &now
	}
	f.items[id] = &cp
	return &cp, nil
}

type fakeVerifier struct{ claims *auth.Claims }

func (v *fakeVerifier) VerifyAccess(string) (*auth.Claims, error) {
	if v.claims == nil {
		return nil, auth.ErrInvalidToken
	}
	return v.claims, nil
}

func setup(t *testing.T, userID int64) (http.Handler, *fakeStore) {
	t.Helper()
	store := newFakeStore()
	svc := NewService(store)
	verifier := &fakeVerifier{claims: &auth.Claims{UserID: userID, Role: "client"}}
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

func TestList_OnlyOwn(t *testing.T) {
	router, store := setup(t, 100)

	// сеем два уведомления: одно моё (100), одно чужое (200)
	_, _ = store.Create(context.Background(), &Notification{UserID: 100, Kind: KindSystem})
	_, _ = store.Create(context.Background(), &Notification{UserID: 200, Kind: KindSystem})

	rec := doJSON(t, router, http.MethodGet, "/", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list: %d %s", rec.Code, rec.Body.String())
	}
	var page ListResponse
	_ = json.NewDecoder(rec.Body).Decode(&page)
	if page.Total != 1 {
		t.Errorf("own notifications: want 1, got %d", page.Total)
	}
}

func TestMarkRead_IDOR(t *testing.T) {
	router, store := setup(t, 100)

	// чужое уведомление
	other, _ := store.Create(context.Background(), &Notification{UserID: 200, Kind: KindSystem})

	rec := doJSON(t, router, http.MethodPatch, "/"+itoa(other.ID)+"/read", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("foreign mark read: want 404, got %d", rec.Code)
	}

	// своё
	own, _ := store.Create(context.Background(), &Notification{UserID: 100, Kind: KindSystem})
	rec = doJSON(t, router, http.MethodPatch, "/"+itoa(own.ID)+"/read", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("own mark read: %d %s", rec.Code, rec.Body.String())
	}
	var n Notification
	_ = json.NewDecoder(rec.Body).Decode(&n)
	if n.ReadAt == nil {
		t.Errorf("expected read_at set, got nil")
	}

	// идемпотентность — повторный PATCH не падает
	rec = doJSON(t, router, http.MethodPatch, "/"+itoa(own.ID)+"/read", "")
	if rec.Code != http.StatusOK {
		t.Errorf("idempotent mark read: want 200, got %d", rec.Code)
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