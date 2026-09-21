package governance

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/iiwish/semlia/internal/domain/semantic"
	"github.com/iiwish/semlia/pkg/identity"
)

var productionAddress = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$`)

type ProductionPhysicalReference struct {
	SnapshotID string `json:"snapshotId"`
	Kind       string `json:"kind"`
	ObjectID   string `json:"objectId"`
	RevisionID string `json:"revisionId"`
}

type ProductionPublishedReference struct {
	Kind          string `json:"kind"`
	TargetID      string `json:"targetId"`
	ReleaseID     string `json:"releaseId"`
	RevisionID    string `json:"revisionId,omitempty"`
	ObjectVersion int    `json:"objectVersion,omitempty"`
	ContentDigest string `json:"contentDigest,omitempty"`
}

type ProductionContentReferences struct {
	Physical         []ProductionPhysicalReference
	Published        []ProductionPublishedReference
	LocalKeys        []string
	OwnerPrincipalID string
}

func ParseProductionPublishedReference(raw json.RawMessage) (ProductionPublishedReference, error) {
	value, err := ParseProductionJSON(raw)
	if err != nil {
		return ProductionPublishedReference{}, err
	}
	return parseProductionPublishedReference(value)
}

func ValidateProductionContentDeclarations(targets []TargetDeclaration) error {
	local := map[string]TargetDeclaration{}
	identities := map[string]bool{}
	for _, target := range targets {
		if proof := target.ReuseIdentity; proof != nil {
			if target.Intent != ProductionIntentCreate {
				return ErrInvalidArgument
			}
			if err := ValidateProductionTargetID(target.Kind, proof.TargetID); err != nil {
				return err
			}
			if _, err := identity.ParseProductionOperationID(proof.CreationOperationID); err != nil {
				return ErrInvalidArgument
			}
			if _, err := identity.ParseReleaseID(proof.CreationReleaseID); err != nil {
				return ErrInvalidArgument
			}
			if _, err := identity.ParseReleaseID(proof.AbsenceReleaseID); err != nil {
				return ErrInvalidArgument
			}
			if proof.ExpectedHead.Presence != PresencePresent || proof.ExpectedHead.ReleaseID == nil || proof.ExpectedHead.ReleaseID.IsZero() || proof.ExpectedHead.ManifestDigest == nil || !IsValidContentDigest(*proof.ExpectedHead.ManifestDigest) {
				return ErrInvalidArgument
			}
			if err := proof.ExpectedHead.Validate(); err != nil {
				return err
			}
		}
		local[target.LocalKey] = target
	}
	for _, target := range targets {
		if !productionText(target.Title, 1, 256) {
			return fmt.Errorf("%w: title", ErrInvalidArgument)
		}
		if len(target.EvidenceIDs) > 256 {
			return ErrLimitExceeded
		}
		for _, id := range target.EvidenceIDs {
			if _, err := identity.ParseEvidenceID(id); err != nil {
				return ErrInvalidArgument
			}
		}
		if target.Intent == ProductionIntentCreate {
			if target.TargetID != nil || target.BaseRevisionID != nil || target.BaseObjectVersion != nil || target.RegistryWriteVersion != nil || target.IdentityKey == nil || !productionText(*target.IdentityKey, 1, 512) {
				return fmt.Errorf("%w: create identity or baseline", ErrInvalidArgument)
			}
		} else {
			if target.TargetID == nil || target.IdentityKey != nil || target.RegistryWriteVersion != nil {
				return fmt.Errorf("%w: update target or identity", ErrInvalidArgument)
			}
			if err := ValidateProductionTargetID(target.Kind, *target.TargetID); err != nil {
				return err
			}
			if target.Kind == TargetKindSemanticAsset {
				if target.BaseRevisionID == nil || target.BaseObjectVersion != nil {
					return ErrInvalidArgument
				}
				if _, err := identity.ParseRevisionID(*target.BaseRevisionID); err != nil {
					return ErrInvalidArgument
				}
			} else if target.BaseRevisionID != nil || target.BaseObjectVersion == nil || *target.BaseObjectVersion < 1 {
				return ErrInvalidArgument
			}
		}
		identityKey := target.Kind + ":"
		if target.IdentityKey != nil {
			identityKey += *target.IdentityKey
		} else if target.TargetID != nil {
			identityKey += *target.TargetID
		}
		if identities[identityKey] {
			return fmt.Errorf("%w: duplicate target identity", ErrInvalidArgument)
		}
		identities[identityKey] = true
		refs, err := InspectProductionContent(target.Kind, target.Content)
		if err != nil {
			return fmt.Errorf("target %s: %w", target.LocalKey, err)
		}
		if target.Kind == TargetKindSemanticAsset && target.Intent == ProductionIntentCreate {
			var content struct {
				Address string `json:"address"`
			}
			if err := json.Unmarshal(target.Content, &content); err != nil || content.Address != *target.IdentityKey {
				return fmt.Errorf("%w: identityKey must equal asset address", ErrInvalidArgument)
			}
		}
		for _, key := range refs.LocalKeys {
			ref, ok := local[key]
			if !ok || ref.Kind != TargetKindSemanticAsset || ref.Intent != ProductionIntentCreate {
				return fmt.Errorf("%w: semantic reference must name a local create asset", ErrInvalidArgument)
			}
		}
		if err := validateProductionChangePaths(target); err != nil {
			return err
		}
	}
	return nil
}

func ValidateProductionTargetID(kind, value string) error {
	var err error
	switch kind {
	case TargetKindSemanticAsset:
		_, err = identity.ParseAssetID(value)
	case TargetKindPhysicalBinding:
		_, err = identity.ParsePhysicalBindingID(value)
	case TargetKindModelGrain:
		_, err = identity.ParseModelGrainID(value)
	case TargetKindEntityKey:
		_, err = identity.ParseEntityKeyID(value)
	case TargetKindJoinContract:
		_, err = identity.ParseJoinContractID(value)
	default:
		return ErrInvalidArgument
	}
	if err != nil {
		return fmt.Errorf("%w: target ID kind mismatch", ErrInvalidArgument)
	}
	return nil
}

func productionText(value any, min, max int) bool {
	text, ok := value.(string)
	return ok && utf8.ValidString(text) && utf8.RuneCountInString(text) >= min && utf8.RuneCountInString(text) <= max
}

func productionObject(value any, required, optional string) (map[string]any, error) {
	object, ok := value.(map[string]any)
	if !ok || object == nil {
		return nil, ErrInvalidArgument
	}
	allowed := map[string]bool{}
	for _, key := range strings.Fields(required) {
		allowed[key] = true
		if _, ok := object[key]; !ok {
			return nil, fmt.Errorf("%w: missing %s", ErrInvalidArgument, key)
		}
	}
	for _, key := range strings.Fields(optional) {
		allowed[key] = true
	}
	for key := range object {
		if !allowed[key] {
			return nil, fmt.Errorf("%w: unknown %s", ErrInvalidArgument, key)
		}
	}
	return object, nil
}

func productionEnum(value any, allowed string) bool {
	text, ok := value.(string)
	if !ok {
		return false
	}
	for _, option := range strings.Fields(allowed) {
		if text == option {
			return true
		}
	}
	return false
}

func InspectProductionContent(kind string, content json.RawMessage) (ProductionContentReferences, error) {
	refs := ProductionContentReferences{}
	value, err := ParseProductionJSON(content)
	if err != nil {
		return refs, err
	}
	var object map[string]any
	switch kind {
	case TargetKindSemanticAsset:
		object, err = productionObject(value, "address assetType displayName definition scope ownerPrincipalId", "spec")
		if err != nil {
			return refs, err
		}
		if !productionText(object["address"], 1, 512) || !productionAddress.MatchString(object["address"].(string)) || !productionEnum(object["assetType"], "business_object business_term metric data_asset analysis_model") || !productionText(object["displayName"], 1, 256) {
			return refs, ErrInvalidArgument
		}
		if object["definition"] != nil && !productionText(object["definition"], 1, 16384) {
			return refs, ErrInvalidArgument
		}
		if object["scope"] != nil && !productionText(object["scope"], 1, 4096) {
			return refs, ErrInvalidArgument
		}
		owner, ok := object["ownerPrincipalId"].(string)
		if !ok {
			return refs, ErrInvalidArgument
		}
		if _, err := identity.ParsePrincipalID(owner); err != nil {
			return refs, ErrInvalidArgument
		}
		refs.OwnerPrincipalID = owner
		raw, _ := json.Marshal(object["spec"])
		spec, err := semantic.ParseKnowledgeSpec(semantic.AssetType(object["assetType"].(string)), raw, false)
		if err != nil {
			return refs, fmt.Errorf("%w: %s", ErrInvalidArgument, err)
		}
		for _, ref := range spec.References() {
			refs.Published = append(refs.Published, ProductionPublishedReference{Kind: TargetKindSemanticAsset, TargetID: ref.AssetID, ReleaseID: ref.ReleaseID, RevisionID: ref.RevisionID})
		}
		if spec.DatasetRef != nil {
			refs.Physical = append(refs.Physical, ProductionPhysicalReference(*spec.DatasetRef))
		}
		for _, member := range spec.Members {
			if member.SourceFieldRef != nil {
				refs.Physical = append(refs.Physical, ProductionPhysicalReference(*member.SourceFieldRef))
			}
		}
	case TargetKindPhysicalBinding:
		object, err = productionObject(value, "asset dataset", "field transform")
	case TargetKindModelGrain:
		object, err = productionObject(value, "asset expression fields", "")
	case TargetKindEntityKey:
		object, err = productionObject(value, "asset fields uniqueness", "")
	case TargetKindJoinContract:
		object, err = productionObject(value, "leftDataset rightDataset pairs joinType cardinality expression", "notes")
	default:
		return refs, ErrInvalidArgument
	}
	if err != nil {
		return refs, err
	}
	for _, field := range []string{"transform", "expression", "notes"} {
		if value, ok := object[field]; ok {
			min := 1
			if field == "notes" {
				min = 0
			}
			if !productionText(value, min, 4096) {
				return refs, ErrInvalidArgument
			}
		}
	}
	if value, ok := object["uniqueness"]; ok && !productionEnum(value, "exact deduplicated") {
		return refs, ErrInvalidArgument
	}
	if value, ok := object["joinType"]; ok && !productionEnum(value, "inner left right full") {
		return refs, ErrInvalidArgument
	}
	if value, ok := object["cardinality"]; ok && !productionEnum(value, "one_to_one one_to_many many_to_one many_to_many") {
		return refs, ErrInvalidArgument
	}
	if asset, exists := object["asset"]; exists {
		if local, ok := asset.(map[string]any); ok && len(local) == 1 && local["localKey"] != nil {
			key, ok := local["localKey"].(string)
			if !ok || !IsValidLocalKey(key) {
				return refs, ErrInvalidArgument
			}
			refs.LocalKeys = append(refs.LocalKeys, key)
		} else {
			pin, err := parseProductionPublishedReference(asset)
			if err != nil || pin.Kind != TargetKindSemanticAsset {
				return refs, ErrInvalidArgument
			}
			refs.Published = append(refs.Published, pin)
		}
	}
	addPhysical := func(value any, requiredKind string) error {
		ref, err := parseProductionPhysicalReference(value)
		if err != nil || ref.Kind != requiredKind {
			return ErrInvalidArgument
		}
		refs.Physical = append(refs.Physical, ref)
		return nil
	}
	for _, field := range []string{"dataset", "leftDataset", "rightDataset", "field"} {
		if value, exists := object[field]; exists {
			kind := "dataset"
			if field == "field" {
				kind = "field"
			}
			if err := addPhysical(value, kind); err != nil {
				return refs, err
			}
		}
	}
	for _, field := range []string{"fields", "pairs"} {
		if value, exists := object[field]; exists {
			array, ok := value.([]any)
			if !ok || len(array) < 1 || len(array) > 64 {
				return refs, ErrInvalidArgument
			}
			seen := map[string]bool{}
			for _, entry := range array {
				canonical, _ := json.Marshal(entry)
				if seen[string(canonical)] {
					return refs, ErrInvalidArgument
				}
				seen[string(canonical)] = true
				if field == "fields" {
					if err := addPhysical(entry, "field"); err != nil {
						return refs, err
					}
				} else {
					pair, err := productionObject(entry, "left right", "")
					if err != nil {
						return refs, err
					}
					if err := addPhysical(pair["left"], "field"); err != nil {
						return refs, err
					}
					if err := addPhysical(pair["right"], "field"); err != nil {
						return refs, err
					}
				}
			}
		}
	}
	return refs, nil
}

func parseProductionPhysicalReference(value any) (ProductionPhysicalReference, error) {
	ref := ProductionPhysicalReference{}
	object, err := productionObject(value, "snapshotId kind objectId revisionId", "")
	if err != nil {
		return ref, err
	}
	for _, field := range []string{"snapshotId", "kind", "objectId", "revisionId"} {
		if _, ok := object[field].(string); !ok {
			return ref, ErrInvalidArgument
		}
	}
	ref = ProductionPhysicalReference{SnapshotID: object["snapshotId"].(string), Kind: object["kind"].(string), ObjectID: object["objectId"].(string), RevisionID: object["revisionId"].(string)}
	if _, err := identity.ParseSourceSnapshotID(ref.SnapshotID); err != nil {
		return ref, ErrInvalidArgument
	}
	if ref.Kind == "dataset" {
		if _, err := identity.ParsePhysicalDatasetID(ref.ObjectID); err != nil {
			return ref, ErrInvalidArgument
		}
		if _, err := identity.ParsePhysicalDatasetRevisionID(ref.RevisionID); err != nil {
			return ref, ErrInvalidArgument
		}
	} else if ref.Kind == "field" {
		if _, err := identity.ParsePhysicalFieldID(ref.ObjectID); err != nil {
			return ref, ErrInvalidArgument
		}
		if _, err := identity.ParsePhysicalFieldRevisionID(ref.RevisionID); err != nil {
			return ref, ErrInvalidArgument
		}
	} else {
		return ref, ErrInvalidArgument
	}
	return ref, nil
}

func parseProductionPublishedReference(value any) (ProductionPublishedReference, error) {
	ref := ProductionPublishedReference{}
	object, ok := value.(map[string]any)
	if !ok {
		return ref, ErrInvalidArgument
	}
	required := "kind targetId releaseId objectVersion contentDigest"
	if object["kind"] == TargetKindSemanticAsset {
		required = "kind targetId releaseId revisionId"
	}
	object, err := productionObject(value, required, "")
	if err != nil {
		return ref, err
	}
	raw, err := json.Marshal(object)
	if err != nil || json.Unmarshal(raw, &ref) != nil {
		return ref, ErrInvalidArgument
	}
	if err := ValidateProductionTargetID(ref.Kind, ref.TargetID); err != nil {
		return ref, err
	}
	if _, err := identity.ParseReleaseID(ref.ReleaseID); err != nil {
		return ref, ErrInvalidArgument
	}
	if ref.Kind == TargetKindSemanticAsset {
		if _, err := identity.ParseRevisionID(ref.RevisionID); err != nil {
			return ref, ErrInvalidArgument
		}
	} else if ref.ObjectVersion < 1 || !IsValidContentDigest(ref.ContentDigest) {
		return ref, ErrInvalidArgument
	}
	return ref, nil
}

func validateProductionChangePaths(target TargetDeclaration) error {
	allowed := map[string]string{
		TargetKindSemanticAsset:   "displayName definition scope ownerPrincipalId spec",
		TargetKindPhysicalBinding: "asset dataset field transform",
		TargetKindModelGrain:      "asset expression fields",
		TargetKindEntityKey:       "asset fields uniqueness",
		TargetKindJoinContract:    "leftDataset rightDataset pairs joinType cardinality expression notes",
	}
	seen := map[string]bool{}
	for _, change := range target.Changes {
		// References and arrays are atomic business fields; no identity subfield edits.
		if !productionEnum(change.FieldPath, allowed[target.Kind]) || seen[change.FieldPath] {
			return fmt.Errorf("%w: invalid or overlapping fieldPath", ErrInvalidArgument)
		}
		seen[change.FieldPath] = true
		item := ChangeSetItem{FieldPath: change.FieldPath, Op: ChangeOp(change.Op), BeforeValue: change.BeforeValue, AfterValue: change.AfterValue}
		for _, raw := range []json.RawMessage{change.BeforeValue, change.AfterValue} {
			if raw != nil {
				if _, err := ParseProductionJSON(raw); err != nil {
					return err
				}
			}
		}
		if change.BeforeValue != nil {
			item.BeforeDigest, _ = DigestJSON(change.BeforeValue)
		}
		if change.AfterValue != nil {
			item.AfterDigest, _ = DigestJSON(change.AfterValue)
		}
		if err := item.Validate(); err != nil {
			return err
		}
	}
	return nil
}
