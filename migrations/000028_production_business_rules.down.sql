DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM production_business_rule_events) THEN
        RAISE EXCEPTION 'DOWN_MIGRATION_UNSAFE: business rule confirmation history exists';
    END IF;
END $$;
DROP TABLE production_business_rule_events;
DROP FUNCTION guard_production_business_rule_event();
