package semantic

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/iiwish/semlia/pkg/identity"
)

// Knowledge references retain their immutable publication basis in stored content.
type KnowledgeReference struct {
	AssetID    string `json:"assetId"`
	RevisionID string `json:"revisionId"`
	ReleaseID  string `json:"releaseId"`
	MemberID   string `json:"memberId,omitempty"`
}

type SourceReference struct {
	SnapshotID string `json:"snapshotId"`
	Kind       string `json:"kind"`
	ObjectID   string `json:"objectId"`
	RevisionID string `json:"revisionId"`
}

type KnowledgeMember struct {
	ID             string           `json:"id"`
	Name           string           `json:"name,omitempty"`
	ValueType      string           `json:"valueType,omitempty"`
	NullPolicy     string           `json:"nullPolicy,omitempty"`
	HistoryPolicy  string           `json:"historyPolicy,omitempty"`
	SourceFieldRef *SourceReference `json:"sourceFieldRef,omitempty"`
}

type KnowledgeExpression struct {
	Op        string               `json:"op"`
	Ref       *KnowledgeReference  `json:"ref,omitempty"`
	Parameter string               `json:"parameter,omitempty"`
	Value     json.RawMessage      `json:"value,omitempty"`
	Left      *KnowledgeExpression `json:"left,omitempty"`
	Right     *KnowledgeExpression `json:"right,omitempty"`
}

type KnowledgeParameter struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Required bool   `json:"required"`
}

type MemberBinding struct {
	SemanticRef KnowledgeReference `json:"semanticRef"`
	DataRef     KnowledgeReference `json:"dataRef"`
}

// The decoder rejects unknown fields and fields belonging to another knowledge type.
// Missing values are permitted only in drafts; publication uses the same parser.
type KnowledgeSpec struct {
	Grain                   string               `json:"grain,omitempty"`
	Keys                    []string             `json:"keys,omitempty"`
	IdentityPolicy          string               `json:"identityPolicy,omitempty"`
	Lifecycle               string               `json:"lifecycle,omitempty"`
	Members                 []KnowledgeMember    `json:"members,omitempty"`
	Capability              string               `json:"capability,omitempty"`
	SubjectRef              *KnowledgeReference  `json:"subjectRef,omitempty"`
	Predicate               *KnowledgeExpression `json:"predicate,omitempty"`
	Parameters              []KnowledgeParameter `json:"parameters,omitempty"`
	Stage                   string               `json:"stage,omitempty"`
	Kind                    string               `json:"kind,omitempty"`
	InputRef                *KnowledgeReference  `json:"inputRef,omitempty"`
	Aggregation             string               `json:"aggregation,omitempty"`
	Expression              *KnowledgeExpression `json:"expression,omitempty"`
	FilterRefs              []KnowledgeReference `json:"filterRefs,omitempty"`
	TimeAttributeRef        *KnowledgeReference  `json:"timeAttributeRef,omitempty"`
	Unit                    string               `json:"unit,omitempty"`
	NullPolicy              string               `json:"nullPolicy,omitempty"`
	ZeroDenominator         string               `json:"zeroDenominator,omitempty"`
	Rollup                  string               `json:"rollup,omitempty"`
	DatasetRef              *SourceReference     `json:"datasetRef,omitempty"`
	Coverage                string               `json:"coverage,omitempty"`
	RefreshFrequency        string               `json:"refreshFrequency,omitempty"`
	Sensitivity             string               `json:"sensitivity,omitempty"`
	BaseObjectRef           *KnowledgeReference  `json:"baseObjectRef,omitempty"`
	PublicAttributeRefs     []KnowledgeReference `json:"publicAttributeRefs,omitempty"`
	MetricRefs              []KnowledgeReference `json:"metricRefs,omitempty"`
	CompatibleTermRefs      []KnowledgeReference `json:"compatibleTermRefs,omitempty"`
	DefaultTimeAttributeRef *KnowledgeReference  `json:"defaultTimeAttributeRef,omitempty"`
	DataAssetRefs           []KnowledgeReference `json:"dataAssetRefs,omitempty"`
	MemberBindings          []MemberBinding      `json:"memberBindings,omitempty"`
	JoinContractIDs         []string             `json:"joinContractIds,omitempty"`
}

var memberIDPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
var knowledgeFields = map[AssetType]string{
	"business_object": "grain keys identityPolicy lifecycle members",
	"business_term":   "capability subjectRef predicate parameters stage nullPolicy",
	"metric":          "kind inputRef aggregation expression filterRefs timeAttributeRef unit nullPolicy zeroDenominator rollup",
	"data_asset":      "datasetRef members grain keys coverage refreshFrequency sensitivity",
	"analysis_model":  "baseObjectRef grain publicAttributeRefs metricRefs compatibleTermRefs defaultTimeAttributeRef dataAssetRefs memberBindings joinContractIds",
}

func ParseKnowledgeSpec(kind AssetType, raw json.RawMessage, complete bool) (KnowledgeSpec, error) {
	spec := KnowledgeSpec{}
	fields, ok := knowledgeFields[kind]
	bad := func(message string) (KnowledgeSpec, error) {
		return spec, fmt.Errorf("invalid %s spec: %s", kind, message)
	}
	if !ok {
		return bad("unknown knowledge type")
	}
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		if complete {
			return bad("spec is required for publication")
		}
		return spec, nil
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil || object == nil {
		return bad("expected object")
	}
	allowed := map[string]bool{}
	for _, field := range strings.Fields(fields) {
		allowed[field] = true
	}
	for key := range object {
		if !allowed[key] {
			return bad("unknown field " + key)
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&spec); err != nil {
		return bad(err.Error())
	}
	if len(spec.Members) > 256 || len(spec.MemberBindings) > 256 || len(spec.MetricRefs) > 64 || len(spec.Parameters) > 32 {
		return bad("collection limit exceeded")
	}
	memberIDs := map[string]bool{}
	for _, member := range spec.Members {
		if !memberIDPattern.MatchString(member.ID) || memberIDs[member.ID] {
			return bad("invalid or duplicate member ID")
		}
		memberIDs[member.ID] = true
		if !optionalEnum(member.ValueType, "string number integer boolean date timestamp") || !optionalEnum(member.NullPolicy, "required unknown excluded") || !optionalEnum(member.HistoryPolicy, "stable current_value event_time") {
			return bad("invalid member semantics")
		}
		if kind == "business_object" && member.SourceFieldRef != nil {
			return bad("business members cannot own source fields")
		}
		if member.SourceFieldRef != nil && (member.SourceFieldRef.Kind != "field" || member.SourceFieldRef.Validate() != nil) {
			return bad("invalid source member reference")
		}
		if complete && (strings.TrimSpace(member.Name) == "" || member.NullPolicy == "" || (kind == "business_object" && (member.ValueType == "" || member.HistoryPolicy == "")) || (kind == "data_asset" && member.SourceFieldRef == nil)) {
			return bad("incomplete member " + member.ID)
		}
		if spec.DatasetRef != nil && member.SourceFieldRef != nil && spec.DatasetRef.SnapshotID != member.SourceFieldRef.SnapshotID {
			return bad("member belongs to another snapshot")
		}
	}
	seenKeys := map[string]bool{}
	for _, key := range spec.Keys {
		if !memberIDs[key] || seenKeys[key] {
			return bad("invalid or duplicate key member")
		}
		seenKeys[key] = true
	}
	for _, ref := range spec.References() {
		if err := ref.Validate(); err != nil {
			return bad(err.Error())
		}
	}
	if spec.DatasetRef != nil && (spec.DatasetRef.Kind != "dataset" || spec.DatasetRef.Validate() != nil) {
		return bad("invalid dataset reference")
	}
	for _, ref := range append(append([]KnowledgeReference{}, spec.PublicAttributeRefs...), pointerRefs(spec.InputRef, spec.TimeAttributeRef, spec.DefaultTimeAttributeRef)...) {
		if ref.MemberID == "" {
			return bad("attribute reference requires memberId")
		}
	}
	for _, binding := range spec.MemberBindings {
		if binding.SemanticRef.MemberID == "" || binding.DataRef.MemberID == "" {
			return bad("binding requires member references")
		}
	}
	params := map[string]bool{}
	for _, param := range spec.Parameters {
		if !memberIDPattern.MatchString(param.Name) || params[param.Name] || !optionalEnum(param.Type, "string number integer boolean date timestamp") || param.Type == "" {
			return bad("invalid parameter")
		}
		params[param.Name] = true
	}
	if spec.Expression != nil {
		if err := validateExpression(spec.Expression, true, params, 0); err != nil {
			return bad(err.Error())
		}
	}
	if spec.Predicate != nil {
		if err := validateExpression(spec.Predicate, false, params, 0); err != nil {
			return bad(err.Error())
		}
	}
	if !optionalEnum(spec.Kind, "aggregate derived") || !optionalEnum(spec.Capability, "definition predicate") || !optionalEnum(spec.Aggregation, "sum avg count count_distinct min max") || !optionalEnum(spec.Stage, "object") || !optionalEnum(spec.ZeroDenominator, "null") || !optionalEnum(spec.Rollup, "recompute_from_inputs") || !optionalEnum(spec.NullPolicy, "exclude unknown_does_not_match required") {
		return bad("unsupported computation policy")
	}
	if kind == "metric" && ((spec.Kind == "aggregate" && spec.Expression != nil) || (spec.Kind == "derived" && (spec.InputRef != nil || spec.Aggregation != ""))) {
		return bad("mixed aggregate and derived contract")
	}
	if kind == "business_term" && spec.Capability == "definition" && (spec.Predicate != nil || len(spec.Parameters) > 0 || spec.Stage != "") {
		return bad("definition-only term cannot declare a predicate")
	}
	for _, id := range spec.JoinContractIDs {
		if _, err := identity.ParseJoinContractID(id); err != nil {
			return bad("invalid join contract ID")
		}
	}
	if !complete {
		return spec, nil
	}
	switch kind {
	case "business_object":
		if strings.TrimSpace(spec.Grain) == "" || len(spec.Keys) == 0 || strings.TrimSpace(spec.IdentityPolicy) == "" || strings.TrimSpace(spec.Lifecycle) == "" {
			return bad("grain, keys, identity policy and lifecycle required")
		}
	case "business_term":
		if spec.Capability == "" || (spec.Capability == "predicate" && (spec.SubjectRef == nil || spec.Predicate == nil || spec.Stage == "" || spec.NullPolicy != "unknown_does_not_match")) {
			return bad("term capability incomplete")
		}
	case "metric":
		if spec.Kind == "" || strings.TrimSpace(spec.Unit) == "" || spec.NullPolicy == "" || spec.TimeAttributeRef == nil {
			return bad("metric semantics incomplete")
		}
		if spec.Kind == "aggregate" && (spec.InputRef == nil || spec.Aggregation == "") {
			return bad("aggregate input required")
		}
		if spec.Kind == "derived" && (spec.Expression == nil || spec.ZeroDenominator != "null" || spec.Rollup != "recompute_from_inputs") {
			return bad("derived calculation and rollup required")
		}
	case "data_asset":
		if spec.DatasetRef == nil || len(spec.Members) == 0 || strings.TrimSpace(spec.Grain) == "" || len(spec.Keys) == 0 || strings.TrimSpace(spec.Coverage) == "" || strings.TrimSpace(spec.RefreshFrequency) == "" || strings.TrimSpace(spec.Sensitivity) == "" {
			return bad("data usage contract incomplete")
		}
	case "analysis_model":
		if spec.BaseObjectRef == nil || strings.TrimSpace(spec.Grain) == "" || len(spec.MetricRefs) == 0 || spec.DefaultTimeAttributeRef == nil || len(spec.DataAssetRefs) == 0 || len(spec.MemberBindings) == 0 {
			return bad("analysis model implementation incomplete")
		}
	}
	return spec, nil
}

func (ref KnowledgeReference) Validate() error {
	if _, err := identity.ParseAssetID(ref.AssetID); err != nil {
		return fmt.Errorf("invalid asset reference")
	}
	if _, err := identity.ParseRevisionID(ref.RevisionID); err != nil {
		return fmt.Errorf("invalid revision reference")
	}
	if _, err := identity.ParseReleaseID(ref.ReleaseID); err != nil {
		return fmt.Errorf("invalid release reference")
	}
	if ref.MemberID != "" && !memberIDPattern.MatchString(ref.MemberID) {
		return fmt.Errorf("invalid member reference")
	}
	return nil
}

func (ref SourceReference) Validate() error {
	if _, err := identity.ParseSourceSnapshotID(ref.SnapshotID); err != nil {
		return err
	}
	if ref.Kind == "dataset" {
		if _, err := identity.ParsePhysicalDatasetID(ref.ObjectID); err != nil {
			return err
		}
		_, err := identity.ParsePhysicalDatasetRevisionID(ref.RevisionID)
		return err
	}
	if ref.Kind == "field" {
		if _, err := identity.ParsePhysicalFieldID(ref.ObjectID); err != nil {
			return err
		}
		_, err := identity.ParsePhysicalFieldRevisionID(ref.RevisionID)
		return err
	}
	return fmt.Errorf("invalid source kind")
}

func (spec KnowledgeSpec) References() []KnowledgeReference {
	refs := pointerRefs(spec.SubjectRef, spec.InputRef, spec.TimeAttributeRef, spec.BaseObjectRef, spec.DefaultTimeAttributeRef)
	for _, list := range [][]KnowledgeReference{spec.FilterRefs, spec.PublicAttributeRefs, spec.MetricRefs, spec.CompatibleTermRefs, spec.DataAssetRefs} {
		refs = append(refs, list...)
	}
	for _, binding := range spec.MemberBindings {
		refs = append(refs, binding.SemanticRef, binding.DataRef)
	}
	var walk func(*KnowledgeExpression)
	walk = func(e *KnowledgeExpression) {
		if e == nil {
			return
		}
		if e.Ref != nil {
			refs = append(refs, *e.Ref)
		}
		walk(e.Left)
		walk(e.Right)
	}
	walk(spec.Expression)
	walk(spec.Predicate)
	return refs
}

func pointerRefs(values ...*KnowledgeReference) []KnowledgeReference {
	refs := []KnowledgeReference{}
	for _, ref := range values {
		if ref != nil {
			refs = append(refs, *ref)
		}
	}
	return refs
}

func optionalEnum(value, values string) bool {
	if value == "" {
		return true
	}
	for _, option := range strings.Fields(values) {
		if value == option {
			return true
		}
	}
	return false
}

func validateExpression(e *KnowledgeExpression, metric bool, params map[string]bool, depth int) error {
	if e == nil || depth > 16 {
		return fmt.Errorf("invalid expression depth")
	}
	leaf := e.Left == nil && e.Right == nil
	switch e.Op {
	case "ref":
		if !leaf || e.Ref == nil || e.Parameter != "" || len(e.Value) != 0 || (metric && e.Ref.MemberID != "") || (!metric && e.Ref.MemberID == "") {
			return fmt.Errorf("invalid expression reference")
		}
		return e.Ref.Validate()
	case "parameter":
		if metric || !leaf || e.Ref != nil || len(e.Value) != 0 || !params[e.Parameter] {
			return fmt.Errorf("undeclared expression parameter")
		}
		return nil
	case "literal":
		if metric || !leaf || e.Ref != nil || e.Parameter != "" || len(e.Value) == 0 {
			return fmt.Errorf("invalid literal")
		}
		var value any
		if err := json.Unmarshal(e.Value, &value); err != nil {
			return err
		}
		switch value.(type) {
		case string, bool, float64:
			return nil
		}
		return fmt.Errorf("literal must be scalar")
	default:
		ops := "eq neq lt lte gt gte and or"
		if metric {
			ops = "add subtract multiply divide"
		}
		if e.Op == "" || !optionalEnum(e.Op, ops) || e.Ref != nil || e.Parameter != "" || len(e.Value) != 0 {
			return fmt.Errorf("unsupported expression operator")
		}
		if err := validateExpression(e.Left, metric, params, depth+1); err != nil {
			return err
		}
		return validateExpression(e.Right, metric, params, depth+1)
	}
}
