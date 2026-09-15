-- Drop the M2-T006 review batch audit record; reviews (T002), proposals and
-- every other table stay untouched.
DROP TRIGGER IF EXISTS review_batches_guard_decision ON review_batches;
DROP TRIGGER IF EXISTS review_batch_members_guard ON review_batch_members;
DROP TABLE IF EXISTS review_batch_members;
DROP TABLE IF EXISTS review_batches;
DROP FUNCTION IF EXISTS guard_review_batch_member;
DROP FUNCTION IF EXISTS guard_review_batch_decision;
