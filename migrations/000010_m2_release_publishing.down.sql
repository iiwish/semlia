-- Downgrade 000010: drop the governed-object manifest pins and the
-- originating-proposal link. Release rows themselves are untouched.

DROP TRIGGER IF EXISTS release_objects_immutable ON release_objects;
DROP INDEX IF EXISTS release_objects_object_idx;
DROP TABLE IF EXISTS release_objects;

ALTER TABLE releases DROP CONSTRAINT IF EXISTS releases_origin_proposal_fkey;
ALTER TABLE releases DROP COLUMN IF EXISTS origin_proposal_id;
