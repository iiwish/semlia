-- name: GetAskRequest :one
SELECT * FROM ask_requests WHERE workspace_id=$1 AND requested_by=$2 AND idempotency_key=$3;

-- name: ClaimAskRequest :one
INSERT INTO ask_requests(workspace_id,requested_by,idempotency_key,input_digest,knowledge_digest,agent_run_id,claim_token,call_deadline,status,created_at)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,'running',$9)
ON CONFLICT DO NOTHING RETURNING *;

-- name: LockAskRequest :one
SELECT * FROM ask_requests WHERE workspace_id=$1 AND requested_by=$2 AND idempotency_key=$3 FOR UPDATE;

-- name: CompleteAskRequest :one
UPDATE ask_requests SET status=$5,semantic_query_id=$6,error_code=$7,completed_at=$8
WHERE workspace_id=$1 AND requested_by=$2 AND idempotency_key=$3 AND claim_token=$4 AND status='running'
RETURNING *;

-- name: ExpiredAskRequests :many
SELECT * FROM ask_requests WHERE status='running' AND call_deadline<=clock_timestamp()
ORDER BY call_deadline LIMIT $1 FOR UPDATE SKIP LOCKED;
