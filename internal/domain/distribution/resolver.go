package distribution

import (
	"encoding/json"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/internal/domain/semantic"
	"github.com/iiwish/semlia/pkg/identity"
)

type Match struct {
	Asset      ReleasedAsset
	Candidates []ReleasedAsset
	Score      float64
}

func MatchSelector(selector Selector, expected string, assets []ReleasedAsset) (Match, *Refusal) {
	eligible := make([]ReleasedAsset, 0, len(assets))
	for _, asset := range assets {
		if allowedType(asset.AssetType, expected) {
			eligible = append(eligible, asset)
		}
	}
	if selector.AssetID != nil {
		for _, asset := range eligible {
			if asset.AssetID == *selector.AssetID {
				return Match{Asset: asset, Score: 1}, nil
			}
		}
		return Match{}, &Refusal{Code: RefusalNoMatch, Clarification: "请选择一个已发布且可读取的语义资产。"}
	}
	if address := normalize(selector.Address); address != "" {
		for _, asset := range eligible {
			if normalize(asset.Address) == address {
				return Match{Asset: asset, Score: 1}, nil
			}
		}
		return Match{}, &Refusal{Code: RefusalNoMatch, Clarification: "请确认语义地址或选择一个已发布资产。"}
	}
	term := normalize(selector.Search)
	type scored struct {
		asset ReleasedAsset
		score float64
	}
	scores := make([]scored, 0, len(eligible))
	for _, asset := range eligible {
		score := lexicalScore(term, asset)
		if score >= .55 {
			scores = append(scores, scored{asset: asset, score: score})
		}
	}
	if len(scores) == 0 {
		return Match{}, &Refusal{Code: RefusalNoMatch, Clarification: "请使用更准确的名称、别名或语义地址。"}
	}
	sort.Slice(scores, func(i, j int) bool {
		if scores[i].score != scores[j].score {
			return scores[i].score > scores[j].score
		}
		return scores[i].asset.AssetID.String() < scores[j].asset.AssetID.String()
	})
	if len(scores) > 1 && scores[0].score-scores[1].score < .12 {
		candidates := []ReleasedAsset{scores[0].asset, scores[1].asset}
		for index := 2; index < len(scores) && scores[0].score-scores[index].score < .12 && index < 5; index++ {
			candidates = append(candidates, scores[index].asset)
		}
		return Match{Candidates: candidates}, &Refusal{Code: RefusalAmbiguousMatch, Clarification: "匹配到多个接近的已发布资产，请指定语义地址。"}
	}
	return Match{Asset: scores[0].asset, Score: scores[0].score}, nil
}

func BuildPlan(query SemanticQueryInput, snapshot ReleaseSnapshot, queryID identity.SemanticQueryID,
	planID identity.ResolvedSemanticPlanID, selected []ReleasedAsset, now time.Time,
) (ResolvedSemanticPlan, *Refusal, error) {
	resolvedAssets := make([]ResolvedAsset, 0, len(selected))
	assetSeen := make(map[string]bool, len(selected))
	for _, asset := range selected {
		if assetSeen[asset.AssetID.String()] {
			continue
		}
		assetSeen[asset.AssetID.String()] = true
		resolvedAssets = append(resolvedAssets, ResolvedAsset{AssetID: asset.AssetID, RevisionID: asset.RevisionID,
			Address: asset.Address, AssetType: asset.AssetType})
	}
	sort.Slice(resolvedAssets, func(i, j int) bool { return resolvedAssets[i].AssetID.String() < resolvedAssets[j].AssetID.String() })

	objects := make([]ResolvedObject, 0)
	datasets := make(map[string]identity.PhysicalDatasetID)
	if query.Intent != IntentDescribe {
		for _, asset := range resolvedAssets {
			binding, ok := bindingFor(snapshot.Bindings, asset.AssetID)
			if !ok || binding.Retired {
				return ResolvedSemanticPlan{}, &Refusal{Code: RefusalMissingPhysicalBinding,
					Clarification: "该已发布资产缺少可用的物理绑定。"}, nil
			}
			datasets[binding.DatasetID.String()] = binding.DatasetID
			objects = append(objects, ResolvedObject{ObjectType: string(governance.TargetPhysicalBinding),
				ObjectID: binding.ID.String(), Version: binding.Version})
			if grain, found := grainFor(snapshot.Grains, asset.AssetID); found {
				objects = append(objects, ResolvedObject{ObjectType: string(governance.TargetModelGrain),
					ObjectID: grain.ID.String(), Version: grain.Version})
			}
			if key, found := entityKeyFor(snapshot.Keys, asset.AssetID); found {
				objects = append(objects, ResolvedObject{ObjectType: string(governance.TargetEntityKey),
					ObjectID: key.ID.String(), Version: key.Version})
			}
		}
	}
	if len(datasets) > 1 {
		joins, ok := joinTree(datasets, snapshot.Joins)
		if !ok {
			return ResolvedSemanticPlan{}, &Refusal{Code: RefusalMissingJoinPath,
				Clarification: "所选资产之间没有已发布的 JoinContract 路径。"}, nil
		}
		for _, join := range joins {
			if join.Cardinality == governance.CardinalityManyToMany {
				return ResolvedSemanticPlan{}, &Refusal{Code: RefusalIncompatibleGrain,
					Clarification: "所选资产会形成多对多粒度，请缩小查询范围。"}, nil
			}
			objects = append(objects, ResolvedObject{ObjectType: string(governance.TargetJoinContract),
				ObjectID: join.ID.String(), Version: join.Version})
		}
	}
	objects = sortedResolvedObjects(objects)
	var execution *ExecutionProvenance
	if snapshot.Execution != nil && query.Intent != IntentDescribe {
		execution = &ExecutionProvenance{CompilerVersion: snapshot.Execution.CompilerVersion}
		for _, r := range snapshot.Execution.Relations {
			if !datasets[r.DatasetID].IsZero() {
				execution.Relations = append(execution.Relations, r)
			} else {
				for _, id := range datasets {
					if id.UUID() == r.DatasetID {
						execution.Relations = append(execution.Relations, r)
						break
					}
				}
			}
		}
		for _, a := range resolvedAssets {
			b, ok := bindingFor(snapshot.Bindings, a.AssetID)
			if !ok || b.FieldID == nil {
				execution = nil
				break
			}
			pinned := false
			for _, pin := range snapshot.Execution.BindingPins {
				if pin.BindingID == b.ID.UUID() && pin.Version == b.Version && pin.DatasetID == b.DatasetID.UUID() {
					execution.BindingPins = append(execution.BindingPins, pin)
					pinned = true
					break
				}
			}
			if !pinned {
				execution = nil
				break
			}
			binding := ExecutionBinding{AssetID: a.AssetID.String(), DatasetID: b.DatasetID.UUID(), FieldID: b.FieldID.UUID(), Expression: b.Transform}
			for _, asset := range selected {
				if asset.AssetID == a.AssetID {
					var content struct {
						Execution struct {
							Aggregation string `json:"aggregation"`
							Expression  string `json:"expression"`
						} `json:"execution"`
					}
					_ = json.Unmarshal(asset.Content, &content)
					binding.Aggregation = content.Execution.Aggregation
					if content.Execution.Expression != "" {
						binding.Expression = content.Execution.Expression
					}
					break
				}
			}
			execution.Bindings = append(execution.Bindings, binding)
		}
		if execution != nil {
			for _, j := range snapshot.Execution.Joins {
				for _, o := range objects {
					if o.ObjectType == "join_contract" && o.ObjectID == j.ID && o.Version == j.Version {
						execution.Joins = append(execution.Joins, j)
					}
				}
			}
			for _, g := range snapshot.Grains {
				if assetSeen[g.AssetID.String()] {
					execution.Grains = append(execution.Grains, g)
				}
			}
			// Freeze the exact selector result; execution never repeats fuzzy matching.
			canonical := func(s Selector) Selector {
				m, r := MatchSelector(s, "any", selected)
				if r != nil {
					return s
				}
				id := m.Asset.AssetID
				return Selector{AssetID: &id}
			}
			query.Measures = append([]Selector(nil), query.Measures...)
			for i := range query.Measures {
				query.Measures[i] = canonical(query.Measures[i])
			}
			query.Dimensions = append([]Selector(nil), query.Dimensions...)
			for i := range query.Dimensions {
				query.Dimensions[i] = canonical(query.Dimensions[i])
			}
			query.Filters = append([]Filter(nil), query.Filters...)
			for i := range query.Filters {
				query.Filters[i].Selector = canonical(query.Filters[i].Selector)
			}
			query.Order = append([]Order(nil), query.Order...)
			for i := range query.Order {
				query.Order[i].Selector = canonical(query.Order[i].Selector)
			}
			if query.TimeRange != nil {
				tr := *query.TimeRange
				tr.Selector = canonical(tr.Selector)
				query.TimeRange = &tr
			}
		}
	}
	plan := ResolvedSemanticPlan{ID: planID, QueryID: queryID, ReleaseID: snapshot.ReleaseID,
		ResolverVersion: ResolverVersion, Intent: query.Intent, Assets: resolvedAssets, Objects: objects,
		Measures: query.Measures, Filters: query.Filters, Grouping: query.Dimensions, TimeRange: query.TimeRange,
		Order: query.Order, Limit: query.Limit,
		ExecutionStatus: "not_configured", CreatedAt: now.UTC()}
	plan.Execution = execution
	if execution != nil {
		plan.ExecutionStatus = "requires_execution_validation"
	}
	digestShape := struct {
		ReleaseID       string           `json:"releaseId"`
		ResolverVersion string           `json:"resolverVersion"`
		Intent          Intent           `json:"intent"`
		Assets          []ResolvedAsset  `json:"assets"`
		Objects         []ResolvedObject `json:"objects"`
		Measures        []Selector       `json:"measures,omitempty"`
		Filters         []Filter         `json:"filters,omitempty"`
		Grouping        []Selector       `json:"grouping,omitempty"`
		TimeRange       *TimeRange       `json:"timeRange,omitempty"`
		Order           []Order          `json:"order,omitempty"`
		Limit           int              `json:"limit,omitempty"`
		ExecutionStatus string           `json:"executionStatus"`
	}{snapshot.ReleaseID.String(), ResolverVersion, query.Intent, resolvedAssets, objects, query.Measures,
		query.Filters, query.Dimensions, query.TimeRange, query.Order, query.Limit, "not_configured"}
	_, digest, err := stableDigest(digestShape)
	if err != nil {
		return ResolvedSemanticPlan{}, nil, invalidPlan(err.Error())
	}
	plan.PlanDigest = digest
	if execution != nil {
		plan.PlanDigest = PlanDigest(plan)
	}
	return plan, nil, nil
}

// PlanDigest excludes generated identities and time while including all
// release-pinned physical facts and typed execution semantics.
func PlanDigest(plan ResolvedSemanticPlan) string {
	type digestPlan ResolvedSemanticPlan
	shape := struct {
		*digestPlan
		ID         *identity.ResolvedSemanticPlanID `json:"id,omitempty"`
		QueryID    *identity.SemanticQueryID        `json:"queryId,omitempty"`
		CreatedAt  *time.Time                       `json:"createdAt,omitempty"`
		PlanDigest *string                          `json:"planDigest,omitempty"`
	}{digestPlan: (*digestPlan)(&plan)}
	_, digest, _ := stableDigest(shape)
	return digest
}

func allowedType(assetType semantic.AssetType, expected string) bool {
	if expected == "measure" {
		return assetType == semantic.Measure || assetType == semantic.Metric
	}
	if expected == "dimension" {
		return assetType == semantic.Dimension || assetType == semantic.Entity || assetType == semantic.Concept
	}
	return true
}

func normalize(value string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(value))), " ")
}

func lexicalScore(term string, asset ReleasedAsset) float64 {
	if term == "" {
		return 0
	}
	values := append([]string{asset.Address, asset.Name}, asset.Aliases...)
	best := 0.0
	for _, value := range values {
		candidate := normalize(value)
		score := trigramDice(term, candidate)
		if candidate == term {
			score = 1
		} else if strings.HasPrefix(candidate, term) || strings.HasPrefix(term, candidate) {
			score = math.Max(score, .88)
		} else if strings.Contains(candidate, term) || strings.Contains(term, candidate) {
			score = math.Max(score, .76)
		}
		if score > best {
			best = score
		}
	}
	return best
}

func trigramDice(left, right string) float64 {
	leftSet, rightSet := trigrams(left), trigrams(right)
	if len(leftSet) == 0 || len(rightSet) == 0 {
		return 0
	}
	intersection := 0
	for value := range leftSet {
		if rightSet[value] {
			intersection++
		}
	}
	return float64(2*intersection) / float64(len(leftSet)+len(rightSet))
}

func trigrams(value string) map[string]bool {
	value = "  " + normalize(value) + "  "
	result := make(map[string]bool)
	for index := 0; index+3 <= len(value); index++ {
		result[value[index:index+3]] = true
	}
	return result
}

func bindingFor(bindings []ReleasedPhysicalBinding, asset identity.AssetID) (ReleasedPhysicalBinding, bool) {
	for _, binding := range bindings {
		if binding.AssetID == asset {
			return binding, true
		}
	}
	return ReleasedPhysicalBinding{}, false
}

func grainFor(grains []ReleasedModelGrain, asset identity.AssetID) (ReleasedModelGrain, bool) {
	for _, grain := range grains {
		if grain.AssetID == asset {
			return grain, true
		}
	}
	return ReleasedModelGrain{}, false
}

func entityKeyFor(keys []ReleasedEntityKey, asset identity.AssetID) (ReleasedEntityKey, bool) {
	for _, key := range keys {
		if key.AssetID == asset {
			return key, true
		}
	}
	return ReleasedEntityKey{}, false
}

func joinTree(targets map[string]identity.PhysicalDatasetID, joins []ReleasedJoinContract) ([]ReleasedJoinContract, bool) {
	ordered := append([]ReleasedJoinContract(nil), joins...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].ID.String() < ordered[j].ID.String() })
	keys := make([]string, 0, len(targets))
	for key := range targets {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	connected := map[string]bool{keys[0]: true}
	selected := make([]ReleasedJoinContract, 0, len(keys)-1)
	for len(connected) < len(keys) {
		advanced := false
		for _, join := range ordered {
			left, right := join.LeftDatasetID.String(), join.RightDatasetID.String()
			if !targets[left].IsZero() && !targets[right].IsZero() && connected[left] != connected[right] {
				connected[left], connected[right] = true, true
				selected = append(selected, join)
				advanced = true
				break
			}
		}
		if !advanced {
			return nil, false
		}
	}
	return selected, true
}
