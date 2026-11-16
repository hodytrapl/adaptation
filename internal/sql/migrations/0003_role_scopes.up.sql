-- 0003_role_scopes.up.sql
BEGIN;

-- 1) Скоупы на роли
ALTER TABLE roles
    ADD COLUMN IF NOT EXISTS scopes TEXT[] NOT NULL DEFAULT ARRAY[]::TEXT[];

CREATE INDEX IF NOT EXISTS idx_roles_scopes ON roles USING GIN(scopes);

-- 2) Отдельная (опциональная) таблица scopes как справочник —
--    чтобы фронт/админка знали полный список существующих скоупов.
CREATE TABLE IF NOT EXISTS scopes (
    name        VARCHAR(100) PRIMARY KEY,        -- 'feedback:read'
    module      VARCHAR(50)  NOT NULL,           -- 'feedback'
    action      VARCHAR(20)  NOT NULL,           -- 'read' | 'write' | 'admin'
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),
    CONSTRAINT scopes_action_chk CHECK (action IN ('read','write','admin'))
);

CREATE INDEX IF NOT EXISTS idx_scopes_module ON scopes(module);

INSERT INTO scopes (name, module, action) VALUES
    ('feedback:read','feedback','read'),
    ('feedback:write','feedback','write'),
    ('feedback:admin','feedback','admin'),
    ('map:read','map','read'),
    ('map:write','map','write'),
    ('teachers:read','teachers','read'),
    ('teachers:write','teachers','write'),
    ('events:read','events','read'),
    ('events:write','events','write'),
    ('qa:read','qa','read'),
    ('qa:write','qa','write')
ON CONFLICT (name) DO NOTHING;

-- 3) Сид роли moderator
INSERT INTO roles (name, scopes) VALUES
    ('moderator', ARRAY[
        'feedback:read','feedback:write',
        'map:read','map:write',
        'teachers:read','teachers:write',
        'events:read','events:write',
        'qa:read','qa:write'
    ])
ON CONFLICT (name) DO UPDATE SET scopes = EXCLUDED.scopes;

COMMIT;