-- M2-T007 release publishing substrate: governed-object manifest pins and the
-- originating-proposal link of the immutable release manifest. Release rows
-- stay immutable (P-003): the new column and table only extend what a cut can
-- pin, never how an existing row changes. Rollback remains a NEW release (D-008).

ALTER TABLE releases
    ADD COLUMN origin_proposal_id uuid;

ALTER TABLE releases
    ADD CONSTRAINT releases_origin_proposal_fkey
    FOREIGN KEY (workspace_id, origin_proposal_id)
    REFERENCES proposals (workspace_id, id) ON DELETE RESTRICT;

-- Governed-object pins of the release manifest: a release that publishes a
-- governed-object proposal pins the resulting object version the same way a
-- semantic-asset release pins the resulting asset revision.
CREATE TABLE release_objects (
    workspace_id uuid NOT NULL,
    release_id uuid NOT NULL,
    object_type text NOT NULL CHECK (object_type IN ('physical_binding', 'model_grain', 'entity_key', 'join_contract')),
    object_id uuid NOT NULL,
    version integer NOT NULL CHECK (version > 0),
    position integer NOT NULL CHECK (position > 0),
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (release_id, object_type, object_id),
    UNIQUE (release_id, position),
    FOREIGN KEY (workspace_id, release_id)
        REFERENCES releases (workspace_id, id) ON DELETE RESTRICT
);

CREATE INDEX release_objects_object_idx
    ON release_objects (workspace_id, object_type, object_id, version);

CREATE TRIGGER release_objects_immutable
    BEFORE UPDATE OR DELETE ON release_objects
    FOR EACH ROW EXECUTE FUNCTION reject_immutable_registry_row();
