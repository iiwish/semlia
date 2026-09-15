BEGIN;

-- Versioned policy rule table (docs/specs/m2-governed-authoring/data-model.md
-- "Policy and risk"; decision D3: rules are data rows evaluated by the Go
-- evaluator, no cel-go/DSL). One rule row pins its match condition over the
-- closed DecisionInputs fields and its explainable outcome. Rows are
-- immutable: a new rule semantics ships as a new rule_version, never as an
-- in-place edit, so every persisted decision stays recomputable (SSOT §8.2).
-- The routing vocabulary is expert|batch only: the deferred auto channel
-- (SSOT §8.4, M4/M5) is unrepresentable at the schema level.
CREATE TABLE policy_rules (
    rule_id text NOT NULL CHECK (rule_id <> '' AND length(rule_id) <= 128),
    rule_version text NOT NULL CHECK (rule_version ~ '^[0-9]+\.[0-9]+$'),
    priority integer NOT NULL,
    match jsonb NOT NULL CHECK (jsonb_typeof(match) = 'object'),
    outcome_risk_level text NOT NULL CHECK (outcome_risk_level IN ('low', 'medium', 'high')),
    outcome_routing text NOT NULL CHECK (outcome_routing IN ('expert', 'batch')),
    outcome_reason_code text NOT NULL CHECK (outcome_reason_code ~ '^[A-Z][A-Z0-9_]{2,63}$'),
    outcome_explanation text NOT NULL CHECK (outcome_explanation <> '' AND length(outcome_explanation) <= 512),
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (rule_version, rule_id)
);

CREATE INDEX policy_rules_version_priority_idx
    ON policy_rules (rule_version, priority DESC, rule_id);

CREATE TRIGGER policy_rules_immutable
    BEFORE UPDATE OR DELETE ON policy_rules
    FOR EACH ROW EXECUTE FUNCTION reject_immutable_registry_row();

-- Idempotent seed of the 1.0 rule table. The rows mirror
-- governance.CanonicalPolicyRules("1.0") in internal/domain/governance and
-- reproduce the exact outcomes the T002 recomputability suite locks.
-- Re-invoking this function must not duplicate rows.
CREATE FUNCTION seed_m2_policy_rules()
RETURNS void
LANGUAGE sql
AS $$
INSERT INTO policy_rules (
    rule_id, rule_version, priority, match, outcome_risk_level, outcome_routing,
    outcome_reason_code, outcome_explanation
) VALUES
    ('semlia.risk.v1/blockers', '1.0', 100,
     '{"field":"blockerCount","op":"gt","value":0}',
     'high', 'expert', 'RISK_BLOCKER',
     'Validation blockers require expert review before the change can proceed (SSOT §8.4).'),
    ('semlia.risk.v1/production-computation', '1.0', 90,
     '{"all":[{"field":"affectsComputation","op":"eq","value":true},{"field":"productionEnvironment","op":"eq","value":true}]}',
     'high', 'expert', 'RISK_PRODUCTION_COMPUTATION',
     'Computation changes in production always take the expert channel (SSOT §8.4).'),
    ('semlia.risk.v1/access-or-contract', '1.0', 80,
     '{"any":[{"field":"affectsAccess","op":"eq","value":true},{"field":"affectsContract","op":"eq","value":true}]}',
     'high', 'expert', 'RISK_ACCESS_OR_CONTRACT_CHANGE',
     'Access-scope and consumption-contract changes always take the expert channel (SSOT §8.4).'),
    ('semlia.risk.v1/computation', '1.0', 70,
     '{"field":"affectsComputation","op":"eq","value":true}',
     'medium', 'expert', 'RISK_COMPUTATION_CHANGE',
     'Computation or expression changes are reviewed item by item by the asset owner.'),
    ('semlia.risk.v1/production-change', '1.0', 60,
     '{"field":"productionEnvironment","op":"eq","value":true}',
     'medium', 'expert', 'RISK_PRODUCTION_CHANGE',
     'Production-affecting changes are reviewed item by item.'),
    ('semlia.risk.v1/low-risk', '1.0', 10,
     '{}',
     'low', 'batch', 'RISK_LOW_BATCH',
     'No risk category matched; the change joins the batch confirmation channel (SSOT §8.4).')
ON CONFLICT (rule_version, rule_id) DO NOTHING;
$$;

SELECT seed_m2_policy_rules();

COMMIT;
