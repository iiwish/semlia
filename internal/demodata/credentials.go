package demodata

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"reflect"

	discoveryapp "github.com/iiwish/semlia/internal/application/discovery"
	"github.com/iiwish/semlia/internal/domain/discovery"
	"github.com/iiwish/semlia/internal/domain/distribution"
	"github.com/iiwish/semlia/pkg/identity"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ConfigureSource enables normal read-only previews of the isolated fixture.
// Existing credentials are verified, never replaced or rotated by this helper.
func ConfigureSource(ctx context.Context, pool *pgxpool.Pool, workspace identity.WorkspaceID, actor identity.PrincipalID, snapshot *distribution.ReleaseSnapshot, password string, rootSecret []byte) error {
	if _, err := sourceStatements(workspace, actor, snapshot); err != nil {
		return err
	}
	if pool == nil || password == "" {
		return fmt.Errorf("synthetic preview requires a database and nonempty password")
	}
	cipher, err := discoveryapp.NewCredentialCipher(rootSecret)
	if err != nil {
		return err
	}
	rawSource, err := identity.FromUUID(identity.SourceConnection, snapshot.Execution.Relations[0].SourceID)
	if err != nil {
		rawSource, err = identity.Parse(identity.SourceConnection, snapshot.Execution.Relations[0].SourceID)
	}
	if err != nil {
		return err
	}
	source, err := identity.ParseSourceConnectionID(rawSource.String())
	if err != nil {
		return err
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
	if !exists {
		return fmt.Errorf("synthetic source must be imported before preview configuration")
	}
	var rawMetadata []byte
	var active *int64
	if err := tx.QueryRow(ctx, `SELECT metadata,active_credential_version FROM source_connections WHERE workspace_id=$1 AND id=$2 FOR UPDATE`, workspace.UUID(), source.UUID()).Scan(&rawMetadata, &active); err != nil {
		return err
	}
	metadata, configured, err := previewMetadataFor(rawMetadata, snapshot.Execution.Relations[0].SourceLocator)
	if err != nil {
		return err
	}
	var count int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM source_credentials WHERE workspace_id=$1 AND source_connection_id=$2`, workspace.UUID(), source.UUID()).Scan(&count); err != nil {
		return err
	}
	if active != nil {
		if *active != 1 || count != 1 || !configured {
			return fmt.Errorf("existing synthetic preview credential or configuration conflicts")
		}
		envelope := discovery.CredentialEnvelope{WorkspaceID: workspace, SourceConnectionID: source, Version: 1}
		if err := tx.QueryRow(ctx, `SELECT key_version,algorithm,nonce,ciphertext FROM source_credentials WHERE workspace_id=$1 AND source_connection_id=$2 AND version=1 AND retired_at IS NULL`, workspace.UUID(), source.UUID()).Scan(&envelope.KeyVersion, &envelope.Algorithm, &envelope.Nonce, &envelope.Ciphertext); err != nil {
			return err
		}
		secret, err := cipher.Open(envelope)
		if err != nil || subtle.ConstantTimeCompare([]byte(secret.Password), []byte(password)) != 1 {
			return fmt.Errorf("existing synthetic preview credential does not match")
		}
		return tx.Commit(ctx)
	}
	if count != 0 {
		return fmt.Errorf("synthetic source has unexpected inactive credentials")
	}
	nonce, ciphertext, keyVersion, err := cipher.Seal(workspace, source, 1, discovery.CredentialSecret{Password: password})
	if err != nil {
		return err
	}
	credential, err := identity.NewSourceCredentialID()
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO source_credentials(id,workspace_id,source_connection_id,version,key_version,algorithm,nonce,ciphertext,created_by) VALUES($1,$2,$3,1,$4,'AES-256-GCM',$5,$6,$7)`, credential.UUID(), workspace.UUID(), source.UUID(), keyVersion, nonce, ciphertext, actor.String()); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE source_connections SET metadata=$3,source_kind='postgresql',active_credential_version=1,version=version+1,updated_at=CURRENT_TIMESTAMP WHERE workspace_id=$1 AND id=$2 AND active_credential_version IS NULL`, workspace.UUID(), source.UUID(), metadata); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func previewMetadata(raw []byte) ([]byte, bool, error) {
	return previewMetadataFor(raw, demoSourceLocator)
}

func previewMetadataFor(raw []byte, locator string) ([]byte, bool, error) {
	var metadata map[string]any
	if err := json.Unmarshal(raw, &metadata); err != nil {
		return nil, false, err
	}
	if metadata["synthetic"] != true || metadata["provenance"] != "declared_local_fixture" {
		return nil, false, fmt.Errorf("preview source is not a declared synthetic fixture")
	}
	expected := map[string]any{"host": "127.0.0.1", "port": float64(5433), "database": demoSourceSchema, "username": demoSourceSchema, "sslMode": "disable"}
	if locator == MacoSourceLocator {
		expected = map[string]any{"host": "postgresql", "port": float64(5432), "database": "semlia_test", "username": "semlia_test_demo", "sslMode": "disable"}
	} else if locator != demoSourceLocator {
		return nil, false, fmt.Errorf("unsupported synthetic deployment")
	}
	present := 0
	for key, value := range expected {
		if existing, ok := metadata[key]; ok {
			if !reflect.DeepEqual(existing, value) {
				return nil, false, fmt.Errorf("synthetic preview configuration conflicts at %s", key)
			}
			present++
		}
	}
	if present != 0 && present != len(expected) {
		return nil, false, fmt.Errorf("synthetic preview configuration is incomplete")
	}
	for key, value := range expected {
		metadata[key] = value
	}
	encoded, err := json.Marshal(metadata)
	return encoded, present == len(expected), err
}
