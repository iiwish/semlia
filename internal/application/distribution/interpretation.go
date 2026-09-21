package distribution

import (
	"context"
	"encoding/json"

	"github.com/iiwish/semlia/internal/domain/authorization"
	d "github.com/iiwish/semlia/internal/domain/distribution"
	"github.com/iiwish/semlia/internal/domain/semantic"
)

// InterpretationContext discloses only authorized semantic names and members,
// never physical locations, credentials or unpublished authoring content.
func (s *Service) InterpretationContext(ctx context.Context, request ResolveRequest) (json.RawMessage, error) {
	decision, err := s.authorize(ctx, request.WorkspaceID, authorization.ActionSemanticResolve, authorization.Resource{Type: authorization.ScopeWorkspace, ID: request.WorkspaceID.UUID()}, request.PrincipalRef, request.TraceID)
	if err != nil {
		return nil, err
	}
	snapshot, _, _, refusal, err := s.selectSnapshot(ctx, request, decision, s.clock.Now())
	if err != nil {
		return nil, err
	}
	if refusal != nil {
		return json.RawMessage(`{"assets":[]}`), nil
	}
	items := []any{}
	bytesRemaining := 64 * 1024
	for _, a := range snapshot.Assets {
		if len(items) >= 128 {
			break
		}
		_, refusal, err := s.resolveSelector(ctx, request, d.Selector{AssetID: &a.AssetID}, "any", snapshot.Assets)
		if err != nil {
			return nil, err
		}
		if refusal != nil {
			continue
		}
		var content struct {
			Definition string                 `json:"definition"`
			Spec       semantic.KnowledgeSpec `json:"spec"`
		}
		if json.Unmarshal(a.Content, &content) != nil {
			continue
		}
		members := []map[string]string{}
		for _, m := range content.Spec.Members {
			if len(members) >= 64 {
				break
			}
			members = append(members, map[string]string{"id": m.ID, "name": m.Name, "valueType": m.ValueType})
		}
		item := map[string]any{"assetId": a.AssetID.String(), "revisionId": a.RevisionID.String(), "address": a.Address, "name": a.Name, "type": a.AssetType, "definition": content.Definition, "members": members, "capability": content.Spec.Capability, "metricRefs": content.Spec.MetricRefs, "publicAttributeRefs": content.Spec.PublicAttributeRefs, "compatibleTermRefs": content.Spec.CompatibleTermRefs, "defaultTimeAttributeRef": content.Spec.DefaultTimeAttributeRef}
		encoded, err := json.Marshal(item)
		if err != nil {
			return nil, err
		}
		if len(encoded) > bytesRemaining {
			continue
		}
		bytesRemaining -= len(encoded)
		items = append(items, item)
	}
	return json.Marshal(map[string]any{"releaseId": snapshot.ReleaseID.String(), "currentTime": s.clock.Now().UTC(), "assets": items})
}
