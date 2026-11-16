BEGIN;

-- 1) Новые статусы для Q&A workflow (ID: 6,7,8 — после существующих 1..5).
INSERT INTO statuses (name) VALUES
    ('pending_review'),
    ('published'),
    ('needs_revision')
ON CONFLICT (name) DO NOTHING;

-- 2) Расширяем qa_a: поля ответа и ревью.
ALTER TABLE qa_a
    ADD COLUMN IF NOT EXISTS moderator_id   BIGINT REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS answer         TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS answered_at    TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS reviewer_id    BIGINT REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS review_comment TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS reviewed_at    TIMESTAMPTZ;

-- 3) Публичный вопрос может быть без автора (гость).
ALTER TABLE qa_a ALTER COLUMN sender_id DROP NOT NULL;
-- 4) type_forum_id по умолчанию = question (id=2 в сиде).
ALTER TABLE qa_a ALTER COLUMN type_forum_id SET DEFAULT 2;

-- 5) Ускоряем выборки по статусу (pending_review для админа, new для модератора).
CREATE INDEX IF NOT EXISTS idx_qa_a_status_created
    ON qa_a(status_id, created_at DESC);

COMMIT;