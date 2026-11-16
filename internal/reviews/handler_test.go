package reviews

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"task-manager/internal/authz"

	"github.com/go-chi/chi/v5"
)

// fakeStore — in-memory реализация Store.
// Позволяет тестировать service+handler без Postgres/Docker.
type fakeStore struct {
	mu     sync.Mutex
	nextID int64
	items  map[int64]*Review
}

type noopVerifier struct {
	claims *authz.Claims
}

func (v noopVerifier) VerifyAccess(string) (*authz.Claims, error) {
	if v.claims == nil {
		return nil, authz.ErrInvalidToken
	}
	return v.claims, nil
}

func newFakeStore() *fakeStore {
	return &fakeStore{nextID: 1, items: make(map[int64]*Review)}
}

func (f *fakeStore) Create(_ context.Context, r *Review) (*Review, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := *r
	cp.ID = f.nextID
	cp.CreatedAt = time.Now()
	f.nextID++
	f.items[cp.ID] = &cp
	return &cp, nil
}

func (f *fakeStore) GetByID(_ context.Context, id int64) (*Review, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.items[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *r
	return &cp, nil
}

func (f *fakeStore) List(_ context.Context, flt ListFilter) ([]Review, int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	var out []Review
	for _, r := range f.items {
		if flt.StatusID > 0 && r.StatusID != flt.StatusID {
			continue
		}
		cp := *r
		out = append(out, cp)
	}
	total := len(out)

	// пагинация — как в реальном store
	limit := flt.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	offset := flt.Offset
	if offset < 0 {
		offset = 0
	}
	if offset >= len(out) {
		return []Review{}, total, nil
	}
	end := offset + limit
	if end > len(out) {
		end = len(out)
	}
	return out[offset:end], total, nil
}

func (f *fakeStore) Update(_ context.Context, r *Review) (*Review, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.items[r.ID]; !ok {
		return nil, ErrNotFound
	}
	cp := *r
	f.items[r.ID] = &cp
	return &cp, nil
}

func (f *fakeStore) UpdateStatus(_ context.Context, id, statusID int64) (*Review, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.items[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *r
	cp.StatusID = statusID
	f.items[id] = &cp
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

func (f *fakeStore) FindStale(_ context.Context, threshold time.Duration) ([]Review, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cutoff := time.Now().Add(-threshold)
	var out []Review
	for _, r := range f.items {
		if r.StatusID == StatusNew && r.CreatedAt.Before(cutoff) {
			cp := *r
			out = append(out, cp)
		}
	}
	return out, nil
}

func setupTestRouter(t *testing.T) http.Handler {
	t.Helper()

	svc := NewService(newFakeStore(), nil)
	handler := NewHandler(svc, noopVerifier{})

	// Router() локальный (без префикса), поэтому в тестах монтируем
	// его под тот же префикс, что и в main.
	root := chi.NewRouter()
	root.Mount("/api/v1/reviews", handler.Router())
	return root
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

func TestCreateReview(t *testing.T) {
	router := setupTestRouter(t)

	tests := []struct {
		name     string
		payload  string
		expected int
	}{
		{
			name:     "success",
			payload:  `{"key_id":1,"sender_id":1,"type_forum_id":1,"title":"Test","description":"desc","tags":["bug"]}`,
			expected: http.StatusCreated,
		},
		{
			name:     "invalid JSON",
			payload:  `{"key_id":1,`,
			expected: http.StatusBadRequest,
		},
		{
			name:     "missing title",
			payload:  `{"key_id":1,"sender_id":1,"type_forum_id":1}`,
			expected: http.StatusBadRequest,
		},
		{
			name:     "unknown field",
			payload:  `{"key_id":1,"sender_id":1,"type_forum_id":1,"title":"T","bogus":"x"}`,
			expected: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := doJSON(t, router, http.MethodPost, "/api/v1/reviews", tt.payload)
			if rec.Code != tt.expected {
				t.Fatalf("expected %d, got %d: %s", tt.expected, rec.Code, rec.Body.String())
			}
			if tt.expected == http.StatusCreated {
				var r Review
				if err := json.NewDecoder(rec.Body).Decode(&r); err != nil {
					t.Fatalf("decode response: %v", err)
				}
				if r.ID == 0 {
					t.Errorf("expected non-zero ID in response")
				}
				if r.StatusID != StatusNew {
					t.Errorf("expected initial status new (%d), got %d", StatusNew, r.StatusID)
				}
			}
		})
	}
}

func TestChangeStatus(t *testing.T) {
	// Отдельный router с правами feedback:write — иначе защищённые
	// роуты (POST /{id}/status) отвечают 401.
	svc := NewService(newFakeStore(), nil)
	verifier := noopVerifier{claims: &authz.Claims{
		UserID: 1,
		Role:   "moderator",
		Scopes: []string{"feedback:write"},
	}}
	handler := NewHandler(svc, verifier)

	root := chi.NewRouter()
	root.Mount("/api/v1/reviews", handler.Router())
	router := root

	// создаём отзыв
	rec := doJSON(t, router, http.MethodPost, "/api/v1/reviews",
		`{"key_id":1,"sender_id":1,"type_forum_id":1,"title":"T"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("setup: %d %s", rec.Code, rec.Body.String())
	}
	var created Review
	if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// new → in_progress: разрешено
	rec = doJSON(t, router, http.MethodPost,
		fmt.Sprintf("/api/v1/reviews/%d/status", created.ID), `{"status_id":2}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("valid transition: %d %s", rec.Code, rec.Body.String())
	}

	// in_progress → new: запрещено
	rec = doJSON(t, router, http.MethodPost,
		fmt.Sprintf("/api/v1/reviews/%d/status", created.ID), `{"status_id":1}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("invalid transition: expected 409, got %d: %s", rec.Code, rec.Body.String())
	}

	// in_progress → resolved: разрешено
	rec = doJSON(t, router, http.MethodPost,
		fmt.Sprintf("/api/v1/reviews/%d/status", created.ID), `{"status_id":3}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("resolved transition: %d %s", rec.Code, rec.Body.String())
	}

	// resolved → что угодно: терминальный статус, запрещено
	rec = doJSON(t, router, http.MethodPost,
		fmt.Sprintf("/api/v1/reviews/%d/status", created.ID), `{"status_id":2}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("terminal transition: expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestList(t *testing.T) {
	router := setupTestRouter(t)
	for i := 0; i < 3; i++ {
		rec := doJSON(t, router, http.MethodPost, "/api/v1/reviews",
			fmt.Sprintf(`{"key_id":%d,"sender_id":1,"type_forum_id":1,"title":"T%d"}`, i+1, i+1))
		if rec.Code != http.StatusCreated {
			t.Fatalf("setup item %d: %d %s", i, rec.Code, rec.Body.String())
		}
	}

	rec := doJSON(t, router, http.MethodGet, "/api/v1/reviews?limit=2", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list: %d %s", rec.Code, rec.Body.String())
	}
	var page ListResponse
	if err := json.NewDecoder(rec.Body).Decode(&page); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if page.Total != 3 {
		t.Errorf("expected total=3, got %d", page.Total)
	}
	if len(page.Items) != 2 {
		t.Errorf("expected 2 items (limit), got %d", len(page.Items))
	}
}