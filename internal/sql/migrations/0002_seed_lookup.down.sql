-- 0002_seed_lookup.down.sql
BEGIN;
DELETE FROM tags       WHERE name IN ('bug','feature','docs','urgent','question');
DELETE FROM type_forum WHERE name IN ('review','question','suggestion');
DELETE FROM statuses   WHERE name IN ('new','in_progress','resolved','rejected');
DELETE FROM roles      WHERE name IN ('admin','super-admin','client','moderator','teacher');
COMMIT;