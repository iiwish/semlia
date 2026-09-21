package distribution

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/iiwish/semlia/internal/domain/semantic"
	"github.com/iiwish/semlia/pkg/identity"
)

// Analysis models are selected against the entire demand, never the first binding.
func buildKnowledgePlan(query SemanticQueryInput, snapshot ReleaseSnapshot, queryID identity.SemanticQueryID, planID identity.ResolvedSemanticPlanID, selected []ReleasedAsset, now time.Time) (ResolvedSemanticPlan, *Refusal, error) {
	refuse := func(code RefusalCode, message string) (ResolvedSemanticPlan, *Refusal, error) {
		return ResolvedSemanticPlan{}, &Refusal{Code: code, Clarification: message}, nil
	}
	assets := map[string]ReleasedAsset{}
	for _, asset := range snapshot.Assets {
		assets[asset.AssetID.String()] = asset
	}
	selectorAssets := selected
	if selectorAssets == nil {
		selectorAssets = snapshot.Assets
	}
	canonical := func(selector Selector) (Selector, error) {
		match, refusal := MatchSelector(selector, "any", selectorAssets)
		if refusal != nil {
			return selector, fmt.Errorf("unresolved selector")
		}
		id := match.Asset.AssetID
		return Selector{AssetID: &id, MemberID: selector.MemberID}, nil
	}
	query.Measures = append([]Selector(nil), query.Measures...)
	query.Dimensions = append([]Selector(nil), query.Dimensions...)
	query.Filters = append([]Filter(nil), query.Filters...)
	query.Order = append([]Order(nil), query.Order...)
	var err error
	for i := range query.Measures {
		query.Measures[i], err = canonical(query.Measures[i])
		if err != nil {
			return refuse(RefusalNoMatch, "指标无法定位到已发布版本。")
		}
	}
	for i := range query.Dimensions {
		query.Dimensions[i], err = canonical(query.Dimensions[i])
		if err != nil || query.Dimensions[i].MemberID == "" {
			return refuse(RefusalNoMatch, "分组需要明确的业务对象属性成员。")
		}
	}
	for i := range query.Filters {
		query.Filters[i].Selector, err = canonical(query.Filters[i].Selector)
		if err != nil {
			return refuse(RefusalNoMatch, "筛选条件无法定位。")
		}
	}
	for i := range query.Order {
		query.Order[i].Selector, err = canonical(query.Order[i].Selector)
		if err != nil {
			return refuse(RefusalNoMatch, "排序成员无法定位。")
		}
	}
	if query.TimeRange != nil {
		tr := *query.TimeRange
		tr.Selector, err = canonical(tr.Selector)
		if err != nil || tr.Selector.MemberID == "" {
			return refuse(RefusalInvalidTimeRange, "统计时间需要明确的属性成员。")
		}
		query.TimeRange = &tr
	}
	key := func(ref semantic.KnowledgeReference) string { return ref.AssetID + "/" + ref.MemberID }
	selectorKey := func(s Selector) string { return s.AssetID.String() + "/" + s.MemberID }
	refFor := func(s Selector) semantic.KnowledgeReference {
		a := assets[s.AssetID.String()]
		return semantic.KnowledgeReference{AssetID: a.AssetID.String(), RevisionID: a.RevisionID.String(), ReleaseID: snapshot.ReleaseID.String(), MemberID: s.MemberID}
	}
	readSpec := func(a ReleasedAsset) (semantic.KnowledgeSpec, error) {
		var c struct {
			Spec json.RawMessage `json:"spec"`
		}
		if json.Unmarshal(a.Content, &c) != nil {
			return semantic.KnowledgeSpec{}, ErrInvalidArgument
		}
		return semantic.ParseKnowledgeSpec(a.AssetType, c.Spec, true)
	}
	wantedAttrs := append([]Selector{}, query.Dimensions...)
	wantedTerms := []Selector{}
	for _, f := range query.Filters {
		if assets[f.Selector.AssetID.String()].AssetType == semantic.BusinessTerm {
			wantedTerms = append(wantedTerms, f.Selector)
		} else if f.Selector.MemberID != "" {
			wantedAttrs = append(wantedAttrs, f.Selector)
		}
	}
	if query.TimeRange != nil {
		wantedAttrs = append(wantedAttrs, query.TimeRange.Selector)
	}
	for _, o := range query.Order {
		if o.Selector.MemberID != "" {
			wantedAttrs = append(wantedAttrs, o.Selector)
		}
	}
	models := []ReleasedAsset{}
	for _, a := range snapshot.Assets {
		if a.AssetType != semantic.AnalysisModel || (query.ModelID != nil && *query.ModelID != a.AssetID) {
			continue
		}
		spec, e := readSpec(a)
		if e != nil {
			continue
		}
		covers := func(refs []semantic.KnowledgeReference, selectors []Selector) bool {
			for _, s := range selectors {
				found := false
				for _, r := range refs {
					if key(r) == selectorKey(s) && r.RevisionID == assets[s.AssetID.String()].RevisionID.String() {
						found = true
					}
				}
				if !found {
					return false
				}
			}
			return true
		}
		attrs := append([]semantic.KnowledgeReference{}, spec.PublicAttributeRefs...)
		if spec.DefaultTimeAttributeRef != nil {
			attrs = append(attrs, *spec.DefaultTimeAttributeRef)
		}
		if covers(spec.MetricRefs, query.Measures) && covers(attrs, wantedAttrs) && covers(spec.CompatibleTermRefs, wantedTerms) {
			models = append(models, a)
		}
	}
	if len(models) == 0 {
		return refuse(RefusalMissingPhysicalBinding, "没有已发布分析模型同时覆盖指标、属性、口径与统计时间。")
	}
	if len(models) > 1 {
		return refuse(RefusalAmbiguousMatch, "多个分析模型可覆盖完整需求，请明确选择模型。")
	}
	model := models[0]
	spec, _ := readSpec(model)
	used := map[string]ReleasedAsset{model.AssetID.String(): model}
	resolveRef := func(ref semantic.KnowledgeReference, kind semantic.AssetType) (ReleasedAsset, semantic.KnowledgeSpec, error) {
		a, ok := assets[ref.AssetID]
		if !ok || a.RevisionID.String() != ref.RevisionID || (kind != "" && a.AssetType != kind) {
			return a, semantic.KnowledgeSpec{}, ErrInvalidArgument
		}
		s, e := readSpec(a)
		if e != nil {
			return a, s, e
		}
		if ref.MemberID != "" {
			found := false
			for _, m := range s.Members {
				if m.ID == ref.MemberID {
					found = true
				}
			}
			if !found {
				return a, s, ErrInvalidArgument
			}
		}
		used[ref.AssetID] = a
		return a, s, nil
	}
	_, base, baseErr := resolveRef(*spec.BaseObjectRef, semantic.BusinessObject)
	if baseErr != nil || base.Grain != spec.Grain {
		return refuse(RefusalInvalidPlan, "模型基础对象版本不一致。")
	}
	ir := &ExecutionProvenance{}
	if snapshot.Execution == nil {
		return refuse(RefusalMissingPhysicalBinding, "模型没有固定来源执行快照。")
	}
	ir.CompilerVersion = snapshot.Execution.CompilerVersion
	modelPin := ResolvedAsset{AssetID: model.AssetID, RevisionID: model.RevisionID, Address: model.Address, AssetType: model.AssetType}
	ir.Model = &modelPin
	bindings := map[string]ExecutionBinding{}
	objects := map[string]ResolvedObject{}
	relations := map[string]ExecutionRelation{}
	bindAttribute := func(ref semantic.KnowledgeReference) (ExecutionBinding, error) {
		_, objectSpec, e := resolveRef(ref, semantic.BusinessObject)
		if e != nil || ref.MemberID == "" {
			return ExecutionBinding{}, ErrInvalidArgument
		}
		if b, ok := bindings[key(ref)]; ok {
			return b, nil
		}
		matches := []semantic.MemberBinding{}
		for _, b := range spec.MemberBindings {
			if key(b.SemanticRef) == key(ref) && b.SemanticRef.RevisionID == ref.RevisionID {
				matches = append(matches, b)
			}
		}
		if len(matches) != 1 {
			return ExecutionBinding{}, ErrInvalidArgument
		}
		dataRef := matches[0].DataRef
		declared := false
		for _, r := range spec.DataAssetRefs {
			if r.AssetID == dataRef.AssetID && r.RevisionID == dataRef.RevisionID {
				declared = true
			}
		}
		if !declared {
			return ExecutionBinding{}, ErrInvalidArgument
		}
		data, dataSpec, e := resolveRef(dataRef, semantic.DataAsset)
		if e != nil || objectSpec.Grain != dataSpec.Grain {
			return ExecutionBinding{}, ErrInvalidArgument
		}
		var field *semantic.SourceReference
		for _, m := range dataSpec.Members {
			if m.ID == dataRef.MemberID {
				field = m.SourceFieldRef
			}
		}
		if dataSpec.DatasetRef == nil || field == nil {
			return ExecutionBinding{}, ErrInvalidArgument
		}
		datasetID, _ := identity.ParsePhysicalDatasetID(dataSpec.DatasetRef.ObjectID)
		datasetRev, _ := identity.ParsePhysicalDatasetRevisionID(dataSpec.DatasetRef.RevisionID)
		fieldID, _ := identity.ParsePhysicalFieldID(field.ObjectID)
		fieldRev, _ := identity.ParsePhysicalFieldRevisionID(field.RevisionID)
		var relation ExecutionRelation
		for _, r := range snapshot.Execution.Relations {
			if r.DatasetID == datasetID.UUID() && r.DatasetRevisionID == datasetRev.UUID() {
				relation = r
			}
		}
		fieldFound := false
		for _, f := range relation.Fields {
			if f.FieldID == fieldID.UUID() && f.RevisionID == fieldRev.UUID() {
				fieldFound = true
			}
		}
		if !fieldFound {
			return ExecutionBinding{}, ErrInvalidArgument
		}
		// Retain the release's governed binding that captured this dataset version.
		pinFound := false
		for _, b := range snapshot.Bindings {
			if b.AssetID == data.AssetID && b.DatasetID == datasetID && !b.Retired && b.Transform == "" {
				for _, p := range snapshot.Execution.BindingPins {
					if p.BindingID == b.ID.UUID() && p.Version == b.Version && p.DatasetRevisionID == datasetRev.UUID() {
						pinFound = true
						objects[b.ID.String()] = ResolvedObject{ObjectType: "physical_binding", ObjectID: b.ID.String(), Version: b.Version}
						exists := false
						for _, prior := range ir.BindingPins {
							if prior == p {
								exists = true
							}
						}
						if !exists {
							ir.BindingPins = append(ir.BindingPins, p)
						}
					}
				}
			}
		}
		if !pinFound {
			return ExecutionBinding{}, ErrInvalidArgument
		}
		relations[relation.DatasetID] = relation
		nullPolicy := ""
		for _, member := range objectSpec.Members {
			if member.ID == ref.MemberID {
				nullPolicy = map[string]string{"required": "required", "excluded": "exclude", "unknown": ""}[member.NullPolicy]
			}
		}
		binding := ExecutionBinding{AssetID: ref.AssetID, MemberID: ref.MemberID, DatasetID: relation.DatasetID, FieldID: fieldID.UUID(), NullPolicy: nullPolicy}
		bindings[key(ref)] = binding
		return binding, nil
	}
	baseKey := ExecutionKey{}
	for _, member := range base.Keys {
		ref := *spec.BaseObjectRef
		ref.MemberID = member
		binding, err := bindAttribute(ref)
		if err != nil {
			return refuse(RefusalMissingPhysicalBinding, "基础对象业务键缺少已发布的成员映射。")
		}
		if baseKey.DatasetID != "" && baseKey.DatasetID != binding.DatasetID {
			return refuse(RefusalIncompatibleGrain, "基础对象业务键缺少同一数据集的成员映射。")
		}
		baseKey.DatasetID = binding.DatasetID
		baseKey.FieldIDs = append(baseKey.FieldIDs, binding.FieldID)
	}
	ir.Keys = []ExecutionKey{baseKey}
	for _, s := range wantedAttrs {
		if _, e := bindAttribute(refFor(s)); e != nil {
			return refuse(RefusalMissingPhysicalBinding, "所需属性缺少唯一、同版本的数据成员映射。")
		}
	}
	visited, visiting := map[string]bool{}, map[string]bool{}
	fixedTerms := map[string]semantic.KnowledgeReference{}
	filterBasis := ""
	haveBasis := false
	var addMetric func(semantic.KnowledgeReference, int) error
	addMetric = func(ref semantic.KnowledgeReference, depth int) error {
		if depth > 32 || visiting[ref.AssetID] {
			return ErrInvalidArgument
		}
		_, m, e := resolveRef(ref, semantic.Metric)
		if e != nil {
			return e
		}
		if visited[ref.AssetID] {
			return nil
		}
		if m.TimeAttributeRef == nil || key(*m.TimeAttributeRef) != key(*spec.DefaultTimeAttributeRef) || m.TimeAttributeRef.RevisionID != spec.DefaultTimeAttributeRef.RevisionID {
			return ErrInvalidArgument
		}
		if _, _, err := resolveRef(*m.TimeAttributeRef, semantic.BusinessObject); err != nil {
			return err
		}
		visiting[ref.AssetID] = true
		if m.Kind == "aggregate" {
			if m.InputRef.AssetID != spec.BaseObjectRef.AssetID {
				return ErrInvalidArgument
			}
			b, e := bindAttribute(*m.InputRef)
			if e != nil {
				return e
			}
			b.AssetID = ref.AssetID
			b.MemberID = ""
			b.Aggregation = m.Aggregation
			b.NullPolicy = m.NullPolicy
			bindings[key(ref)] = b
			filters := []string{}
			for _, r := range m.FilterRefs {
				filters = append(filters, r.AssetID+"@"+r.RevisionID)
				fixedTerms[r.AssetID] = r
			}
			sort.Strings(filters)
			basis := strings.Join(filters, ",")
			if haveBasis && filterBasis != basis {
				return ErrInvalidArgument
			}
			filterBasis = basis
			haveBasis = true
		} else {
			if len(m.FilterRefs) > 0 {
				return ErrInvalidArgument
			}
			var walk func(*semantic.KnowledgeExpression) error
			walk = func(e *semantic.KnowledgeExpression) error {
				if e == nil {
					return ErrInvalidArgument
				}
				if e.Op == "ref" {
					return addMetric(*e.Ref, depth+1)
				}
				if err := walk(e.Left); err != nil {
					return err
				}
				return walk(e.Right)
			}
			if e := walk(m.Expression); e != nil {
				return e
			}
			ir.Calculations = append(ir.Calculations, ExecutionCalculation{AssetID: ref.AssetID, Expression: m.Expression})
		}
		if query.TimeRange != nil && key(*m.TimeAttributeRef) != selectorKey(query.TimeRange.Selector) {
			return ErrInvalidArgument
		}
		delete(visiting, ref.AssetID)
		visited[ref.AssetID] = true
		return nil
	}
	for _, s := range query.Measures {
		if e := addMetric(refFor(s), 0); e != nil {
			return refuse(RefusalIncompatibleGrain, "指标依赖缺失、循环、粒度或分子分母统计范围不一致。")
		}
	}
	filters := []Filter{}
	for _, f := range query.Filters {
		if assets[f.Selector.AssetID.String()].AssetType == semantic.BusinessTerm {
			if f.Operator != "eq" || string(f.Value) != "true" {
				return refuse(RefusalInvalidFilter, "业务口径只支持明确的适用判定。")
			}
			r := refFor(f.Selector)
			fixedTerms[r.AssetID] = r
		} else {
			filters = append(filters, f)
		}
	}
	termIDs := []string{}
	for id := range fixedTerms {
		termIDs = append(termIDs, id)
	}
	sort.Strings(termIDs)
	for _, id := range termIDs {
		ref := fixedTerms[id]
		compatible := false
		for _, r := range spec.CompatibleTermRefs {
			if r.AssetID == id && r.RevisionID == ref.RevisionID {
				compatible = true
			}
		}
		if !compatible {
			return refuse(RefusalInvalidFilter, "模型不支持所需业务口径。")
		}
		_, term, e := resolveRef(ref, semantic.BusinessTerm)
		if e != nil || term.Capability != "predicate" {
			return refuse(RefusalInvalidFilter, "口径仅有解释定义或缺少已发布判定。")
		}
		if term.SubjectRef == nil {
			return refuse(RefusalInvalidFilter, "口径缺少适用对象。")
		}
		if _, _, err := resolveRef(*term.SubjectRef, semantic.BusinessObject); err != nil {
			return refuse(RefusalInvalidFilter, "口径对象版本不一致。")
		}
		if err := term.ValidateReferences(func(ref semantic.KnowledgeReference) (semantic.AssetType, semantic.KnowledgeSpec, error) {
			asset, target, err := resolveRef(ref, "")
			return asset.AssetType, target, err
		}); err != nil {
			return refuse(RefusalInvalidFilter, "口径属性、适用对象或参数类型不一致。")
		}
		var walk func(*semantic.KnowledgeExpression) error
		walk = func(e *semantic.KnowledgeExpression) error {
			if e == nil {
				return nil
			}
			if e.Ref != nil {
				_, err := bindAttribute(*e.Ref)
				if err != nil {
					return err
				}
			}
			if e.Op == "parameter" && (e.Parameter != "period_start" || query.TimeRange == nil) {
				return ErrInvalidArgument
			}
			if err := walk(e.Left); err != nil {
				return err
			}
			return walk(e.Right)
		}
		if e := walk(term.Predicate); e != nil {
			return refuse(RefusalInvalidFilter, "口径属性或查询参数未满足。")
		}
		ir.Predicates = append(ir.Predicates, term.Predicate)
	}
	keys := []string{}
	for k := range bindings {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		ir.Bindings = append(ir.Bindings, bindings[k])
	}
	keys = nil
	for k := range relations {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		ir.Relations = append(ir.Relations, relations[k])
	}
	for _, id := range spec.JoinContractIDs {
		for _, join := range snapshot.Execution.Joins {
			if join.ID == id && relations[join.LeftDatasetID].DatasetID != "" && relations[join.RightDatasetID].DatasetID != "" {
				ir.Joins = append(ir.Joins, join)
				objects[id] = ResolvedObject{ObjectType: "join_contract", ObjectID: id, Version: join.Version}
			}
		}
	}
	plan := ResolvedSemanticPlan{ID: planID, QueryID: queryID, ReleaseID: snapshot.ReleaseID, ResolverVersion: ResolverVersion, Intent: query.Intent, Model: &modelPin, Execution: ir, Measures: query.Measures, Grouping: query.Dimensions, Filters: filters, TimeRange: query.TimeRange, Order: query.Order, Limit: query.Limit, ExecutionStatus: "requires_execution_validation", CreatedAt: now.UTC(), Assets: []ResolvedAsset{}, Objects: []ResolvedObject{}}
	keys = nil
	for k := range used {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		a := used[k]
		plan.Assets = append(plan.Assets, ResolvedAsset{AssetID: a.AssetID, RevisionID: a.RevisionID, Address: a.Address, AssetType: a.AssetType})
	}
	for _, o := range objects {
		plan.Objects = append(plan.Objects, o)
	}
	plan.Objects = sortedResolvedObjects(plan.Objects)
	sort.Slice(ir.Calculations, func(i, j int) bool { return ir.Calculations[i].AssetID < ir.Calculations[j].AssetID })
	sort.Slice(ir.BindingPins, func(i, j int) bool { return ir.BindingPins[i].BindingID < ir.BindingPins[j].BindingID })
	plan.PlanDigest = PlanDigest(plan)
	return plan, nil, nil
}
