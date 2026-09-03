BEGIN;

-- M2-T006 review batches (docs/specs/m2-governed-authoring/data-model.md
-- "Reviews aggregate"; SSOT §8.4 batch confirmation channel). A review batch
-- is the persisted §8.4 audit record of one batch confirmation: it saves the
-- grouping rule, the member snapshots with their added reasons, the
-- representative samples, the exclusions with reasons, the reviewer and the
-- policy version that was current at assembly time. The human decisions
-- themselves remain the T002 immutable reviews rows — the batch rows only
-- record the confirmation outcome per member.

CREATE TABLE review_batches (
    id uuid PRIMARY KEY CHECK (substring(id::text, 15, 1) = '7'),
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE RESTRICT,
    grouping_rule jsonb NOT NULL CHECK (jsonb_typeof(grouping_rule) = 'object'),
    policy_version text NOT NULL CHECK (policy_version ~ '^[0-9]+\.[0-9]+$'),
    status text NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'confirmed', 'rejected')),
    created_by text NOT NULL CHECK (created_by <> '' AND length(created_by) <= 256),
    decided_by text CHECK (
        decided_by IS NULL OR (decided_by <> '' AND length(decided_by) <= 256)
    ),
    decided_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (workspace_id, id),
    -- An open batch is undecided; a decided batch always names its reviewer.
    CONSTRAINT review_batches_decision_shape CHECK (
        (status = 'open') = (decided_at IS NULL)
        AND (status = 'open') = (decided_by IS NULL)
    )
);

CREATE INDEX review_batches_workspace_status_idx
    ON review_batches (workspace_id, status, created_at DESC, id);

-- One member snapshot per proposal per batch. added_reason freezes the
-- policy decision snapshot that made the proposal eligible (matched rule,
-- risk level, reason code, rule version, inputs digest); decision and the
-- split columns fill at confirm time. The reviews themselves stay the
-- immutable T002 rows.
CREATE TABLE review_batch_members (
    review_batch_id uuid NOT NULL,
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE RESTRICT,
    proposal_id uuid NOT NULL,
    added_reason jsonb NOT NULL CHECK (jsonb_typeof(added_reason) = 'object'),
    decision text CHECK (decision IS NULL OR decision IN ('approved', 'rejected')),
    sample boolean NOT NULL DEFAULT false,
    split_out boolean NOT NULL DEFAULT false,
    split_reason text CHECK (
        split_reason IS NULL OR (split_reason <> '' AND length(split_reason) <= 512)
    ),
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (review_batch_id, proposal_id),
    FOREIGN KEY (workspace_id, review_batch_id)
        REFERENCES review_batches (workspace_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (workspace_id, proposal_id)
        REFERENCES proposals (workspace_id, id) ON DELETE RESTRICT,
    CONSTRAINT review_batch_members_split_shape CHECK (
        split_out = false OR (decision IS NULL AND split_reason IS NOT NULL)
    )
);

CREATE INDEX review_batch_members_proposal_idx ON review_batch_members (proposal_id);

-- Members are snapshots: the proposal reference and the frozen added reason
-- never change after assembly. The confirm-time outcome columns (decision,
-- sample, split_out, split_reason) are the only mutable fields.
CREATE FUNCTION guard_review_batch_member()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF TG_OP = 'UPDATE' THEN
        IF NEW.proposal_id <> OLD.proposal_id OR NEW.added_reason <> OLD.added_reason
            OR NEW.review_batch_id <> OLD.review_batch_id THEN
            RAISE EXCEPTION 'review batch member snapshots are immutable' USING ERRCODE = '55000';
        END IF;
        RETURN NEW;
    END IF;
    IF (SELECT status FROM review_batches WHERE id = NEW.review_batch_id) <> 'open' THEN
        RAISE EXCEPTION 'review batch members can only join an open batch' USING ERRCODE = '55000';
    END IF;
    IF (SELECT state FROM proposals
        WHERE workspace_id = NEW.workspace_id AND id = NEW.proposal_id) <> 'in_review' THEN
        RAISE EXCEPTION 'review batch members must be in_review proposals' USING ERRCODE = '55000';
    END IF;
    IF EXISTS (
        SELECT 1 FROM review_batch_members member
        JOIN review_batches batch ON batch.id = member.review_batch_id
        WHERE member.proposal_id = NEW.proposal_id
          AND member.split_out = false
          AND batch.status = 'open'
    ) THEN
        RAISE EXCEPTION 'proposal is already an active member of an open review batch'
            USING ERRCODE = '55000';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER review_batch_members_guard
    BEFORE INSERT OR UPDATE ON review_batch_members
    FOR EACH ROW EXECUTE FUNCTION guard_review_batch_member();

-- Batch rows follow the one-way discipline: assembly writes an open batch and
-- the confirm command performs the single open -> confirmed|rejected step that
-- records reviewer and decided_at together. Decided batches are frozen.
CREATE FUNCTION guard_review_batch_decision()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF OLD.status <> 'open' THEN
        RAISE EXCEPTION 'decided review batches are frozen' USING ERRCODE = '55000';
    END IF;
    IF NEW.status = 'open' OR NEW.decided_at IS NULL OR NEW.decided_by IS NULL
        OR NEW.grouping_rule <> OLD.grouping_rule OR NEW.policy_version <> OLD.policy_version
        OR NEW.created_by <> OLD.created_by THEN
        RAISE EXCEPTION 'a review batch confirm must record status, reviewer and decided_at together'
            USING ERRCODE = '55000';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER review_batches_guard_decision
    BEFORE UPDATE ON review_batches
    FOR EACH ROW EXECUTE FUNCTION guard_review_batch_decision();

COMMIT;
