BEGIN;
DO $$ BEGIN
    IF EXISTS(SELECT 1 FROM production_generation_requests) OR EXISTS(SELECT 1 FROM production_generation_links) OR
       EXISTS(SELECT 1 FROM production_generation_outputs) OR EXISTS(SELECT 1 FROM production_generation_applications) THEN
        RAISE EXCEPTION 'DOWN_MIGRATION_UNSAFE: production generation history exists';
    END IF;
END; $$;
DROP TABLE production_generation_requests;
DROP FUNCTION validate_production_generation_result();
DROP FUNCTION protect_production_generation_request();
ALTER TABLE production_generation_links ALTER COLUMN model_config_revision TYPE integer USING model_config_revision::integer;
COMMIT;
