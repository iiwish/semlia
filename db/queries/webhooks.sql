-- name: ListWebhookSubscriptions :many
SELECT * FROM webhook_subscriptions WHERE workspace_id=$1 ORDER BY created_at DESC,id DESC LIMIT 200;

-- name: GetWebhookSubscription :one
SELECT * FROM webhook_subscriptions WHERE workspace_id=$1 AND id=$2;

-- name: LoadWebhookSecret :one
SELECT envelope FROM webhook_signing_secrets WHERE subscription_id=$1 AND version=$2;

-- name: ListWebhookDeliveries :many
SELECT * FROM webhook_deliveries WHERE workspace_id=$1 ORDER BY created_at DESC,id DESC LIMIT 200;

-- name: GetWebhookDelivery :one
SELECT * FROM webhook_deliveries WHERE workspace_id=$1 AND id=$2;

-- name: MatchingWebhookSubscriptions :many
SELECT * FROM webhook_subscriptions WHERE workspace_id=$1 AND enabled AND $2::text=ANY(event_types) ORDER BY id;

-- name: ClaimWebhookDelivery :one
WITH candidate AS (
 SELECT id FROM webhook_deliveries WHERE
 (state='queued' AND next_attempt_at<=$1) OR (state='running' AND leased_until<=$1)
 ORDER BY next_attempt_at,id FOR UPDATE SKIP LOCKED LIMIT 1
)
UPDATE webhook_deliveries d SET state='running',attempt=attempt+1,lease_owner=$2,leased_until=$1::timestamptz+interval '30 seconds',updated_at=$1
FROM candidate WHERE d.id=candidate.id RETURNING d.*;
