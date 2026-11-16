-- 0003_role_scopes.down.sql
BEGIN;
DELETE FROM roles WHERE name = 'moderator';
DROP TABLE IF EXISTS scopes;
DROP INDEX IF EXISTS idx_roles_scopes;
ALTER TABLE roles DROP COLUMN IF EXISTS scopes;
COMMIT;