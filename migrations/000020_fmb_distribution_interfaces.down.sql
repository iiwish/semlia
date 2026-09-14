BEGIN;
DROP TABLE webhook_fanout_receipts;
DROP TABLE webhook_deliveries;
DROP TABLE webhook_signing_secrets;
DROP TABLE webhook_subscriptions;
-- Historic channel attribution survives rollback; a restrictive old constraint
-- would require rewriting immutable events. The widened vocabulary is safe.
DROP TABLE client_credentials;
DROP TABLE consumer_machine_principals;
COMMIT;
