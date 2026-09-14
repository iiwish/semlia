package governance_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	gov "github.com/iiwish/semlia/internal/domain/governance"
)

func TestProductionGenerationCreatesRelatedTargetsOnlyAfterHumanApplication(t *testing.T) {
	g := newProductionGenerationFixture(t)
	dataset, field := productionPhysicalFixture(t, g.f, g.w, g.payload["input"].(map[string]any))
	contents := map[string]map[string]any{
		"physical_binding": {"asset": map[string]any{"localKey": "orders"}, "dataset": dataset, "field": field, "transform": "id"},
		"model_grain":      {"asset": map[string]any{"localKey": "orders"}, "expression": "one row per order", "fields": []any{field}},
		"entity_key":       {"asset": map[string]any{"localKey": "orders"}, "fields": []any{field}, "uniqueness": "exact"},
		"join_contract":    {"leftDataset": dataset, "rightDataset": dataset, "pairs": []any{map[string]any{"left": field, "right": field}}, "joinType": "inner", "cardinality": "one_to_one", "expression": "left.id = right.id"},
	}
	targets := append([]any{}, g.payload["targets"].([]any)...)
	for _, kind := range []string{"physical_binding", "model_grain", "entity_key", "join_contract"} {
		targets = append(targets, map[string]any{"localKey": kind, "intent": "create", "kind": kind, "identityKey": "sales." + kind, "title": kind, "content": contents[kind], "changes": []any{}, "evidenceIds": []any{}})
	}
	raw, _ := json.Marshal(map[string]any{"schemaVersion": gov.ProductionSuggestionsSchema, "targets": targets})
	g.output = string(raw)
	queued := g.queue(t)
	g.run(t)
	result := g.get(t, queued.RunID)
	if result.Status != "succeeded" {
		t.Fatalf("related suggestions: %+v", result)
	}
	assertTableCount(t, g.f.pool, "proposals", 1)
	g.payload["expectedVersion"] = 1
	g.payload["suggestionRunId"] = queued.RunID.String()
	g.payload["targets"] = targets
	applied := productionRequestResult(t, g.h, http.MethodPut, g.path(), g.actor, "apply-related-targets", g.payload, http.StatusOK)
	if len(applied["proposalIds"].([]any)) != 5 {
		t.Fatalf("related proposals: %v", applied)
	}
	assertTableCount(t, g.f.pool, "releases", 0)
	read := productionRequestResult(t, g.h, http.MethodGet, g.path()+"?version=2", g.actor, "", nil, http.StatusOK)
	apps := read["generationApplications"].([]any)
	if len(apps) != 1 || len(apps[0].(map[string]any)["delta"].([]any)) != 0 {
		t.Fatalf("unchanged human application claimed corrections: %v", apps)
	}
	var initiators, agents int
	if err := g.f.pool.QueryRow(context.Background(), `SELECT count(*) FILTER(WHERE principal_id=$2),count(*) FILTER(WHERE role='agent') FROM production_contributors WHERE operation_id=$1`, g.op.UUID(), g.actor.UUID()).Scan(&initiators, &agents); err != nil {
		t.Fatal(err)
	}
	if initiators != 1 || agents != 1 {
		t.Fatalf("missing generation SoD attribution: %d %d", initiators, agents)
	}
}
