-- 0004_auth.down.sql
BEGIN;
DROP TABLE IF EXISTS refresh_tokens;
DELETE FROM roles WHERE name IN ('super_admin','teacher');
COMMIT;