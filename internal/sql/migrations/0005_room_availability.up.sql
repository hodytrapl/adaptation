BEGIN;

CREATE TABLE IF NOT EXISTS room_availability (
    room_id    BIGINT      NOT NULL REFERENCES map_build(id) ON DELETE CASCADE,
    date       DATE        NOT NULL,
    status     VARCHAR(20) NOT NULL,
    updated_by BIGINT      REFERENCES users(id) ON DELETE SET NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (room_id, date),
    CONSTRAINT room_availability_status_chk CHECK (status IN ('free','busy'))
);

CREATE INDEX IF NOT EXISTS idx_room_availability_date ON room_availability(date);

COMMIT;