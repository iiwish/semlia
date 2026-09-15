package postgres

import (
	"context"
	"encoding/json"
	"strings"

	domain "github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
	"github.com/jackc/pgx/v5"
)

func businessRuleDeclaration(ver domain.ProductionVersion, target domain.ProductionTarget, statement string) (json.RawMessage, error) {
	if strings.TrimSpace(statement) == "" || len(statement) > 8192 {
		return nil, domain.ErrInvalidArgument
	}
	return json.Marshal(map[string]any{"schema": "semlia.business-rule-declaration/v1", "workspaceId": ver.WorkspaceID.String(), "operationId": ver.OperationID.String(), "productionVersion": ver.Version, "targetKey": target.LocalKey, "setDigest": ver.SetDigest, "contentDigest": target.ContentDigest, "declaredBy": ver.CreatedBy.String(), "statement": statement})
}

// The authenticated human's explicit declaration is new evidence, not a rewrite
// of source observations or of the immutable draft input.
func createBusinessRuleDeclarationTx(ctx context.Context, tx pgx.Tx, ver domain.ProductionVersion, target domain.ProductionTarget, statement string) (identity.EvidenceID, string, error) {
	empty := identity.EvidenceID{}
	raw, err := businessRuleDeclaration(ver, target, statement)
	if err != nil {
		return empty, "", err
	}
	digest, err := domain.DigestJSON(raw)
	if err != nil {
		return empty, "", err
	}
	id, err := identity.NewEvidenceID()
	if err != nil {
		return empty, "", err
	}
	_, err = tx.Exec(ctx, `INSERT INTO evidence_artifacts(id,workspace_id,evidence_type,locator,content_digest,metadata) VALUES($1,$2,'declared',$3,$4,$5)`, id.UUID(), ver.WorkspaceID.UUID(), "semlia://business-rule-declarations/"+id.String(), digest, raw)
	return id, digest, err
}

func businessRuleDeclarationDigestTx(ctx context.Context, tx pgx.Tx, ver domain.ProductionVersion, target domain.ProductionTarget, evidence string) (string, error) {
	id, err := identity.ParseEvidenceID(evidence)
	if err != nil {
		return "", domain.ErrEvidenceMissing
	}
	var raw []byte
	var digest, kind string
	if err := tx.QueryRow(ctx, `SELECT evidence_type,content_digest,metadata FROM evidence_artifacts WHERE workspace_id=$1 AND id=$2`, ver.WorkspaceID.UUID(), id.UUID()).Scan(&kind, &digest, &raw); err != nil {
		return "", governanceRepositoryError("read human declaration", err)
	}
	if kind != "declared" {
		return "", domain.ErrEvidenceMissing
	}
	var content struct {
		Statement string `json:"statement"`
	}
	if json.Unmarshal(raw, &content) != nil {
		return "", domain.ErrEvidenceMissing
	}
	expected, err := businessRuleDeclaration(ver, target, content.Statement)
	if err != nil {
		return "", domain.ErrEvidenceMissing
	}
	actual, err := domain.DigestJSON(raw)
	if err != nil {
		return "", err
	}
	expectedDigest, err := domain.DigestJSON(expected)
	if err != nil {
		return "", err
	}
	if digest != actual || actual != expectedDigest {
		return "", domain.ErrEvidenceMissing
	}
	return digest, nil
}
