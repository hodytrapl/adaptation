-- 0007_teacher_user_link.down.sql
BEGIN;
DROP TRIGGER IF EXISTS trg_teachers_check_user_role ON teachers;
DROP FUNCTION IF EXISTS check_teacher_user_role();
DROP INDEX IF EXISTS idx_teachers_user_id;
ALTER TABLE teachers DROP COLUMN IF EXISTS user_id;
COMMIT;