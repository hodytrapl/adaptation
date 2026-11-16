-- =====================================================================
-- 0001_init.up.sql
-- Схема сервиса Task Manager: справочники, пользователи и доменные сущности
-- =====================================================================

BEGIN;

-- ---------------------------------------------------------------------
-- Расширения (нужны для gen_random_uuid(), если PK — uuid)
-- ---------------------------------------------------------------------
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- ---------------------------------------------------------------------
-- Справочники (lookup tables)
-- ---------------------------------------------------------------------

-- roles — роли пользователей
CREATE TABLE IF NOT EXISTS roles (
    id          BIGSERIAL    PRIMARY KEY,
    name        VARCHAR(100) NOT NULL UNIQUE,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT now()
);

-- statuses — статусы (используется в QaA и reviews)
CREATE TABLE IF NOT EXISTS statuses (
    id          BIGSERIAL    PRIMARY KEY,
    name        VARCHAR(100) NOT NULL UNIQUE,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT now()
);

-- tags — справочник тегов
CREATE TABLE IF NOT EXISTS tags (
    id          BIGSERIAL    PRIMARY KEY,
    name        VARCHAR(100) NOT NULL UNIQUE,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT now()
);

-- type_forum — типы форумных обращений (reviews / QaA)
CREATE TABLE IF NOT EXISTS type_forum (
    id          BIGSERIAL    PRIMARY KEY,
    name        VARCHAR(100) NOT NULL UNIQUE,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT now()
);

-- ---------------------------------------------------------------------
-- users — пользователи
-- ---------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS users (
    id          BIGSERIAL    PRIMARY KEY,
    login       VARCHAR(100) NOT NULL UNIQUE,
    password    VARCHAR(255) NOT NULL,                 -- хранить ХЭШ, не plaintext!
    passport    VARCHAR(50),
    img_photo   TEXT,
    name        VARCHAR(255) NOT NULL,
    data        JSONB        NOT NULL DEFAULT '{}'::jsonb,
    role_id     BIGINT       REFERENCES roles(id) ON DELETE SET NULL,
    scopes      TEXT[]       NOT NULL DEFAULT ARRAY[]::TEXT[],
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_users_role_id   ON users(role_id);
CREATE INDEX IF NOT EXISTS idx_users_scopes    ON users USING GIN(scopes);
CREATE INDEX IF NOT EXISTS idx_users_data_gin  ON users USING GIN(data);

-- ---------------------------------------------------------------------
-- reviews — отзывы / обращения
-- ---------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS reviews (
    id              BIGSERIAL    PRIMARY KEY,
    key_id          BIGINT       NOT NULL,             -- внешний идентификатор ключа/объекта
    sender_id       BIGINT       NOT NULL REFERENCES users(id)      ON DELETE RESTRICT,
    type_forum_id   BIGINT       NOT NULL REFERENCES type_forum(id) ON DELETE RESTRICT,
    title           VARCHAR(500) NOT NULL,
    description     TEXT         NOT NULL DEFAULT '',
    tags            TEXT[]       NOT NULL DEFAULT ARRAY[]::TEXT[],
    status_id       BIGINT       NOT NULL REFERENCES statuses(id)   ON DELETE RESTRICT,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_reviews_key_id        ON reviews(key_id);
CREATE INDEX IF NOT EXISTS idx_reviews_sender_id     ON reviews(sender_id);
CREATE INDEX IF NOT EXISTS idx_reviews_type_forum_id ON reviews(type_forum_id);
CREATE INDEX IF NOT EXISTS idx_reviews_status_id     ON reviews(status_id);
CREATE INDEX IF NOT EXISTS idx_reviews_tags_gin      ON reviews USING GIN(tags);
CREATE INDEX IF NOT EXISTS idx_reviews_created_at    ON reviews(created_at DESC);

-- ---------------------------------------------------------------------
-- events — события
-- ---------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS events (
    id            BIGSERIAL    PRIMARY KEY,
    title         VARCHAR(500) NOT NULL,
    description   TEXT         NOT NULL DEFAULT '',
    date_started  TIMESTAMPTZ  NOT NULL,
    duration      INTERVAL     NOT NULL DEFAULT INTERVAL '0',
    place         VARCHAR(500) NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_events_date_started ON events(date_started);
CREATE INDEX IF NOT EXISTS idx_events_created_at   ON events(created_at DESC);

-- ---------------------------------------------------------------------
-- map_build — точки на карте (здания/этажи)
-- ---------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS map_build (
    id           BIGSERIAL    PRIMARY KEY,
    number       VARCHAR(50)  NOT NULL,
    floor        INTEGER      NOT NULL DEFAULT 0,
    building     VARCHAR(100) NOT NULL,
    coordinates  POINT        NOT NULL,                -- PostGIS: GEOMETRY(Point, 4326), если нужен geo
    description  TEXT         NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_map_build_number   ON map_build(number);
CREATE INDEX IF NOT EXISTS idx_map_build_building ON map_build(building);
-- Поиск ближайших точек (требует PostGIS):
-- CREATE INDEX idx_map_build_coords ON map_build USING GIST(coordinates);

-- ---------------------------------------------------------------------
-- qa_a — вопросы/ответы (QaA)
-- ---------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS qa_a (
    id              BIGSERIAL    PRIMARY KEY,
    key_id          BIGINT       NOT NULL,
    sender_id       BIGINT       NOT NULL REFERENCES users(id)      ON DELETE RESTRICT,
    status_id       BIGINT       NOT NULL REFERENCES statuses(id)   ON DELETE RESTRICT,
    type_forum_id   BIGINT       NOT NULL REFERENCES type_forum(id) ON DELETE RESTRICT,
    title           VARCHAR(500) NOT NULL,
    description     TEXT         NOT NULL DEFAULT '',
    tags            TEXT[]       NOT NULL DEFAULT ARRAY[]::TEXT[],
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_qa_a_key_id        ON qa_a(key_id);
CREATE INDEX IF NOT EXISTS idx_qa_a_sender_id     ON qa_a(sender_id);
CREATE INDEX IF NOT EXISTS idx_qa_a_status_id     ON qa_a(status_id);
CREATE INDEX IF NOT EXISTS idx_qa_a_type_forum_id ON qa_a(type_forum_id);
CREATE INDEX IF NOT EXISTS idx_qa_a_tags_gin      ON qa_a USING GIN(tags);
CREATE INDEX IF NOT EXISTS idx_qa_a_created_at    ON qa_a(created_at DESC);

-- ---------------------------------------------------------------------
-- responders — связка "исполнитель -> клиент"
-- ---------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS responders (
    id            BIGSERIAL   PRIMARY KEY,
    responder_id  BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    client_id     BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT responders_responder_client_uniq UNIQUE (responder_id, client_id),
    CONSTRAINT responders_no_self CHECK (responder_id <> client_id)
);

CREATE INDEX IF NOT EXISTS idx_responders_responder_id ON responders(responder_id);
CREATE INDEX IF NOT EXISTS idx_responders_client_id    ON responders(client_id);

COMMIT;