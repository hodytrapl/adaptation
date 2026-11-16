BEGIN;

-- 1) Уведомления. Адресат — user_id. Payload — JSONB, чтобы любой модуль
--    мог положить свой контекст (qa_id, review_id, comment, ...).
CREATE TABLE IF NOT EXISTS notifications (
    id         BIGSERIAL    PRIMARY KEY,
    user_id    BIGINT       NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind       VARCHAR(50)  NOT NULL,
    payload    JSONB        NOT NULL DEFAULT '{}'::jsonb,
    read_at    TIMESTAMPTZ,
    created_at TIMESTAMPTZ  NOT NULL DEFAULT now()
);

-- Быстрый «мои непрочитанные» — самый частый запрос.
CREATE INDEX IF NOT EXISTS idx_notifications_user_unread
    ON notifications(user_id, created_at DESC) WHERE read_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_notifications_user_created
    ON notifications(user_id, created_at DESC);

-- 2) Чистим избыточный users:admin у роли admin:
--    по ТЗ управление пользователями — только super_admin.
UPDATE roles SET scopes = array_remove(scopes, 'users:admin') WHERE name = 'admin';

COMMIT;