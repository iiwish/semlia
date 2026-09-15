package governance

import (
	"bytes"
	"encoding/json"
	"fmt"
)

type ProductionTargetBaseline struct {
	Content              json.RawMessage
	RegistryWriteVersion *int
}

type ProductionBaseline struct {
	Head          HeadReference
	CanonicalJSON json.RawMessage
	Targets       map[string]ProductionTargetBaseline
}

func ReplayProductionChanges(target TargetDeclaration, baseline json.RawMessage, localMappings ...map[string]string) ([]ChangeSetItem, bool, error) {
	if err := validateProductionChangePaths(target); err != nil {
		return nil, false, err
	}
	var localIDs map[string]string
	if len(localMappings) > 0 {
		localIDs = localMappings[0]
	}
	var err error
	baseline, err = ResolveLocalReferences(baseline, nil)
	if err != nil {
		return nil, false, ErrPriorStateUnknown
	}
	target.Content, err = ResolveLocalReferences(target.Content, localIDs)
	if err != nil {
		return nil, false, err
	}
	target.Changes = append([]TargetChangeInput{}, target.Changes...)
	for i := range target.Changes {
		for _, value := range []*json.RawMessage{&target.Changes[i].BeforeValue, &target.Changes[i].AfterValue} {
			if *value != nil {
				*value, err = ResolveLocalReferences(*value, localIDs)
				if err != nil {
					return nil, false, err
				}
			}
		}
	}
	value, err := ParseProductionJSON(baseline)
	if err != nil {
		return nil, false, ErrPriorStateUnknown
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, false, ErrPriorStateUnknown
	}
	items := make([]ChangeSetItem, 0, len(target.Changes))
	for _, change := range target.Changes {
		before, exists := object[change.FieldPath]
		if change.Op == "add" && exists || change.Op != "add" && !exists {
			return nil, false, ErrContentMismatch
		}
		item := ChangeSetItem{FieldPath: change.FieldPath, Op: ChangeOp(change.Op), BeforeValue: change.BeforeValue, AfterValue: change.AfterValue}
		if change.BeforeValue != nil {
			canonicalBefore, _ := json.Marshal(before)
			supplied, err := CanonicalJSON(change.BeforeValue)
			if err != nil || !bytes.Equal(canonicalBefore, supplied) {
				return nil, false, fmt.Errorf("%w: beforeValue differs from published baseline", ErrContentMismatch)
			}
			item.BeforeValue = canonicalBefore
			item.BeforeDigest, err = DigestJSON(canonicalBefore)
			if err != nil {
				return nil, false, err
			}
		}
		if change.AfterValue != nil {
			item.AfterValue, err = CanonicalJSON(change.AfterValue)
			if err != nil {
				return nil, false, err
			}
			item.AfterDigest, err = DigestJSON(item.AfterValue)
			if err != nil {
				return nil, false, err
			}
		}
		if err := item.Validate(); err != nil {
			return nil, false, err
		}
		items = append(items, item)
	}
	got, err := ApplyChangeSet(baseline, items)
	if err != nil {
		return nil, false, err
	}
	want, err := CanonicalJSON(target.Content)
	if err != nil {
		return nil, false, err
	}
	if !bytes.Equal(got, want) {
		return nil, false, ErrContentMismatch
	}
	canonicalBase, err := CanonicalJSON(baseline)
	if err != nil {
		return nil, false, err
	}
	return items, bytes.Equal(canonicalBase, want), nil
}
