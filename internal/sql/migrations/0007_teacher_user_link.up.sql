BEGIN;

-- Один аккаунт ↔ одна карточка. NULL = карточка без аккаунта.
-- UNIQUE гарантирует инвариант на уровне БД.
ALTER TABLE teachers
    ADD COLUMN IF NOT EXISTS user_id BIGINT UNIQUE
    REFERENCES users(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_teachers_user_id ON teachers(user_id);

-- Инвариант: если teacher.user_id задан, у этого user'а должна быть role=teacher.
-- Триггер, потому что FK не умеет смотреть «вглубь» связанной таблицы.
CREATE OR REPLACE FUNCTION check_teacher_user_role() RETURNS TRIGGER AS $$
DECLARE
    r_name TEXT;
BEGIN
    IF NEW.user_id IS NULL THEN
        RETURN NEW;
    END IF;

    SELECT r.name INTO r_name
    FROM users u JOIN roles r ON r.id = u.role_id
    WHERE u.id = NEW.user_id;

    IF r_name IS NULL THEN
        RAISE EXCEPTION 'users.id=% not found', NEW.user_id;
    END IF;
    IF r_name <> 'teacher' THEN
        RAISE EXCEPTION 'user % has role %, expected teacher', NEW.user_id, r_name;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_teachers_check_user_role ON teachers;
CREATE TRIGGER trg_teachers_check_user_role
    BEFORE INSERT OR UPDATE OF user_id ON teachers
    FOR EACH ROW EXECUTE FUNCTION check_teacher_user_role();

COMMIT;