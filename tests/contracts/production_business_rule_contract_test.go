package contracts_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

func TestProductionBusinessRuleContracts(t *testing.T) {
	for _, path := range []string{"api/openapi/semlia.v1.yaml", "docs/specs/semantic-production/contracts/production.openapi.yaml"} {
		t.Run(path, func(t *testing.T) {
			doc, err := openapi3.NewLoader().LoadFromFile(filepath.Join(root, path))
			if err != nil {
				t.Fatal(err)
			}
			if err := doc.Validate(context.Background()); err != nil {
				t.Fatal(err)
			}
			endpoint := doc.Paths.Value("/api/v1/workspaces/{workspaceId}/production-operations/{operationId}/business-rule-confirmations")
			if endpoint == nil || endpoint.Get == nil || endpoint.Post == nil {
				t.Fatal("missing business rule methods")
			}
			if !productionHasParameter(endpoint.Parameters, endpoint.Post.Parameters, "header", "Idempotency-Key") || !productionHasParameter(endpoint.Parameters, endpoint.Get.Parameters, "query", "version") {
				t.Fatal("unbound business rule request")
			}
			body := map[string]any{"expectedVersion": 1, "setDigest": productionDigest(), "targetKey": "revenue", "action": "confirm", "evidenceId": productionID("evd")}
			productionAccepts(t, doc, "ProductionBusinessRuleRequest", body, true)
			for _, field := range []string{"expectedVersion", "setDigest", "targetKey", "action", "evidenceId"} {
				bad := productionClone(body)
				delete(bad, field)
				productionAccepts(t, doc, "ProductionBusinessRuleRequest", bad, false)
			}
			for _, field := range []string{"principalId", "contentDigest", "authorizationVersion", "confirmed"} {
				bad := productionClone(body)
				bad[field] = "forged"
				productionAccepts(t, doc, "ProductionBusinessRuleRequest", bad, false)
			}
			body["action"] = "revoke"
			productionAccepts(t, doc, "ProductionBusinessRuleRequest", body, false)
			delete(body, "evidenceId")
			productionAccepts(t, doc, "ProductionBusinessRuleRequest", body, true)
			body["evidenceId"] = nil
			productionAccepts(t, doc, "ProductionBusinessRuleRequest", body, false)
			delete(body, "evidenceId")
			body["action"] = "confirm"
			body["declaration"] = "An explicit human business rule declaration"
			productionAccepts(t, doc, "ProductionBusinessRuleRequest", body, true)
			body["evidenceId"] = productionID("evd")
			productionAccepts(t, doc, "ProductionBusinessRuleRequest", body, false)
		})
	}
}
