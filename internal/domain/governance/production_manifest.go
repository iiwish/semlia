package governance

import (
	"encoding/json"
	"github.com/iiwish/semlia/pkg/identity"
)

func (head HeadReference) Validate() error {
	if head.Presence == PresenceAbsent && head.ReleaseID == nil && head.ManifestDigest == nil {
		return nil
	}
	if head.Presence == PresencePresent && head.ReleaseID != nil && !head.ReleaseID.IsZero() && head.ManifestDigest != nil && IsValidContentDigest(*head.ManifestDigest) {
		return nil
	}
	return ErrInvalidArgument
}

type productionManifestAsset struct {
	AssetID       string          `json:"assetId"`
	RevisionID    string          `json:"revisionId"`
	Compatibility json.RawMessage `json:"compatibility"`
	Position      int             `json:"position"`
}
type productionManifestObject struct {
	Kind          string `json:"kind"`
	TargetID      string `json:"targetId"`
	ObjectVersion int    `json:"objectVersion"`
	Position      int    `json:"position"`
}
type productionManifestSnapshot struct {
	Assets  []productionManifestAsset  `json:"assets"`
	Objects []productionManifestObject `json:"objects"`
}

// The snapshot is wire-shaped; the release digest retains the legacy digest
// algorithm, including the asset-only representation.
func ProductionManifestJSON(assets []ManifestEntry, objects []ObjectManifestEntry) (json.RawMessage, error) {
	if len(assets) > 1000 || len(objects) > 1000 {
		return nil, ErrLimitExceeded
	}
	snapshot := productionManifestSnapshot{Assets: []productionManifestAsset{}, Objects: []productionManifestObject{}}
	for _, asset := range assets {
		if err := asset.Validate(); err != nil {
			return nil, err
		}
		compatibility := asset.Compatibility
		if len(compatibility) == 0 {
			compatibility = json.RawMessage(`{}`)
		}
		snapshot.Assets = append(snapshot.Assets, productionManifestAsset{asset.AssetID.String(), asset.RevisionID.String(), compatibility, asset.Position})
	}
	for _, object := range objects {
		if err := object.Validate(); err != nil {
			return nil, err
		}
		id, err := object.TypedID()
		if err != nil {
			return nil, err
		}
		snapshot.Objects = append(snapshot.Objects, productionManifestObject{string(object.ObjectType), id, object.Version, object.Position})
	}
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return nil, err
	}
	if len(raw) > 1<<20 {
		return nil, ErrLimitExceeded
	}
	return CanonicalJSON(raw)
}

func ParseProductionManifestJSON(raw json.RawMessage) ([]ManifestEntry, []ObjectManifestEntry, error) {
	var snapshot productionManifestSnapshot
	if len(raw) > 1<<20 {
		return nil, nil, ErrLimitExceeded
	}
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return nil, nil, ErrPriorStateUnknown
	}
	if snapshot.Assets == nil || snapshot.Objects == nil {
		return nil, nil, ErrPriorStateUnknown
	}
	assets := []ManifestEntry{}
	objects := []ObjectManifestEntry{}
	for _, asset := range snapshot.Assets {
		a, err := identity.ParseAssetID(asset.AssetID)
		if err != nil {
			return nil, nil, err
		}
		r, err := identity.ParseRevisionID(asset.RevisionID)
		if err != nil {
			return nil, nil, err
		}
		assets = append(assets, ManifestEntry{AssetID: a, RevisionID: r, Compatibility: asset.Compatibility, Position: asset.Position})
	}
	for _, object := range snapshot.Objects {
		if err := ValidateProductionTargetID(object.Kind, object.TargetID); err != nil {
			return nil, nil, err
		}
		id, err := identity.ParseAny(object.TargetID)
		if err != nil {
			return nil, nil, err
		}
		objects = append(objects, ObjectManifestEntry{ObjectType: TargetObjectType(object.Kind), ObjectID: id.UUID(), Version: object.ObjectVersion, Position: object.Position})
	}
	if _, err := ProductionManifestJSON(assets, objects); err != nil {
		return nil, nil, err
	}
	return assets, objects, nil
}
