-- Drop only the M2-T002 governed-authoring tables and functions; M0/M1/M2-T001
-- rows and schema stay untouched.
ALTER TABLE proposals DROP CONSTRAINT IF EXISTS proposals_policy_decision_fkey;
DROP TABLE IF EXISTS release_assets;
DROP TABLE IF EXISTS releases;
DROP TABLE IF EXISTS policy_decisions;
DROP TABLE IF EXISTS validation_results;
DROP TABLE IF EXISTS validation_runs;
DROP TABLE IF EXISTS reviews;
DROP TABLE IF EXISTS proposal_changes;
DROP TABLE IF EXISTS proposals;
DROP TABLE IF EXISTS agent_steps;
DROP TABLE IF EXISTS agent_runs;
DROP FUNCTION IF EXISTS validate_proposal_state_transition;
DROP FUNCTION IF EXISTS guard_proposal_change_set;
