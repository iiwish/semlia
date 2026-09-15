-- Drop the M2-T005 policy rule vocabulary; policy_decisions (T002) and every
-- other table stay untouched.
DROP TABLE IF EXISTS policy_rules;
DROP FUNCTION IF EXISTS seed_m2_policy_rules;
