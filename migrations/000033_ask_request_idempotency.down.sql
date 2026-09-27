BEGIN;
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM ask_requests) THEN
        RAISE EXCEPTION 'Ask history requires a verified backup restore, not destructive downgrade';
    END IF;
END $$;
DROP TABLE ask_requests;
DROP FUNCTION validate_ask_attribution();
DROP FUNCTION protect_ask_request();
COMMIT;
