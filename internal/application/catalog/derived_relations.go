package catalog

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"slices"
	"sort"
	"strings"
	"unicode/utf8"

	domain "github.com/iiwish/semlia/internal/domain/catalog"
	"github.com/iiwish/semlia/internal/domain/semantic"
	"github.com/iiwish/semlia/pkg/identity"
)

const (
	// relationDerivedKind marks a cursor that points inside the spec-derived segment of the relations stream.
	relationDerivedKind = "relation_derived"
	// maxReferencingAssets bounds how many referencing specs are read for one asset's incoming relations.
	maxReferencingAssets = 500
	derivedAuthority     = "spec_references"
	maxCounterpartTitle  = 256
)

// relationSubject is the asset whose relations are being listed, with the content its outgoing references come from.
type relationSubject struct {
	ID      identity.AssetID
	Type    semantic.AssetType
	Content json.RawMessage
}

// derivedRelationRecords projects the typed relations implied by published specs: the asset's own references
// (outgoing) and the references other assets hold to it (incoming). Both are read from current revisions, so the
// result is always consistent with the specs; nothing is stored. Counterparts the caller may not read are omitted.
// The list is sorted so that a cursor can resume inside it.
func (service *Service) derivedRelationRecords(ctx context.Context, workspace identity.WorkspaceID, subject relationSubject, visible func(identity.AssetID) bool) ([]domain.AuthorityRecord, error) {
	merged := map[string]*projectedRelation{}
	add := func(projection projectedRelation, ok bool) {
		if !ok {
			return
		}
		if existing, found := merged[projection.record.ID]; found {
			existing.fields = append(existing.fields, projection.fields...)
			return
		}
		merged[projection.record.ID] = &projection
	}

	outgoing := semantic.DeriveSpecRelations(subject.ID.String(), subject.Type, specOf(subject.Content))
	objectIDs := make([]identity.AssetID, 0, len(outgoing))
	for _, relation := range outgoing {
		if id, err := identity.ParseAssetID(relation.ObjectAssetID); err == nil {
			objectIDs = append(objectIDs, id)
		}
	}
	refs, err := service.repository.ListCatalogAssetRefs(ctx, workspace, objectIDs)
	if err != nil {
		return nil, err
	}
	known := make(map[string]domain.AssetRef, len(refs))
	for _, ref := range refs {
		known[ref.ID.String()] = ref
	}
	for _, relation := range outgoing {
		counterpart, ok := known[relation.ObjectAssetID]
		if !ok || (visible != nil && !visible(counterpart.ID)) {
			continue
		}
		add(derivedRecord("outgoing", relation, subject.ID, subject.Type, counterpart.ID, counterpart.Type, counterpart.Title))
	}

	referencing, err := service.repository.ListCatalogAssetsReferencing(ctx, workspace, subject.ID, maxReferencingAssets)
	if err != nil {
		return nil, err
	}
	for _, other := range referencing {
		if visible != nil && !visible(other.ID) {
			continue
		}
		for _, relation := range semantic.DeriveSpecRelations(other.ID.String(), other.Type, specOf(other.Content)) {
			if relation.ObjectAssetID != subject.ID.String() {
				continue
			}
			// For an incoming relation the counterpart is the referencing asset.
			add(derivedRecord("incoming", relation, other.ID, other.Type, subject.ID, subject.Type, other.Title))
		}
	}

	records := make([]domain.AuthorityRecord, 0, len(merged))
	for _, projection := range merged {
		fields := append([]string(nil), projection.fields...)
		sort.Strings(fields)
		fields = slices.Compact(fields)
		projection.record.Relation.InferenceRule = truncateRunes("spec_reference:"+strings.Join(fields, ","), 160)
		records = append(records, projection.record)
	}
	sort.SliceStable(records, func(i, j int) bool {
		left, right := records[i], records[j]
		if left.Relation.Direction != right.Relation.Direction {
			return left.Relation.Direction == "outgoing"
		}
		if left.RelatedID != right.RelatedID {
			return left.RelatedID < right.RelatedID
		}
		if left.Relation.Predicate != right.Relation.Predicate {
			return left.Relation.Predicate < right.Relation.Predicate
		}
		return left.ID < right.ID
	})
	return records, nil
}

// projectedRelation pairs a record with the spec fields it was read from, so repeated references merge.
type projectedRelation struct {
	record domain.AuthorityRecord
	fields []string
}

func derivedRecord(direction string, relation semantic.DerivedRelation, subjectID identity.AssetID, subjectType semantic.AssetType,
	objectID identity.AssetID, objectType semantic.AssetType, counterpartTitle string) (projectedRelation, bool) {
	predicate, plane := relation.Predicate, relation.Plane
	// The endpoint policy that guards stored relations applies to projected ones. A reference the specific predicate
	// does not allow between these types still is a dependency, so it falls back to the generic predicate.
	valid := func() bool {
		return (semantic.Relation{Predicate: predicate, Plane: plane, AssertionState: semantic.Inferred, SubjectType: subjectType, ObjectType: objectType}).Validate() == nil
	}
	if !valid() {
		predicate, plane = semantic.DependsOn, semantic.DependencyPlane
		if !valid() {
			return projectedRelation{}, false
		}
	}
	counterpartID := objectID
	if direction == "incoming" {
		counterpartID = subjectID
	}
	return projectedRelation{fields: relation.Fields, record: domain.AuthorityRecord{
		Kind: "relation", ID: derivedRelationID(subjectID, predicate, objectID).String(),
		Authority: derivedAuthority, Status: string(semantic.Inferred), Label: string(predicate),
		RelatedID: counterpartID.String(),
		Relation: &domain.RelationAuthority{
			Direction: direction, Predicate: string(predicate), Plane: string(plane),
			AssertionState: string(semantic.Inferred), SubjectAssetID: subjectID, ObjectAssetID: objectID,
			CounterpartTitle: truncateRunes(counterpartTitle, maxCounterpartTitle),
		},
	}}, true
}

// derivedRelationID is deterministic and shaped like a relation id so the contract's typeid format holds. It names
// a projection, not a stored row: nothing persists under it.
func derivedRelationID(subject identity.AssetID, predicate semantic.RelationPredicate, object identity.AssetID) identity.RelationID {
	sum := sha256.Sum256([]byte("spec_reference|" + subject.String() + "|" + string(predicate) + "|" + object.String()))
	var raw [16]byte
	copy(raw[:], sum[:16])
	raw[6] = raw[6]&0x0f | 0x70
	raw[8] = raw[8]&0x3f | 0x80
	id, err := identity.RelationIDFromUUIDBytes(raw)
	if err != nil {
		return identity.RelationID{}
	}
	return id
}

func specOf(content json.RawMessage) semantic.KnowledgeSpec {
	var wrapper struct {
		Spec semantic.KnowledgeSpec `json:"spec"`
	}
	// Content that is not a typed spec has no references to project.
	if json.Unmarshal(content, &wrapper) != nil {
		return semantic.KnowledgeSpec{}
	}
	return wrapper.Spec
}

func truncateRunes(value string, limit int) string {
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	return string([]rune(value)[:limit])
}

// listRelationRecordPage serves the relations stream: spec-derived records first, then stored ones. A cursor either
// points inside the derived segment (by record id) or is an ordinary stored-record cursor.
func (service *Service) listRelationRecordPage(ctx context.Context, workspace identity.WorkspaceID, subject relationSubject,
	visible func(identity.AssetID) bool, limit int, cursor *domain.AuthorityRecordCursor) (AuthorityRecordPage, error) {
	derived, err := service.derivedRelationRecords(ctx, workspace, subject, visible)
	if err != nil {
		return AuthorityRecordPage{}, err
	}
	start, storedCursor, inDerived := 0, cursor, true
	if cursor != nil {
		if cursor.Kind == relationDerivedKind {
			found := false
			for index, record := range derived {
				if record.ID == cursor.ID {
					start, found = index+1, true
					break
				}
			}
			if !found {
				return AuthorityRecordPage{}, domain.ErrInvalidArgument
			}
			storedCursor = nil
		} else {
			start, inDerived = len(derived), false
		}
	}
	page := AuthorityRecordPage{Limit: limit}
	if inDerived {
		end := min(len(derived), start+limit)
		page.Items = append(page.Items, derived[start:end]...)
		start = end
	}
	remaining := limit - len(page.Items)
	storedLimit := 1
	if remaining > 0 {
		storedLimit = remaining + 1
	}
	stored, err := service.repository.ListCatalogAuthorityRecords(ctx, domain.ListAuthorityRecordsQuery{
		WorkspaceID: workspace, AssetID: subject.ID, Section: "relations", Limit: storedLimit, Cursor: storedCursor,
	})
	if err != nil {
		return AuthorityRecordPage{}, err
	}
	page.Total = int64(len(derived)) + stored.Total
	hasMoreDerived := inDerived && start < len(derived)
	if remaining > 0 {
		take := min(remaining, len(stored.Items))
		page.Items = append(page.Items, stored.Items[:take]...)
		if len(stored.Items) > remaining && len(page.Items) > 0 {
			last := page.Items[len(page.Items)-1]
			page.NextCursor = encodeCursor(authorityRecordCursorEnvelope{Kind: "catalog-authority-records",
				WorkspaceID: workspace.String(), AssetID: subject.ID.String(), Section: "relations",
				RecordKind: last.Kind, RecordID: last.ID})
		}
		return page, nil
	}
	// The page is full of derived records: more follows if derived records remain or any stored record exists.
	if hasMoreDerived || stored.Total > 0 || len(stored.Items) > 0 {
		last := page.Items[len(page.Items)-1]
		page.NextCursor = encodeCursor(authorityRecordCursorEnvelope{Kind: "catalog-authority-records",
			WorkspaceID: workspace.String(), AssetID: subject.ID.String(), Section: "relations",
			RecordKind: relationDerivedKind, RecordID: last.ID})
	}
	return page, nil
}
