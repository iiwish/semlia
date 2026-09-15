BEGIN;
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM local_password_credentials) THEN
        RAISE EXCEPTION 'local password credentials prevent destructive downgrade';
    END IF;
END $$;
DROP TABLE password_login_budgets;
DROP TABLE local_password_credentials;
COMMIT;
