package governance

import (
	"bytes"
	"encoding/json"

	domain "github.com/iiwish/semlia/internal/domain/governance"
)

// The model may suggest additional create declarations, but cannot rename a
// reserved identity, select a new published update target or mint reuse proof.
func CheckProductionSuggestionIdentities(before, after []domain.TargetDeclaration) error {
	original := map[string]domain.TargetDeclaration{}
	owners := map[string]bool{}
	for _, t := range before {
		original[t.LocalKey] = t
		refs, err := domain.InspectProductionContent(t.Kind, t.Content)
		if err != nil {
			return err
		}
		if refs.OwnerPrincipalID != "" {
			owners[refs.OwnerPrincipalID] = true
		}
	}
	identity := func(t domain.TargetDeclaration) []byte {
		raw, _ := json.Marshal(struct {
			Kind, Intent                          string
			TargetID, IdentityKey, BaseRevisionID *string
			BaseObjectVersion                     *int
			Reuse                                 *domain.ProductionReintroductionIdentity
		}{t.Kind, t.Intent, t.TargetID, t.IdentityKey, t.BaseRevisionID, t.BaseObjectVersion, t.ReuseIdentity})
		return raw
	}
	for _, t := range after {
		old, exists := original[t.LocalKey]
		if exists && !bytes.Equal(identity(old), identity(t)) {
			return ErrAIOutputInvalid
		}
		if !exists && (t.Intent != domain.ProductionIntentCreate || t.ReuseIdentity != nil) {
			return ErrAIOutputInvalid
		}
		refs, err := domain.InspectProductionContent(t.Kind, t.Content)
		if err != nil {
			return err
		}
		if refs.OwnerPrincipalID != "" && !owners[refs.OwnerPrincipalID] {
			return ErrAIOutputInvalid
		}
		if exists && t.Kind == domain.TargetKindSemanticAsset {
			var a, b map[string]json.RawMessage
			if json.Unmarshal(old.Content, &a) != nil || json.Unmarshal(t.Content, &b) != nil {
				return ErrAIOutputInvalid
			}
			for _, field := range []string{"address", "assetType", "ownerPrincipalId"} {
				x, _ := domain.CanonicalJSON(a[field])
				y, _ := domain.CanonicalJSON(b[field])
				if !bytes.Equal(x, y) {
					return ErrAIOutputInvalid
				}
			}
		}
	}
	return nil
}
