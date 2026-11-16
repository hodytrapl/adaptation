-- =====================================================================
-- 0001_init.down.sql  (откат в обратном порядке)
-- =====================================================================
BEGIN;

DROP TABLE IF EXISTS responders;
DROP TABLE IF EXISTS qa_a;
DROP TABLE IF EXISTS map_build;
DROP TABLE IF EXISTS events;
DROP TABLE IF EXISTS reviews;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS type_forum;
DROP TABLE IF EXISTS tags;
DROP TABLE IF EXISTS statuses;
DROP TABLE IF EXISTS roles;

COMMIT;