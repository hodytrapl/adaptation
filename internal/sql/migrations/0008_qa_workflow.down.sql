-- 0008_qa_workflow.down.sql
BEGIN;
DROP INDEX IF EXISTS idx_qa_a_status_created;

ALTER TABLE qa_a
    DROP COLUMN IF EXISTS moderator_id,
    DROP COLUMN IF EXISTS answer,
    DROP COLUMN IF EXISTS answered_at,
    DROP COLUMN IF EXISTS reviewer_id,
    DROP COLUMN IF EXISTS review_comment,
    DROP COLUMN IF EXISTS reviewed_at;

ALTER TABLE qa_a ALTER COLUMN sender_id SET NOT NULL;
ALTER TABLE qa_a ALTER COLUMN type_forum_id DROP DEFAULT;

DELETE FROM statuses WHERE name IN ('pending_review','published','needs_revision');
COMMIT;