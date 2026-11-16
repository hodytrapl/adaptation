BEGIN;

-- 1) Роли под ТЗ: super_admin, admin, moderator, client, teacher.
--    moderator уже есть из 0003 — ON CONFLICT обновит scopes до единого формата.
INSERT INTO roles (name, scopes) VALUES
    ('super_admin', ARRAY['*:*']),
    ('admin',       ARRAY[
        'feedback:admin','map:admin','teachers:admin','events:admin',
        'qa:admin','users:admin'
    ]),
    ('moderator',   ARRAY[
        'feedback:read','feedback:write',
        'map:read','map:write',
        'teachers:read','teachers:write',
        'events:read','events:write',
        'qa:read','qa:write'
    ]),
    ('client',      ARRAY['feedback:write','map:read','teachers:read','events:read','qa:write']),
    ('teacher',     ARRAY['feedback:write','map:read','teachers:read','events:read','qa:write'])
ON CONFLICT (name) DO UPDATE SET scopes = EXCLUDED.scopes;

-- 2) Refresh-токены. Храним SHA-256 hash, не сам токен.
CREATE TABLE IF NOT EXISTS refresh_tokens (
    id          BIGSERIAL    PRIMARY KEY,
    user_id     BIGINT       NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash  BYTEA        NOT NULL,
    expires_at  TIMESTAMPTZ  NOT NULL,
    revoked_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),
    user_agent  TEXT,
    ip          INET,
    CONSTRAINT refresh_tokens_hash_uniq UNIQUE (token_hash)
);

CREATE INDEX IF NOT EXISTS idx_refresh_tokens_user    ON refresh_tokens(user_id);
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_expires ON refresh_tokens(expires_at);

COMMIT;