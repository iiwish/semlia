-- name: FailExpiredEmbedding :exec
UPDATE embedding_index_versions
SET state='failed',error_code='EMBEDDING_LEASE_EXPIRED',updated_at=clock_timestamp()
WHERE workspace_id=$1 AND job_id=$2 AND state='building';
