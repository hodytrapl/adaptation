BEGIN;

CREATE TABLE IF NOT EXISTS teachers (
    id          BIGSERIAL    PRIMARY KEY,
    full_name   VARCHAR(255) NOT NULL,
    department  VARCHAR(255) NOT NULL,
    position    VARCHAR(255) NOT NULL,
    contacts    JSONB        NOT NULL DEFAULT '{}'::jsonb,
    photo_url   TEXT,
    room_id     BIGINT       REFERENCES map_build(id) ON DELETE SET NULL,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_teachers_department       ON teachers(department);
CREATE INDEX IF NOT EXISTS idx_teachers_room_id          ON teachers(room_id);
CREATE INDEX IF NOT EXISTS idx_teachers_full_name_lower  ON teachers(lower(full_name));

COMMIT;