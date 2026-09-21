package demodata

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"github.com/iiwish/semlia/internal/domain/distribution"
	"github.com/iiwish/semlia/internal/domain/semantic"
	"github.com/iiwish/semlia/pkg/identity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const demoSourceLocator = "127.0.0.1:5433/semlia_demo_202609"
const demoSourceSchema = "semlia_demo_202609"
const demoSourceDisclosure = "合成演示：订单与客户。由本地固定演示模式声明导入；非外部数据源抓取，不包含真实业务数据。"

type sourceStatement struct {
	sql  string
	args []any
}

// SeedSource imports only the declared synthetic schema. The caller must create
// and verify Scenario.DataSQL in the separate demo database before calling it.
// It deliberately creates neither credentials nor a pretend discovery run.
func SeedSource(ctx context.Context, pool *pgxpool.Pool, workspace identity.WorkspaceID, actor identity.PrincipalID, snapshot *distribution.ReleaseSnapshot) error {
	statements, err := sourceStatements(workspace, actor, snapshot)
	if err != nil {
		return err
	}
	if pool == nil {
		return fmt.Errorf("synthetic source requires an application database")
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := lockDemoWorkspace(ctx, tx, workspace); err != nil {
		return err
	}
	exists, err := existingDemoSource(ctx, tx, workspace, snapshot)
	if err != nil {
		return err
	}
	if exists {
		return tx.Commit(ctx)
	}
	for _, statement := range statements {
		if _, err := tx.Exec(ctx, statement.sql, statement.args...); err != nil {
			return fmt.Errorf("import synthetic source: %w", err)
		}
	}
	return tx.Commit(ctx)
}

func lockDemoWorkspace(ctx context.Context, tx pgx.Tx, workspace identity.WorkspaceID) error {
	var slug string
	if err := tx.QueryRow(ctx, `SELECT slug FROM workspaces WHERE id=$1 FOR UPDATE`, workspace.UUID()).Scan(&slug); err != nil {
		return fmt.Errorf("lock synthetic workspace: %w", err)
	}
	if slug != "demo-202609-commerce" {
		return fmt.Errorf("source import requires the isolated synthetic workspace")
	}
	return nil
}

// An exact committed fixture is a receipt for retries after a local state-file
// write failed. A name/locator match alone never permits adopting a source.
func existingDemoSource(ctx context.Context, tx pgx.Tx, workspace identity.WorkspaceID, snapshot *distribution.ReleaseSnapshot) (bool, error) {
	first := snapshot.Execution.Relations[0]
	source, err := identity.FromUUID(identity.SourceConnection, first.SourceID)
	if err != nil {
		source, err = identity.Parse(identity.SourceConnection, first.SourceID)
	}
	if err != nil {
		return false, err
	}
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM source_connections WHERE id=$1)`, source.UUID()).Scan(&exists); err != nil || !exists {
		return false, err
	}
	var snapshotID identity.SourceSnapshotID
	for _, asset := range snapshot.Assets {
		if asset.AssetType == semantic.DataAsset {
			var content struct {
				Spec semantic.KnowledgeSpec `json:"spec"`
			}
			if err := json.Unmarshal(asset.Content, &content); err != nil {
				return false, err
			}
			snapshotID, err = identity.ParseSourceSnapshotID(content.Spec.DatasetRef.SnapshotID)
			if err != nil {
				return false, err
			}
			break
		}
	}
	content, _ := json.Marshal(snapshot.Execution.Relations)
	var matches bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(
 SELECT 1 FROM source_connections c
 JOIN source_snapshots s ON s.workspace_id=c.workspace_id AND s.source_connection_id=c.id
 JOIN source_revisions r ON r.workspace_id=s.workspace_id AND r.id=s.source_revision_id
 WHERE c.id=$1 AND c.workspace_id=$2 AND c.normalized_locator=$3
 AND c.adapter_kind='postgresql_catalog' AND c.source_kind='postgresql' AND c.status='active'
 AND c.metadata @> '{"synthetic":true,"provenance":"declared_local_fixture"}'::jsonb
 AND s.id=$4 AND s.adapter_version='synthetic-demo/1' AND s.content_digest=$5
 AND s.scope_digest=$6 AND s.history_quality='verified' AND s.coverage_status='complete'
 AND r.content_digest=$5 AND r.adapter_version='synthetic-demo/1'
 AND (SELECT count(*) FROM source_snapshots WHERE source_connection_id=c.id)=1
 AND (SELECT count(*) FROM source_revisions WHERE source_connection_id=c.id)=1
 AND (SELECT count(*) FROM physical_datasets WHERE source_connection_id=c.id)=2
 AND (SELECT count(*) FROM physical_fields f JOIN physical_datasets d ON d.id=f.physical_dataset_id WHERE d.source_connection_id=c.id)=8
 AND (SELECT count(*) FROM source_snapshot_members WHERE workspace_id=$2 AND snapshot_id=s.id)=10
 AND (SELECT count(*) FROM source_snapshot_members WHERE workspace_id=$2 AND snapshot_id=s.id AND kind='dataset')=2
 AND (SELECT count(*) FROM source_snapshot_members m JOIN physical_datasets d ON d.workspace_id=m.workspace_id AND d.id=m.object_id AND d.current_revision_id=m.revision_id AND d.qualified_name=m.historical_locator WHERE m.workspace_id=$2 AND m.snapshot_id=s.id AND m.kind='dataset')=2
 AND (SELECT count(*) FROM source_snapshot_members m JOIN physical_fields f ON f.workspace_id=m.workspace_id AND f.id=m.object_id AND f.current_revision_id=m.revision_id AND f.name=m.historical_name WHERE m.workspace_id=$2 AND m.snapshot_id=s.id AND m.kind='field')=8
 AND (SELECT count(*) FROM source_snapshot_scope WHERE workspace_id=$2 AND snapshot_id=s.id)=1
 AND EXISTS(SELECT 1 FROM source_snapshot_scope WHERE workspace_id=$2 AND snapshot_id=s.id AND coverage_key=$7 AND selector=$7 AND status='complete' AND enumeration_complete)
 AND EXISTS(SELECT 1 FROM source_effective_snapshots WHERE workspace_id=$2 AND source_connection_id=c.id AND snapshot_id=s.id AND scope_digest=$6)
)`, source.UUID(), workspace.UUID(), first.SourceLocator, snapshotID.UUID(), sourceDigest(content), sourceDigest([]byte(demoSourceSchema)), demoSourceSchema).Scan(&matches)
	if err != nil {
		return false, err
	}
	if !matches {
		return false, fmt.Errorf("existing synthetic source does not match its fixture receipt")
	}
	return true, nil
}

func sourceStatements(workspace identity.WorkspaceID, actor identity.PrincipalID, snapshot *distribution.ReleaseSnapshot) ([]sourceStatement, error) {
	if workspace.IsZero() || actor.IsZero() || snapshot == nil || snapshot.WorkspaceID != workspace || snapshot.Execution == nil || len(snapshot.Execution.Relations) != 2 {
		return nil, fmt.Errorf("synthetic source requires a matching workspace and exactly two execution relations")
	}
	ids := map[string]string{}
	parse := func(prefix identity.Prefix, value string) error {
		id, err := identity.Parse(prefix, value)
		if err != nil {
			id, err = identity.FromUUID(prefix, value)
		}
		if err != nil {
			return fmt.Errorf("synthetic source %s: %w", prefix, err)
		}
		ids[value] = id.UUID()
		return nil
	}
	first := snapshot.Execution.Relations[0]
	if err := parse(identity.SourceConnection, first.SourceID); err != nil {
		return nil, err
	}
	if err := parse(identity.SourceRevision, first.SourceRevisionID); err != nil {
		return nil, err
	}
	specs := map[string]semantic.KnowledgeSpec{}
	snapshotID := ""
	for _, asset := range snapshot.Assets {
		if asset.AssetType != semantic.DataAsset {
			continue
		}
		var content struct {
			Spec semantic.KnowledgeSpec `json:"spec"`
		}
		if err := json.Unmarshal(asset.Content, &content); err != nil {
			return nil, fmt.Errorf("synthetic data asset: %w", err)
		}
		ref := content.Spec.DatasetRef
		if ref == nil || ref.Kind != "dataset" || (snapshotID != "" && snapshotID != ref.SnapshotID) {
			return nil, fmt.Errorf("synthetic datasets must share one source snapshot")
		}
		if err := parse(identity.PhysicalDataset, ref.ObjectID); err != nil {
			return nil, err
		}
		if err := parse(identity.PhysicalDatasetRevision, ref.RevisionID); err != nil {
			return nil, err
		}
		if _, exists := specs[ids[ref.ObjectID]]; exists {
			return nil, fmt.Errorf("duplicate synthetic dataset reference")
		}
		snapshotID = ref.SnapshotID
		specs[ids[ref.ObjectID]] = content.Spec
	}
	if len(specs) != 2 {
		return nil, fmt.Errorf("synthetic source requires exactly two data assets")
	}
	if err := parse(identity.SourceSnapshot, snapshotID); err != nil {
		return nil, err
	}
	seenTables := map[string]bool{}
	seenObjects := map[string]bool{}
	for _, relation := range snapshot.Execution.Relations {
		if relation.SourceID != first.SourceID || relation.SourceRevisionID != first.SourceRevisionID || relation.SourceLocator != first.SourceLocator || (relation.SourceLocator != demoSourceLocator && relation.SourceLocator != MacoSourceLocator) || relation.AdapterKind != "postgresql_catalog" || relation.Schema != demoSourceSchema || (relation.Relation != "orders" && relation.Relation != "customers") || seenTables[relation.Relation] {
			return nil, fmt.Errorf("synthetic source is restricted to the local demo orders and customers")
		}
		seenTables[relation.Relation] = true
		for _, value := range []struct {
			prefix identity.Prefix
			value  string
		}{{identity.PhysicalDataset, relation.DatasetID}, {identity.PhysicalDatasetRevision, relation.DatasetRevisionID}} {
			if err := parse(value.prefix, value.value); err != nil {
				return nil, err
			}
			if seenObjects[ids[value.value]] {
				return nil, fmt.Errorf("duplicate synthetic object identity")
			}
			seenObjects[ids[value.value]] = true
		}
		spec, exists := specs[ids[relation.DatasetID]]
		if !exists || ids[spec.DatasetRef.RevisionID] != ids[relation.DatasetRevisionID] || len(spec.Members) != len(relation.Fields) || len(relation.Fields) == 0 {
			return nil, fmt.Errorf("synthetic dataset source references do not match execution")
		}
		fields := map[string]*semantic.SourceReference{}
		for _, member := range spec.Members {
			ref := member.SourceFieldRef
			if ref == nil || ref.Kind != "field" || ref.SnapshotID != snapshotID || member.NullPolicy != "required" {
				return nil, fmt.Errorf("synthetic field references must be unique required snapshot members")
			}
			if err := parse(identity.PhysicalField, ref.ObjectID); err != nil {
				return nil, err
			}
			if err := parse(identity.PhysicalFieldRevision, ref.RevisionID); err != nil {
				return nil, err
			}
			if fields[ids[ref.ObjectID]] != nil {
				return nil, fmt.Errorf("duplicate synthetic field reference")
			}
			fields[ids[ref.ObjectID]] = ref
		}
		seenNames := map[string]bool{}
		expected := map[string][][2]string{
			"orders":    {{"id", "text"}, {"amount", "numeric"}, {"paid_at", "timestamptz"}, {"customer_id", "text"}, {"is_valid", "integer"}},
			"customers": {{"id", "text"}, {"region", "text"}, {"first_paid_at", "timestamptz"}},
		}[relation.Relation]
		if len(relation.Fields) != len(expected) {
			return nil, fmt.Errorf("synthetic fields differ from the fixed schema")
		}
		for ordinal, field := range relation.Fields {
			for _, value := range []struct {
				prefix identity.Prefix
				value  string
			}{{identity.PhysicalField, field.FieldID}, {identity.PhysicalFieldRevision, field.RevisionID}} {
				if err := parse(value.prefix, value.value); err != nil {
					return nil, err
				}
				if seenObjects[ids[value.value]] {
					return nil, fmt.Errorf("duplicate synthetic field identity")
				}
				seenObjects[ids[value.value]] = true
			}
			ref := fields[ids[field.FieldID]]
			if field.Name != expected[ordinal][0] || field.DataType != expected[ordinal][1] || seenNames[field.Name] || ref == nil || ids[ref.RevisionID] != ids[field.RevisionID] {
				return nil, fmt.Errorf("synthetic field source references do not match the fixed schema")
			}
			seenNames[field.Name] = true
		}
	}
	metadata, _ := json.Marshal(map[string]any{"synthetic": true, "provenance": "declared_local_fixture", "comment": demoSourceDisclosure, "importedBy": actor.String()})
	content, _ := json.Marshal(snapshot.Execution.Relations)
	contentDigest := sourceDigest(content)
	scopeDigest := sourceDigest([]byte(demoSourceSchema))
	w, src, rev, snap := workspace.UUID(), ids[first.SourceID], ids[first.SourceRevisionID], ids[snapshotID]
	var statements []sourceStatement
	add := func(sql string, args ...any) { statements = append(statements, sourceStatement{sql, args}) }
	add(`INSERT INTO source_connections(id,workspace_id,adapter_kind,name,normalized_locator,status,metadata) VALUES($1,$2,'postgresql_catalog','合成演示：订单与客户',$3,'active',$4)`, src, w, first.SourceLocator, metadata)
	add(`INSERT INTO source_revisions(id,workspace_id,source_connection_id,content_digest,adapter_version,observed_at,metadata) VALUES($1,$2,$3,$4,'synthetic-demo/1',CURRENT_TIMESTAMP,$5)`, rev, w, src, contentDigest, metadata)
	add(`INSERT INTO source_snapshots(id,workspace_id,source_connection_id,source_revision_id,adapter_version,scope_digest,content_digest,history_quality,coverage_status) VALUES($1,$2,$3,$4,'synthetic-demo/1',$5,$6,'verified','complete')`, snap, w, src, rev, scopeDigest, contentDigest)
	add(`INSERT INTO source_snapshot_scope(workspace_id,snapshot_id,coverage_key,selector,config_digest,status,enumeration_complete,diagnostic_codes) VALUES($1,$2,$3,$3,$4,'complete',true,ARRAY['SYNTHETIC_DEMO_IMPORT'])`, w, snap, demoSourceSchema, scopeDigest)
	add(`INSERT INTO source_snapshot_diagnostics(workspace_id,snapshot_id,ordinal,code,severity,coverage_key,message) VALUES($1,$2,1,'SYNTHETIC_DEMO_IMPORT','info',$3,$4)`, w, snap, demoSourceSchema, demoSourceDisclosure)
	for _, relation := range snapshot.Execution.Relations {
		dataset, datasetRevision := ids[relation.DatasetID], ids[relation.DatasetRevisionID]
		locator := relation.Schema + "." + relation.Relation
		content, _ := json.Marshal(relation)
		digest := sourceDigest(content)
		// Deferred current-revision foreign keys allow insert-only initialization.
		add(`INSERT INTO physical_datasets(id,workspace_id,source_connection_id,external_key,qualified_name,current_revision_id) VALUES($1,$2,$3,$4,$4,$5)`, dataset, w, src, locator, datasetRevision)
		add(`INSERT INTO physical_dataset_revisions(id,workspace_id,physical_dataset_id,source_revision_id,dataset_kind,locator,content_digest,metadata) VALUES($1,$2,$3,$4,'table',$5,$6,$7)`, datasetRevision, w, dataset, rev, locator, digest, metadata)
		add(`INSERT INTO source_snapshot_members(workspace_id,snapshot_id,kind,object_id,revision_id,historical_name,historical_locator,content_digest,coverage_key) VALUES($1,$2,'dataset',$3,$4,$5,$6,$7,$8)`, w, snap, dataset, datasetRevision, relation.Relation, locator, digest, demoSourceSchema)
		for ordinal, field := range relation.Fields {
			fieldID, fieldRevision := ids[field.FieldID], ids[field.RevisionID]
			content, _ := json.Marshal(field)
			add(`INSERT INTO physical_fields(id,workspace_id,physical_dataset_id,external_key,name,current_revision_id) VALUES($1,$2,$3,$4,$4,$5)`, fieldID, w, dataset, field.Name, fieldRevision)
			add(`INSERT INTO physical_field_revisions(id,workspace_id,physical_field_id,dataset_revision_id,ordinal,data_type,nullable,metadata) VALUES($1,$2,$3,$4,$5,$6,false,$7)`, fieldRevision, w, fieldID, datasetRevision, ordinal+1, field.DataType, metadata)
			add(`INSERT INTO source_snapshot_members(workspace_id,snapshot_id,kind,object_id,revision_id,historical_name,historical_locator,content_digest,coverage_key,parent_object_id,parent_revision_id) VALUES($1,$2,'field',$3,$4,$5,$6,$7,$8,$9,$10)`, w, snap, fieldID, fieldRevision, field.Name, locator+"."+field.Name, sourceDigest(content), demoSourceSchema, dataset, datasetRevision)
		}
	}
	add(`INSERT INTO source_effective_snapshots(workspace_id,source_connection_id,scope_digest,snapshot_id,version) VALUES($1,$2,$3,$4,1)`, w, src, scopeDigest, snap)
	return statements, nil
}

func sourceDigest(value []byte) string {
	return fmt.Sprintf("sha256:%x", sha256.Sum256(value))
}
