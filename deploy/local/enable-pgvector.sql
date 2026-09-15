-- Explicit administrator operation after installing pgvector for the existing
-- PostgreSQL major version. Never replace or downgrade an existing data volume.
BEGIN;
CREATE EXTENSION IF NOT EXISTS vector;
ALTER TABLE embedding_items ADD COLUMN IF NOT EXISTS vector_value vector NOT NULL;
COMMIT;
