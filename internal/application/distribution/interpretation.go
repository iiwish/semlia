package distribution

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/iiwish/semlia/internal/domain/authorization"
	d "github.com/iiwish/semlia/internal/domain/distribution"
	"github.com/iiwish/semlia/internal/domain/semantic"
	"github.com/iiwish/semlia/pkg/identity"
)

type AskKnowledge struct {
	Payload   json.RawMessage
	Digest    string
	ReleaseID identity.ReleaseID
}

// InterpretationContext discloses only authorized semantic names and members,
// never physical locations, credentials or unpublished authoring content.
func (s *Service) InterpretationContext(ctx context.Context, request ResolveRequest) (json.RawMessage, error) {
	knowledge, err := s.AskKnowledge(ctx, request)
	return knowledge.Payload, err
}

func (s *Service) AskKnowledge(ctx context.Context, request ResolveRequest) (AskKnowledge, error) {
	if err := request.Input.Context.Validate(); err != nil {
		return AskKnowledge{}, err
	}
	decision, err := s.authorize(ctx, request.WorkspaceID, authorization.ActionSemanticResolve, authorization.Resource{Type: authorization.ScopeWorkspace, ID: request.WorkspaceID.UUID()}, request.PrincipalRef, request.TraceID)
	if err != nil {
		return AskKnowledge{}, err
	}
	snapshot, _, _, refusal, err := s.selectSnapshot(ctx, request, decision, s.clock.Now())
	if err != nil {
		return AskKnowledge{}, err
	}
	if refusal != nil {
		payload, _ := json.Marshal(map[string]any{"assets": []any{}, "refusal": refusal.Code})
		return AskKnowledge{Payload: payload, Digest: fmt.Sprintf("sha256:%x", sha256.Sum256(payload))}, nil
	}
	snapshot.Assets = append([]d.ReleasedAsset(nil), snapshot.Assets...)
	sort.Slice(snapshot.Assets, func(i, j int) bool { return snapshot.Assets[i].AssetID.String() < snapshot.Assets[j].AssetID.String() })
	items := []any{}
	bytesRemaining := 64 * 1024
	for _, a := range snapshot.Assets {
		if len(items) >= 128 {
			break
		}
		_, refusal, err := s.resolveSelector(ctx, request, d.Selector{AssetID: &a.AssetID}, "any", snapshot.Assets)
		if err != nil {
			return AskKnowledge{}, err
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
			return AskKnowledge{}, err
		}
		if len(encoded) > bytesRemaining {
			continue
		}
		bytesRemaining -= len(encoded)
		items = append(items, item)
	}
	stable, err := json.Marshal(map[string]any{"releaseId": snapshot.ReleaseID.String(), "manifestDigest": snapshot.ManifestDigest, "assets": items})
	if err != nil {
		return AskKnowledge{}, err
	}
	payload, err := json.Marshal(map[string]any{"releaseId": snapshot.ReleaseID.String(), "currentTime": s.clock.Now().UTC(), "assets": items})
	return AskKnowledge{Payload: payload, Digest: fmt.Sprintf("sha256:%x", sha256.Sum256(stable)), ReleaseID: snapshot.ReleaseID}, err
}
