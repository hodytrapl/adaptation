-- 0009_notifications.down.sql
BEGIN;
UPDATE roles SET scopes = array_append(scopes, 'users:admin') WHERE name = 'admin';
DROP TABLE IF EXISTS notifications;
COMMIT;