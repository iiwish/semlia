BEGIN;
DO $$
BEGIN
    IF EXISTS(SELECT 1 FROM embedding_index_versions) THEN
        RAISE EXCEPTION 'cannot roll back embedding migration while index history exists' USING ERRCODE='55000';
    END IF;
END;
$$;
DROP TABLE embedding_items;
DROP TABLE knowledge_chunks;
DROP TABLE embedding_index_versions;
COMMIT;
