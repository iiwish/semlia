package semantic

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var (
	ErrInvalidAddress  = errors.New("invalid semantic address")
	ErrInvalidRevision = errors.New("invalid asset revision")
	ErrInvalidRelation = errors.New("invalid semantic relation")
)

var semanticVersion = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?$`)

type Address struct {
	namespace string
	key       string
}

func NewAddress(namespace, key string) (Address, error) {
	if !isNamespace(namespace) || !isToken(key) {
		return Address{}, fmt.Errorf("%w: %q.%q", ErrInvalidAddress, namespace, key)
	}
	return Address{namespace: namespace, key: key}, nil
}

func (address Address) Namespace() string { return address.namespace }

func (address Address) Key() string { return address.key }

func (address Address) String() string {
	if address.namespace == "" || address.key == "" {
		return ""
	}
	return address.namespace + "." + address.key
}

type AssetType string

const (
	BusinessObject AssetType = "business_object"
	BusinessTerm   AssetType = "business_term"
	AnalysisModel  AssetType = "analysis_model"
	DataAsset      AssetType = "data_asset"
	Metric         AssetType = "metric"
)

var assetTypes = []AssetType{BusinessObject, BusinessTerm, Metric, DataAsset, AnalysisModel}

func (assetType AssetType) valid() bool { return contains(assetTypes, assetType) }

type AssetRevision struct {
	Sequence      int64
	SchemaVersion string
	ContentDigest string
}

func (revision AssetRevision) Validate() error {
	if revision.Sequence < 1 {
		return fmt.Errorf("%w: sequence must be positive", ErrInvalidRevision)
	}
	if !semanticVersion.MatchString(revision.SchemaVersion) {
		return fmt.Errorf("%w: schema version must be semantic", ErrInvalidRevision)
	}
	if !strings.HasPrefix(revision.ContentDigest, "sha256:") || len(revision.ContentDigest) <= len("sha256:") {
		return fmt.Errorf("%w: content digest must use sha256", ErrInvalidRevision)
	}
	return nil
}

type RelationPlane string

const (
	TaxonomyPlane   RelationPlane = "taxonomy"
	SemanticPlane   RelationPlane = "semantic"
	DependencyPlane RelationPlane = "dependency"
)

type AssertionState string

const (
	Asserted   AssertionState = "asserted"
	Inferred   AssertionState = "inferred"
	Candidate  AssertionState = "candidate"
	Deprecated AssertionState = "deprecated"
)

type RelationPredicate string

const (
	Measures     RelationPredicate = "measures"
	Describes    RelationPredicate = "describes"
	DependsOn    RelationPredicate = "depends_on"
	DerivedFrom  RelationPredicate = "derived_from"
	FiltersBy    RelationPredicate = "filters_by"
	SynonymOf    RelationPredicate = "synonym_of"
	Contains     RelationPredicate = "contains"
	BroaderThan  RelationPredicate = "broader_than"
	NarrowerThan RelationPredicate = "narrower_than"
	EquivalentTo RelationPredicate = "equivalent_to"
	DisjointWith RelationPredicate = "disjoint_with"
)

type Relation struct {
	Predicate      RelationPredicate
	Plane          RelationPlane
	AssertionState AssertionState
	SubjectType    AssetType
	ObjectType     AssetType
}

type relationPolicy struct {
	plane   RelationPlane
	sources []AssetType
	targets []AssetType
	matched bool
}

var relationPolicies = map[RelationPredicate]relationPolicy{
	Measures:     {plane: SemanticPlane, sources: []AssetType{Metric}, targets: []AssetType{BusinessObject, AnalysisModel}},
	Describes:    {plane: SemanticPlane, sources: []AssetType{BusinessTerm, DataAsset}, targets: []AssetType{BusinessObject, AnalysisModel}},
	DependsOn:    {plane: DependencyPlane, sources: assetTypes, targets: assetTypes},
	DerivedFrom:  {plane: DependencyPlane, sources: []AssetType{Metric, DataAsset}, targets: []AssetType{Metric, DataAsset}},
	FiltersBy:    {plane: SemanticPlane, sources: []AssetType{Metric, AnalysisModel}, targets: []AssetType{BusinessTerm}},
	SynonymOf:    {plane: TaxonomyPlane, sources: assetTypes, targets: assetTypes, matched: true},
	Contains:     {plane: SemanticPlane, sources: []AssetType{AnalysisModel}, targets: assetTypes},
	BroaderThan:  {plane: TaxonomyPlane, sources: assetTypes, targets: assetTypes, matched: true},
	NarrowerThan: {plane: TaxonomyPlane, sources: assetTypes, targets: assetTypes, matched: true},
	EquivalentTo: {plane: TaxonomyPlane, sources: assetTypes, targets: assetTypes, matched: true},
	DisjointWith: {plane: TaxonomyPlane, sources: assetTypes, targets: assetTypes, matched: true},
}

func (relation Relation) Validate() error {
	policy, ok := relationPolicies[relation.Predicate]
	if !ok {
		return fmt.Errorf("%w: unknown predicate %q", ErrInvalidRelation, relation.Predicate)
	}
	if relation.Plane != policy.plane {
		return fmt.Errorf("%w: predicate %q belongs to %q", ErrInvalidRelation, relation.Predicate, policy.plane)
	}
	if !relation.SubjectType.valid() || !relation.ObjectType.valid() ||
		!contains(policy.sources, relation.SubjectType) || !contains(policy.targets, relation.ObjectType) {
		return fmt.Errorf("%w: predicate %q does not allow %q to %q", ErrInvalidRelation, relation.Predicate, relation.SubjectType, relation.ObjectType)
	}
	if policy.matched && relation.SubjectType != relation.ObjectType {
		return fmt.Errorf("%w: predicate %q requires matching endpoint types", ErrInvalidRelation, relation.Predicate)
	}
	switch relation.AssertionState {
	case Asserted, Inferred, Candidate, Deprecated:
		return nil
	default:
		return fmt.Errorf("%w: unknown assertion state %q", ErrInvalidRelation, relation.AssertionState)
	}
}

func isNamespace(value string) bool {
	parts := strings.Split(value, ".")
	if len(parts) == 0 {
		return false
	}
	for _, part := range parts {
		if !isToken(part) {
			return false
		}
	}
	return true
}

func isToken(value string) bool {
	if value == "" || value[0] < 'a' || value[0] > 'z' {
		return false
	}
	for _, character := range value[1:] {
		if (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '_' {
			return false
		}
	}
	return true
}

func contains[T comparable](values []T, target T) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
