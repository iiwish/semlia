package distribution

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/iiwish/semlia/internal/domain/authorization"
	d "github.com/iiwish/semlia/internal/domain/distribution"
	"github.com/iiwish/semlia/pkg/identity"
)

type AskKnowledge struct {
	Payload   json.RawMessage
	Digest    string
	ReleaseID identity.ReleaseID
	// Catalog lists what the caller may ask about, by display name. It never reaches the model prompt and is not
	// part of Digest, so it can be rebuilt on replay and cannot change what the model sees.
	Catalog []AskCatalogEntry
	// Ambiguities are question phrases that name several assets of one type with nothing to tell them apart. They
	// are part of Digest; a caller clarifies them instead of asking the model to guess.
	Ambiguities []d.AskAmbiguity
	// Matches bind question phrases to governed wording of authorized assets; like Catalog they are evidence for the
	// caller and are rebuilt deterministically on replay.
	Matches []d.AskMatch
}

// AskCatalogEntry carries only authorized semantic names: no ids, addresses or physical locations.
type AskCatalogEntry struct {
	Type    string
	Name    string
	Members []string
}

// InterpretationContext discloses only authorized semantic names and members,
// never physical locations, credentials or unpublished authoring content.
func (s *Service) InterpretationContext(ctx context.Context, request ResolveRequest) (json.RawMessage, error) {
	knowledge, err := s.AskKnowledge(ctx, request, "")
	return knowledge.Payload, err
}

// AskKnowledge compiles the caller's authorized release view into ask cards and selects what the question needs.
// Selection is deterministic, so recomputing it for the same question, release and grants yields the same digest.
func (s *Service) AskKnowledge(ctx context.Context, request ResolveRequest, question string) (AskKnowledge, error) {
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
	assets := append([]d.ReleasedAsset(nil), snapshot.Assets...)
	sort.Slice(assets, func(i, j int) bool { return assets[i].AssetID.String() < assets[j].AssetID.String() })
	readable := []d.ReleasedAsset{}
	for _, a := range assets {
		_, refusal, err := s.resolveSelector(ctx, request, d.Selector{AssetID: &a.AssetID}, "any", snapshot.Assets)
		if err != nil {
			return AskKnowledge{}, err
		}
		if refusal == nil {
			readable = append(readable, a)
		}
	}
	cards := d.CompileAskCards(readable)
	catalog := []AskCatalogEntry{}
	for _, card := range cards {
		entry := AskCatalogEntry{Type: string(card.Type), Name: card.Name}
		for _, m := range card.Members {
			entry.Members = append(entry.Members, firstNonEmpty(m.Name, m.ID))
		}
		catalog = append(catalog, entry)
	}
	selection := d.SelectAskKnowledge(question, cards, d.DefaultAskBudget)
	selected := map[string]any{"releaseId": snapshot.ReleaseID.String(), "assets": selection.Cards, "questionMatches": selection.Matches, "ambiguities": selection.Ambiguities, "coveringModels": selection.CoveringModels, "omittedAssets": selection.OmittedAssets}
	selected["manifestDigest"] = snapshot.ManifestDigest
	stable, err := json.Marshal(selected)
	if err != nil {
		return AskKnowledge{}, err
	}
	delete(selected, "manifestDigest")
	selected["currentTime"] = s.clock.Now().UTC()
	payload, err := json.Marshal(selected)
	return AskKnowledge{Payload: payload, Digest: fmt.Sprintf("sha256:%x", sha256.Sum256(stable)), ReleaseID: snapshot.ReleaseID, Catalog: catalog, Ambiguities: selection.Ambiguities, Matches: selection.Matches}, err
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
