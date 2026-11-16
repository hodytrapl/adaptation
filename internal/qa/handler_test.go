package qa

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
)

// ---- fake store ----

type fakeStore struct {
	mu     sync.Mutex
	nextID int64
	items  map[int64]*QA
}

func newFakeStore() *fakeStore {
	return &fakeStore{nextID: 1, items: map[int64]*QA{}}
}

func (f *fakeStore) Create(_ context.Context, q *QA) (*QA, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := *q
	cp.ID = f.nextID
	cp.StatusID = StatusNew
	cp.CreatedAt = time.Now()
	f.nextID++
	f.items[cp.ID] = &cp
	return &cp, nil
}

func (f *fakeStore) GetByID(_ context.Context, id int64) (*QA, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	q, ok := f.items[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *q
	return &cp, nil
}

func (f *fakeStore) PublicList(_ context.Context, flt ListFilter) ([]QA, int, error) {
	return f.listBy(func(q *QA) bool {
		if q.StatusID != StatusPublished {
			return false
		}
		if flt.TypeForumID > 0 && q.TypeForumID != flt.TypeForumID {
			return false
		}
		return true
	})
}

func (f *fakeStore) ListByStatus(_ context.Context, flt ModerationFilter) ([]QA, int, error) {
	return f.listBy(func(q *QA) bool {
		if flt.StatusID > 0 && q.StatusID != flt.StatusID {
			return false
		}
		if flt.ModeratorID > 0 && (q.ModeratorID == nil || *q.ModeratorID != flt.ModeratorID) {
			return false
		}
		return true
	})
}

func (f *fakeStore) listBy(pred func(*QA) bool) ([]QA, int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []QA
	for _, q := range f.items {
		if pred(q) {
			out = append(out, *q)
		}
	}
	return out, len(out), nil
}

func (f *fakeStore) Answer(_ context.Context, id int64, moderatorID int64, answer string, now time.Time) (*QA, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	q, ok := f.items[id]
	if !ok {
		return nil, ErrNotFound
	}
	if q.StatusID != StatusNew && q.StatusID != StatusNeedsRevision {
		return nil, ErrInvalidStatus
	}
	cp := *q
	cp.ModeratorID = &moderatorID
	cp.Answer = answer
	cp.AnsweredAt = &now
	cp.StatusID = StatusPendingReview
	cp.ReviewComment = ""
	cp.ReviewerID = nil
	cp.ReviewedAt = nil
	f.items[id] = &cp
	return &cp, nil
}

func (f *fakeStore) Publish(_ context.Context, id int64, reviewerID int64, now time.Time) (*QA, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	q, ok := f.items[id]
	if !ok {
		return nil, ErrNotFound
	}
	if q.StatusID != StatusPendingReview {
		return nil, ErrInvalidStatus
	}
	cp := *q
	cp.StatusID = StatusPublished
	cp.ReviewerID = &reviewerID
	cp.ReviewedAt = &now
	cp.ReviewComment = ""
	f.items[id] = &cp
	return &cp, nil
}

func (f *fakeStore) Reject(_ context.Context, id int64, reviewerID int64, comment string, now time.Time) (*QA, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	q, ok := f.items[id]
	if !ok {
		return nil, ErrNotFound
	}
	if q.StatusID != StatusPendingReview {
		return nil, ErrInvalidStatus
	}
	cp := *q
	cp.StatusID = StatusNeedsRevision
	cp.ReviewerID = &reviewerID
	cp.ReviewedAt = &now
	cp.ReviewComment = comment
	f.items[id] = &cp
	return &cp, nil
}

// ---- fake notifier ----

type fakeNotifier struct {
	mu        sync.Mutex
	published []int64
	rejected  []int64
}

func (n *fakeNotifier) QAPublished(_ context.Context, q *QA) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.published = append(n.published, q.ID)
	return nil
}

func (n *fakeNotifier) QARejected(_ context.Context, q *QA) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.rejected = append(n.rejected, q.ID)
	return nil
}

// ---- fake verifier ----

type fakeVerifier struct {
	anonymous bool
	claims    *authz.Claims
}

func (v *fakeVerifier) VerifyAccess(string) (*authz.Claims, error) {
	if v.anonymous {
		return nil, authz.ErrInvalidToken
	}
	return v.claims, nil
}

// ---- setup ----

type env struct {
	router   http.Handler
	store    *fakeStore
	notifier *fakeNotifier
}

func setup(t *testing.T, scopes []string) *env {
	t.Helper()
	store := newFakeStore()
	notifier := &fakeNotifier{}
	svc := NewService(store, notifier)

	var verifier *fakeVerifier
	if scopes == nil {
		verifier = &fakeVerifier{anonymous: true}
	} else {
		verifier = &fakeVerifier{claims: &authz.Claims{
			UserID: 100, Role: "test", Scopes: scopes,
		}}
	}
	return &env{router: NewHandler(svc, verifier).Router(), store: store, notifier: notifier}
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

func TestCreateQA_AnonymousOK(t *testing.T) {
	e := setup(t, nil) // без claims — гость

	rec := doJSON(t, e.router, http.MethodPost, "/",
		`{"key_id":1,"title":"Как пройти в аудиторию 101?","description":"Не могу найти"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("anonymous create: %d %s", rec.Code, rec.Body.String())
	}
	var q QA
	_ = json.NewDecoder(rec.Body).Decode(&q)
	if q.StatusID != StatusNew {
		t.Errorf("expected status new, got %d", q.StatusID)
	}
	if q.SenderID != nil {
		t.Errorf("expected sender_id=nil for anonymous, got %v", q.SenderID)
	}
}

func TestCreateQA_AuthorizedSenderFromClaims(t *testing.T) {
	e := setup(t, []string{"qa:read"})

	rec := doJSON(t, e.router, http.MethodPost, "/",
		`{"key_id":1,"title":"Q","sender_id":999}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	var q QA
	_ = json.NewDecoder(rec.Body).Decode(&q)
	if q.SenderID == nil || *q.SenderID != 100 {
		t.Errorf("expected sender_id=100 (from claims), got %v", q.SenderID)
	}
}

func TestPublicList_OnlyPublished(t *testing.T) {
	e := setup(t, []string{"qa:admin"})

	// создаём три вопроса
	for i := 0; i < 3; i++ {
		doJSON(t, e.router, http.MethodPost, "/",
			`{"key_id":1,"title":"Q`+string(rune('A'+i))+`"}`)
	}
	// публикуем только id=2
	// сначала нужен ответ модератора
	mod := setup(t, []string{"qa:write"})
	_ = mod
	// в fakeStore данные в e.store
	modEnv := &env{router: e.router, store: e.store}
	rec := doJSON(t, modEnv.router, http.MethodPost, "/2/answer", `{"answer":"Ответ"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("answer: %d %s", rec.Code, rec.Body.String())
	}
	rec = doJSON(t, modEnv.router, http.MethodPost, "/2/approve", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("approve: %d %s", rec.Code, rec.Body.String())
	}

	// публичный список — только опубликованный
	pub := setup(t, nil)
	_ = pub
	// используем тот же store
	pubEnv := &env{router: NewHandler(NewService(e.store, nil), &fakeVerifier{}).Router()}
	rec = doJSON(t, pubEnv.router, http.MethodGet, "/", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list: %d", rec.Code)
	}
	var page ListResponse
	_ = json.NewDecoder(rec.Body).Decode(&page)
	if page.Total != 1 {
		t.Errorf("public list: want 1 published, got %d", page.Total)
	}
	if len(page.Items) > 0 && page.Items[0].ID != 2 {
		t.Errorf("public list: want id=2, got %d", page.Items[0].ID)
	}

	// неопубликованный по ID — 404
	rec = doJSON(t, pubEnv.router, http.MethodGet, "/1", "")
	if rec.Code != http.StatusNotFound {
		t.Errorf("unpublished get: want 404, got %d", rec.Code)
	}
}

func TestWorkflow_ModeratorAnswerAdminApprove(t *testing.T) {
	e := setup(t, []string{"qa:admin", "qa:write"}) // все права для удобства

	// 1. Создаём
	rec := doJSON(t, e.router, http.MethodPost, "/", `{"key_id":1,"title":"Вопрос"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d", rec.Code)
	}

	// 2. Модератор отвечает
	rec = doJSON(t, e.router, http.MethodPost, "/1/answer", `{"answer":"Это ответ"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("answer: %d %s", rec.Code, rec.Body.String())
	}
	var q QA
	_ = json.NewDecoder(rec.Body).Decode(&q)
	if q.StatusID != StatusPendingReview {
		t.Errorf("after answer: want pending_review, got %d", q.StatusID)
	}

	// 3. Админ одобряет
	rec = doJSON(t, e.router, http.MethodPost, "/1/approve", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("approve: %d %s", rec.Code, rec.Body.String())
	}
	_ = json.NewDecoder(rec.Body).Decode(&q)
	if q.StatusID != StatusPublished {
		t.Errorf("after approve: want published, got %d", q.StatusID)
	}

	// 4. Уведомление отправлено
	if len(e.notifier.published) != 1 {
		t.Errorf("published notifications: want 1, got %d", len(e.notifier.published))
	}
}

func TestWorkflow_RejectAndRevision(t *testing.T) {
	e := setup(t, []string{"qa:admin", "qa:write"})

	rec := doJSON(t, e.router, http.MethodPost, "/", `{"key_id":1,"title":"Q"}`)
	_ = rec
	doJSON(t, e.router, http.MethodPost, "/1/answer", `{"answer":"Черновик"}`)

	// Админ отклоняет с комментарием
	rec = doJSON(t, e.router, http.MethodPost, "/1/reject",
		`{"comment":"Уточните формулировку"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("reject: %d %s", rec.Code, rec.Body.String())
	}
	var q QA
	_ = json.NewDecoder(rec.Body).Decode(&q)
	if q.StatusID != StatusNeedsRevision {
		t.Errorf("after reject: want needs_revision, got %d", q.StatusID)
	}
	if q.ReviewComment != "Уточните формулировку" {
		t.Errorf("comment: want 'Уточните формулировку', got %q", q.ReviewComment)
	}
	if len(e.notifier.rejected) != 1 {
		t.Errorf("rejected notifications: want 1, got %d", len(e.notifier.rejected))
	}

	// Модератор переделывает — снова pending_review
	rec = doJSON(t, e.router, http.MethodPost, "/1/answer", `{"answer":"Исправлено"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("re-answer: %d %s", rec.Code, rec.Body.String())
	}
	_ = json.NewDecoder(rec.Body).Decode(&q)
	if q.StatusID != StatusPendingReview {
		t.Errorf("after re-answer: want pending_review, got %d", q.StatusID)
	}
	if q.ReviewComment != "" {
		t.Errorf("re-answer should clear review_comment, got %q", q.ReviewComment)
	}
}

func TestWorkflow_InvalidTransitions(t *testing.T) {
	e := setup(t, []string{"qa:admin", "qa:write"})

	doJSON(t, e.router, http.MethodPost, "/", `{"key_id":1,"title":"Q"}`)

	// Попытка одобрить new → 409
	rec := doJSON(t, e.router, http.MethodPost, "/1/approve", "")
	if rec.Code != http.StatusConflict {
		t.Errorf("approve new: want 409, got %d", rec.Code)
	}

	// Попытка отклонить new → 409
	rec = doJSON(t, e.router, http.MethodPost, "/1/reject", `{"comment":"x"}`)
	if rec.Code != http.StatusConflict {
		t.Errorf("reject new: want 409, got %d", rec.Code)
	}

	// Ответ → pending_review → approve → published
	doJSON(t, e.router, http.MethodPost, "/1/answer", `{"answer":"A"}`)
	doJSON(t, e.router, http.MethodPost, "/1/approve", "")

	// published терминальный: и answer, и reject → 409
	rec = doJSON(t, e.router, http.MethodPost, "/1/answer", `{"answer":"B"}`)
	if rec.Code != http.StatusConflict {
		t.Errorf("answer published: want 409, got %d", rec.Code)
	}
	rec = doJSON(t, e.router, http.MethodPost, "/1/reject", `{"comment":"x"}`)
	if rec.Code != http.StatusConflict {
		t.Errorf("reject published: want 409, got %d", rec.Code)
	}
}

func TestRequireScope(t *testing.T) {
	// только модератор (qa:write), не админ
	e := setup(t, []string{"qa:write"})
	doJSON(t, e.router, http.MethodPost, "/", `{"key_id":1,"title":"Q"}`)
	doJSON(t, e.router, http.MethodPost, "/1/answer", `{"answer":"A"}`)

	// approve — нужен qa:admin → 403
	rec := doJSON(t, e.router, http.MethodPost, "/1/approve", "")
	if rec.Code != http.StatusForbidden {
		t.Errorf("moderator approve: want 403, got %d", rec.Code)
	}

	// reject — тоже 403
	rec = doJSON(t, e.router, http.MethodPost, "/1/reject", `{"comment":"x"}`)
	if rec.Code != http.StatusForbidden {
		t.Errorf("moderator reject: want 403, got %d", rec.Code)
	}
}