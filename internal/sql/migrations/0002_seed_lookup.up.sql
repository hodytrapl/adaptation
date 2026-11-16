-- =====================================================================
-- 0002_seed_lookup.up.sql — стартовые данные для справочников
-- =====================================================================
BEGIN;

INSERT INTO roles (name) VALUES
    ('admin'), ('super-admin'), ('client'), ('moderator'), ('teacher')
ON CONFLICT (name) DO NOTHING;

INSERT INTO statuses (name) VALUES
    ('new'), ('in_progress'), ('resolved'), ('rejected')
ON CONFLICT (name) DO NOTHING;

INSERT INTO type_forum (name) VALUES
    ('review'), ('question'), ('suggestion')
ON CONFLICT (name) DO NOTHING;

INSERT INTO tags (name) VALUES
    ('bug'), ('feature'), ('docs'), ('urgent'), ('question')
ON CONFLICT (name) DO NOTHING;

COMMIT;