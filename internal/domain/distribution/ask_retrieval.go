package distribution

import (
	"encoding/json"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/iiwish/semlia/internal/domain/semantic"
)

// AskCard is the machine-readable wiki page of one released asset: what it means, how people call it, where its
// boundary lies and what it references. The same card feeds the Ask prompt and retrieval.
type AskCard struct {
	AssetID                 string                        `json:"assetId"`
	RevisionID              string                        `json:"revisionId"`
	Address                 string                        `json:"address"`
	Name                    string                        `json:"name"`
	Type                    semantic.AssetType            `json:"type"`
	Definition              string                        `json:"definition,omitempty"`
	Scope                   string                        `json:"scope,omitempty"`
	Aliases                 []string                      `json:"aliases,omitempty"`
	Includes                []string                      `json:"includes,omitempty"`
	Excludes                []string                      `json:"excludes,omitempty"`
	ConfusableWith          []AskConfusion                `json:"confusableWith,omitempty"`
	Unit                    string                        `json:"unit,omitempty"`
	Capability              string                        `json:"capability,omitempty"`
	Members                 []AskMember                   `json:"members"`
	MetricRefs              []semantic.KnowledgeReference `json:"metricRefs,omitempty"`
	PublicAttributeRefs     []semantic.KnowledgeReference `json:"publicAttributeRefs,omitempty"`
	CompatibleTermRefs      []semantic.KnowledgeReference `json:"compatibleTermRefs,omitempty"`
	DefaultTimeAttributeRef *semantic.KnowledgeReference  `json:"defaultTimeAttributeRef,omitempty"`
	spec                    semantic.KnowledgeSpec        `json:"-"`
	confusions              []semantic.Confusion          `json:"-"`
	sampleQuestions         []semantic.SampleQuestion     `json:"-"`
}

type AskConfusion struct {
	AssetID     string `json:"assetId"`
	Name        string `json:"name"`
	Distinction string `json:"distinction"`
}

type AskMember struct {
	ID        string                `json:"id"`
	Name      string                `json:"name,omitempty"`
	ValueType string                `json:"valueType,omitempty"`
	Aliases   []string              `json:"aliases,omitempty"`
	Values    []semantic.ValueAlias `json:"valueAliases,omitempty"`
}

const askCardMemberLimit = 64

// CompileAskCards builds cards for the assets the caller may read. Content that does not decode is skipped, as it was
// before cards existed. Confusable neighbours outside the given set are dropped so no unreadable name leaks.
func CompileAskCards(assets []ReleasedAsset) []AskCard {
	cards := []AskCard{}
	for _, a := range assets {
		var content struct {
			DisplayName string          `json:"displayName"`
			Name        string          `json:"name"`
			Title       string          `json:"title"`
			Definition  string          `json:"definition"`
			Scope       string          `json:"scope"`
			Spec        json.RawMessage `json:"spec"`
		}
		if json.Unmarshal(a.Content, &content) != nil {
			continue
		}
		spec, err := semantic.ParseKnowledgeSpec(a.AssetType, content.Spec, false)
		if err != nil {
			continue
		}
		vocabulary, err := semantic.ParseVocabulary(a.Content)
		if err != nil {
			vocabulary = semantic.Vocabulary{}
		}
		card := AskCard{
			AssetID: a.AssetID.String(), RevisionID: a.RevisionID.String(), Address: a.Address,
			Name: firstText(content.DisplayName, content.Name, content.Title, a.Name), Type: a.AssetType,
			Definition: content.Definition, Scope: content.Scope, Aliases: vocabulary.Aliases,
			Includes: vocabulary.Includes, Excludes: vocabulary.Excludes, Unit: spec.Unit, Capability: spec.Capability,
			Members: []AskMember{}, MetricRefs: spec.MetricRefs, PublicAttributeRefs: spec.PublicAttributeRefs,
			CompatibleTermRefs: spec.CompatibleTermRefs, DefaultTimeAttributeRef: spec.DefaultTimeAttributeRef,
			spec: spec, confusions: vocabulary.ConfusableWith, sampleQuestions: vocabulary.SampleQuestions,
		}
		for _, m := range spec.Members {
			if len(card.Members) >= askCardMemberLimit {
				break
			}
			card.Members = append(card.Members, AskMember{ID: m.ID, Name: m.Name, ValueType: m.ValueType, Aliases: m.Aliases, Values: m.ValueAliases})
		}
		cards = append(cards, card)
	}
	names := map[string]string{}
	for _, card := range cards {
		names[card.AssetID] = card.Name
	}
	for i := range cards {
		for _, confusion := range cards[i].confusions {
			if name, ok := names[confusion.Ref.AssetID]; ok {
				cards[i].ConfusableWith = append(cards[i].ConfusableWith, AskConfusion{AssetID: confusion.Ref.AssetID, Name: name, Distinction: confusion.Distinction})
			}
		}
	}
	return cards
}

// SampleQuestions lists the card's typical questions for retrieval checks.
func (card AskCard) SampleQuestions() []semantic.SampleQuestion { return card.sampleQuestions }

// AskHit says what one matched phrase refers to and through which wording.
type AskHit struct {
	AssetID    string             `json:"assetId"`
	Name       string             `json:"name"`
	Type       semantic.AssetType `json:"type"`
	MemberID   string             `json:"memberId,omitempty"`
	MemberName string             `json:"memberName,omitempty"`
	Value      string             `json:"value,omitempty"`
	// Via is name, alias, member, member_alias, value or value_alias.
	Via string `json:"via"`
}

// AskMatch is one phrase of the question bound to governed wording. ResolvedAssetIDs lists the asset chosen for each
// type whose assets share the phrase when the question itself settles which one is meant.
type AskMatch struct {
	Text             string   `json:"text"`
	Hits             []AskHit `json:"hits"`
	ResolvedAssetIDs []string `json:"resolvedAssetIds,omitempty"`
	// excluded holds contested candidates the question settled against; ambiguous holds unsettled ones.
	excluded, ambiguous map[string]bool
}

// usable reports whether an asset-level hit still stands after disambiguation.
func (match AskMatch) usable(hit AskHit) bool {
	return hit.MemberID != "" || (!match.excluded[hit.AssetID] && !match.ambiguous[hit.AssetID])
}

// AskAmbiguity is a phrase that names several assets of one type with nothing in the question to tell them apart.
// Distinctions are what the candidates' authors published about each other, each stated once.
type AskAmbiguity struct {
	Text         string              `json:"text"`
	Type         semantic.AssetType  `json:"type"`
	Candidates   []AskModelCandidate `json:"candidates"`
	Distinctions []string            `json:"distinctions"`
}

type AskModelCandidate struct {
	AssetID string `json:"assetId"`
	Name    string `json:"name"`
}

// AskSelection is the deterministic retrieval result for one question against one authorized release view.
type AskSelection struct {
	Cards          []AskCard           `json:"assets"`
	Matches        []AskMatch          `json:"questionMatches"`
	Ambiguities    []AskAmbiguity      `json:"ambiguities"`
	CoveringModels []AskModelCandidate `json:"coveringModels"`
	OmittedAssets  int                 `json:"omittedAssets"`
	// Recalled holds every asset retrieval considered relevant, in rank order, before the budget applied.
	Recalled []string `json:"-"`
}

type AskBudget struct {
	MaxCards int
	MaxBytes int
}

// DefaultAskBudget bounds what one question sends to the model. A catalog that fits is sent whole.
var DefaultAskBudget = AskBudget{MaxCards: 128, MaxBytes: 64 * 1024}

type lexiconEntry struct {
	hit  AskHit
	term string
}

// SelectAskKnowledge binds the question to governed wording, expands the knowledge graph around what it names and
// fills the budget in rank order. Equal inputs always produce equal output, so the knowledge digest stays stable
// across the model call and replays.
func SelectAskKnowledge(question string, cards []AskCard, budget AskBudget) AskSelection {
	text := semantic.NormalizeTerm(question)
	byID := map[string]int{}
	for i, card := range cards {
		byID[card.AssetID] = i
	}
	lexicon := map[string][]lexiconEntry{}
	add := func(term string, hit AskHit) {
		key := semantic.NormalizeTerm(term)
		if utf8.RuneCountInString(key) < 2 {
			return
		}
		lexicon[key] = append(lexicon[key], lexiconEntry{hit: hit, term: key})
	}
	for _, card := range cards {
		base := AskHit{AssetID: card.AssetID, Name: card.Name, Type: card.Type}
		hit := base
		hit.Via = "name"
		add(card.Name, hit)
		for _, alias := range card.Aliases {
			hit := base
			hit.Via = "alias"
			add(alias, hit)
		}
		for _, member := range card.Members {
			memberHit := base
			memberHit.MemberID, memberHit.MemberName = member.ID, member.Name
			hit := memberHit
			hit.Via = "member"
			add(member.Name, hit)
			for _, alias := range member.Aliases {
				hit := memberHit
				hit.Via = "member_alias"
				add(alias, hit)
			}
			for _, value := range member.Values {
				hit := memberHit
				hit.Value, hit.Via = value.Value, "value"
				add(value.Value, hit)
				for _, alias := range value.Aliases {
					hit := memberHit
					hit.Value, hit.Via = value.Value, "value_alias"
					add(alias, hit)
				}
			}
		}
	}
	matches := matchLexicon(text, lexicon)
	selection := AskSelection{Matches: []AskMatch{}, Ambiguities: []AskAmbiguity{}, CoveringModels: []AskModelCandidate{}}
	resolveAmbiguities(matches, cards, byID, &selection)

	score := map[string]float64{}
	bump := func(id string, value float64) {
		if value > score[id] {
			score[id] = value
		}
	}
	for _, match := range selection.Matches {
		for _, hit := range match.Hits {
			if match.excluded[hit.AssetID] && hit.MemberID == "" {
				continue
			}
			switch hit.Via {
			case "name":
				bump(hit.AssetID, 1)
			case "alias":
				bump(hit.AssetID, .95)
			default:
				bump(hit.AssetID, .9)
			}
		}
	}
	for _, ambiguity := range selection.Ambiguities {
		for _, candidate := range ambiguity.Candidates {
			bump(candidate.AssetID, .8)
		}
	}
	for term, entries := range lexicon {
		if strings.Contains(text, term) {
			continue
		}
		if coverage := bigramCoverage(term, text); coverage >= .6 {
			for _, entry := range entries {
				bump(entry.hit.AssetID, .7*coverage)
			}
		}
	}
	seeds := make([]string, 0, len(score))
	for id := range score {
		seeds = append(seeds, id)
	}
	sort.Slice(seeds, func(i, j int) bool {
		if score[seeds[i]] != score[seeds[j]] {
			return score[seeds[i]] > score[seeds[j]]
		}
		return seeds[i] < seeds[j]
	})
	selection.CoveringModels = coveringModels(selection.Matches, cards)
	recalled := expandAskGraph(seeds, cards, byID)
	selection.Recalled = recalled
	order := append([]string{}, recalled...)
	seen := map[string]bool{}
	for _, id := range order {
		seen[id] = true
	}
	rest := []string{}
	for _, card := range cards {
		if !seen[card.AssetID] {
			rest = append(rest, card.AssetID)
		}
	}
	sort.Strings(rest)
	order = append(order, rest...)
	remaining := budget.MaxBytes
	for _, id := range order {
		if len(selection.Cards) >= budget.MaxCards {
			break
		}
		card := cards[byID[id]]
		encoded, err := json.Marshal(card)
		if err != nil || len(encoded) > remaining {
			continue
		}
		remaining -= len(encoded)
		selection.Cards = append(selection.Cards, card)
	}
	selection.OmittedAssets = len(cards) - len(selection.Cards)
	return selection
}

type lexiconSpan struct {
	start, end int
	term       string
}

// matchLexicon finds governed phrases in the normalized question, longest first, without overlaps.
func matchLexicon(text string, lexicon map[string][]lexiconEntry) []AskMatch {
	spans := []lexiconSpan{}
	for term := range lexicon {
		for offset := 0; offset < len(text); {
			index := strings.Index(text[offset:], term)
			if index < 0 {
				break
			}
			start := offset + index
			end := start + len(term)
			if bounded(text, start, end) {
				spans = append(spans, lexiconSpan{start, end, term})
			}
			offset = start + 1
			for offset < len(text) && !utf8.RuneStart(text[offset]) {
				offset++
			}
		}
	}
	sort.Slice(spans, func(i, j int) bool {
		a, b := spans[i], spans[j]
		if a.end-a.start != b.end-b.start {
			return a.end-a.start > b.end-b.start
		}
		if a.start != b.start {
			return a.start < b.start
		}
		return a.term < b.term
	})
	accepted := []lexiconSpan{}
	for _, span := range spans {
		overlaps := false
		for _, kept := range accepted {
			if span.start < kept.end && kept.start < span.end {
				overlaps = true
				break
			}
		}
		if !overlaps {
			accepted = append(accepted, span)
		}
	}
	sort.Slice(accepted, func(i, j int) bool { return accepted[i].start < accepted[j].start })
	matches := []AskMatch{}
	for _, span := range accepted {
		hits := []AskHit{}
		for _, entry := range lexicon[span.term] {
			hits = append(hits, entry.hit)
		}
		sort.Slice(hits, func(i, j int) bool {
			a, b := hits[i], hits[j]
			return a.AssetID+"\x00"+a.MemberID+"\x00"+a.Value+"\x00"+a.Via < b.AssetID+"\x00"+b.MemberID+"\x00"+b.Value+"\x00"+b.Via
		})
		matches = append(matches, AskMatch{Text: span.term, Hits: hits})
	}
	return matches
}

// bounded rejects Latin or digit terms that are only part of a longer word, such as "id" inside "paid".
func bounded(text string, start, end int) bool {
	word := func(r rune) bool { return r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)) }
	first, _ := utf8.DecodeRuneInString(text[start:end])
	last, _ := utf8.DecodeLastRuneInString(text[start:end])
	if start > 0 && word(first) {
		if before, _ := utf8.DecodeLastRuneInString(text[:start]); word(before) {
			return false
		}
	}
	if end < len(text) && word(last) {
		if after, _ := utf8.DecodeRuneInString(text[end:]); word(after) {
			return false
		}
	}
	return true
}

// resolveAmbiguities settles phrases that name several assets of one type. A display name beats an alias, and an
// asset the question names unambiguously elsewhere wins; anything left becomes an ambiguity the caller must clarify.
func resolveAmbiguities(matches []AskMatch, cards []AskCard, byID map[string]int, selection *AskSelection) {
	certain := map[string]bool{}
	contested := func(match AskMatch) map[semantic.AssetType][]AskHit {
		groups := map[semantic.AssetType][]AskHit{}
		for _, hit := range match.Hits {
			if hit.MemberID == "" {
				groups[hit.Type] = append(groups[hit.Type], hit)
			}
		}
		return groups
	}
	for _, match := range matches {
		for _, group := range contested(match) {
			if len(group) == 1 {
				certain[group[0].AssetID] = true
			}
		}
	}
	for _, match := range matches {
		match.excluded, match.ambiguous = map[string]bool{}, map[string]bool{}
		groups := contested(match)
		kinds := make([]string, 0, len(groups))
		for kind := range groups {
			kinds = append(kinds, string(kind))
		}
		sort.Strings(kinds)
		for _, kind := range kinds {
			group := groups[semantic.AssetType(kind)]
			if len(group) < 2 {
				continue
			}
			named, known := []string{}, []string{}
			for _, hit := range group {
				if hit.Via == "name" {
					named = append(named, hit.AssetID)
				}
				if certain[hit.AssetID] {
					known = append(known, hit.AssetID)
				}
			}
			chosen := ""
			switch {
			case len(named) == 1:
				chosen = named[0]
			case len(known) == 1:
				chosen = known[0]
			}
			if chosen != "" {
				match.ResolvedAssetIDs = append(match.ResolvedAssetIDs, chosen)
				for _, hit := range group {
					if hit.AssetID != chosen {
						match.excluded[hit.AssetID] = true
					}
				}
				continue
			}
			ambiguity := AskAmbiguity{Text: match.Text, Type: semantic.AssetType(kind), Distinctions: distinctionsAmong(group, cards, byID)}
			for _, hit := range group {
				match.ambiguous[hit.AssetID] = true
				ambiguity.Candidates = append(ambiguity.Candidates, AskModelCandidate{AssetID: hit.AssetID, Name: hit.Name})
			}
			selection.Ambiguities = append(selection.Ambiguities, ambiguity)
		}
		selection.Matches = append(selection.Matches, match)
	}
}

// distinctionsAmong collects what each candidate says about the others, in candidate order, without repeats.
func distinctionsAmong(group []AskHit, cards []AskCard, byID map[string]int) []string {
	members := map[string]bool{}
	for _, hit := range group {
		members[hit.AssetID] = true
	}
	seen := map[string]bool{}
	distinctions := []string{}
	for _, hit := range group {
		for _, confusion := range cards[byID[hit.AssetID]].ConfusableWith {
			if members[confusion.AssetID] && !seen[confusion.Distinction] {
				seen[confusion.Distinction] = true
				distinctions = append(distinctions, confusion.Distinction)
			}
		}
	}
	return distinctions
}

// coveringModels lists analysis models that contain every metric the question names without ambiguity.
func coveringModels(matches []AskMatch, cards []AskCard) []AskModelCandidate {
	metrics := map[string]bool{}
	for _, match := range matches {
		for _, hit := range match.Hits {
			if hit.Type == semantic.Metric && hit.MemberID == "" && match.usable(hit) {
				metrics[hit.AssetID] = true
			}
		}
	}
	models := []AskModelCandidate{}
	if len(metrics) == 0 {
		return models
	}
	for _, card := range cards {
		if card.Type != semantic.AnalysisModel {
			continue
		}
		contained := map[string]bool{}
		for _, ref := range card.MetricRefs {
			contained[ref.AssetID] = true
		}
		covers := true
		for id := range metrics {
			covers = covers && contained[id]
		}
		if covers {
			models = append(models, AskModelCandidate{AssetID: card.AssetID, Name: card.Name})
		}
	}
	sort.Slice(models, func(i, j int) bool { return models[i].AssetID < models[j].AssetID })
	return models
}

// expandAskGraph returns seeds followed by everything they depend on and the analysis models that use them, so the
// model always sees a self-consistent subgraph: metrics with their inputs, terms with their subjects, models with
// their members and bindings.
func expandAskGraph(seeds []string, cards []AskCard, byID map[string]int) []string {
	usedBy := map[string][]string{}
	for _, card := range cards {
		if card.Type != semantic.AnalysisModel {
			continue
		}
		for _, ref := range card.spec.References() {
			usedBy[ref.AssetID] = append(usedBy[ref.AssetID], card.AssetID)
		}
	}
	for id := range usedBy {
		sort.Strings(usedBy[id])
	}
	order := []string{}
	seen := map[string]bool{}
	queue := []string{}
	visit := func(id string) {
		if _, ok := byID[id]; ok && !seen[id] {
			seen[id] = true
			order = append(order, id)
			queue = append(queue, id)
		}
	}
	for _, id := range seeds {
		visit(id)
	}
	for _, id := range seeds {
		for _, model := range usedBy[id] {
			visit(model)
		}
	}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		refs := cards[byID[id]].spec.References()
		sort.SliceStable(refs, func(i, j int) bool { return refs[i].AssetID < refs[j].AssetID })
		for _, ref := range refs {
			visit(ref.AssetID)
		}
	}
	return order
}

func bigramCoverage(term, text string) float64 {
	grams := bigrams(term)
	if len(grams) == 0 {
		return 0
	}
	present := bigrams(text)
	found := 0
	for gram := range grams {
		if present[gram] {
			found++
		}
	}
	return float64(found) / float64(len(grams))
}

func bigrams(value string) map[string]bool {
	runes := []rune(value)
	grams := map[string]bool{}
	for i := 0; i+1 < len(runes); i++ {
		if unicode.IsSpace(runes[i]) || unicode.IsSpace(runes[i+1]) {
			continue
		}
		grams[string(runes[i:i+2])] = true
	}
	return grams
}

func firstText(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

// SampleRecallFailure is a typical question that retrieval would not answer with its own asset: the asset or an
// expected reference is not recalled, or the question would be sent back for clarification.
type SampleRecallFailure struct {
	AssetID   string
	Question  string
	Missing   []string
	Ambiguous bool
}

// CheckSampleQuestions replays each owner's sample questions through the same deterministic retrieval Ask uses.
func CheckSampleQuestions(cards []AskCard, owners map[string]bool, budget AskBudget) []SampleRecallFailure {
	failures := []SampleRecallFailure{}
	for _, card := range cards {
		if !owners[card.AssetID] {
			continue
		}
		for _, sample := range card.sampleQuestions {
			selection := SelectAskKnowledge(sample.Question, cards, budget)
			recalled := map[string]bool{}
			for _, id := range selection.Recalled {
				recalled[id] = true
			}
			sent := map[string]bool{}
			for _, sentCard := range selection.Cards {
				sent[sentCard.AssetID] = true
			}
			want := []string{card.AssetID}
			if sample.Expects != nil {
				for _, ref := range sample.Expects.Refs {
					want = append(want, ref.AssetID)
				}
			}
			failure := SampleRecallFailure{AssetID: card.AssetID, Question: sample.Question, Ambiguous: len(selection.Ambiguities) > 0}
			seen := map[string]bool{}
			for _, id := range want {
				if !seen[id] && (!recalled[id] || !sent[id]) {
					failure.Missing = append(failure.Missing, id)
				}
				seen[id] = true
			}
			if failure.Ambiguous || len(failure.Missing) > 0 {
				failures = append(failures, failure)
			}
		}
	}
	return failures
}
