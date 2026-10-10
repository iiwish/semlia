package semantic

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// Vocabulary is the governed business wording of one asset: how people refer to it, where its boundary lies and
// which neighbours it is easily confused with. It lives at the top level of revision content beside displayName and
// definition, is versioned and reviewed like any other field, and never changes computation.
type Vocabulary struct {
	Aliases         []string         `json:"aliases,omitempty"`
	Includes        []string         `json:"includes,omitempty"`
	Excludes        []string         `json:"excludes,omitempty"`
	Examples        []string         `json:"examples,omitempty"`
	ConfusableWith  []Confusion      `json:"confusableWith,omitempty"`
	SampleQuestions []SampleQuestion `json:"sampleQuestions,omitempty"`
}

// Confusion names a neighbouring asset and states how to tell the two apart.
type Confusion struct {
	Ref         KnowledgeReference `json:"ref"`
	Distinction string             `json:"distinction"`
}

// SampleQuestion is a typical question this asset answers. Expected references are what retrieval must surface for it.
type SampleQuestion struct {
	Question string             `json:"question"`
	Expects  *SampleExpectation `json:"expects,omitempty"`
}

type SampleExpectation struct {
	Refs []KnowledgeReference `json:"refs"`
}

// ValueAlias maps business wording onto one canonical category value of a member.
type ValueAlias struct {
	Value   string   `json:"value"`
	Aliases []string `json:"aliases"`
}

// VocabularyFields are the content keys owned by Vocabulary.
const VocabularyFields = "aliases includes excludes examples confusableWith sampleQuestions"

const (
	maxAliases         = 32
	maxAliasRunes      = 64
	maxBoundaryItems   = 32
	maxBoundaryRunes   = 256
	maxConfusions      = 16
	maxDistinctionRune = 512
	maxSampleQuestions = 16
	maxQuestionRunes   = 256
	maxExpectedRefs    = 8
	maxMemberAliases   = 16
	maxValueAliases    = 64
)

// ParseVocabulary decodes the vocabulary keys of revision content and validates their shape. Other content keys are
// ignored here; the owning content schema decides which keys may exist.
func ParseVocabulary(content json.RawMessage) (Vocabulary, error) {
	vocabulary := Vocabulary{}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(content, &object); err != nil || object == nil {
		return vocabulary, fmt.Errorf("invalid vocabulary: expected object")
	}
	subset := map[string]json.RawMessage{}
	for _, key := range strings.Fields(VocabularyFields) {
		if raw, ok := object[key]; ok && !bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			subset[key] = raw
		}
	}
	encoded, _ := json.Marshal(subset)
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&vocabulary); err != nil {
		return vocabulary, fmt.Errorf("invalid vocabulary: %s", err.Error())
	}
	return vocabulary, vocabulary.validate(displayNameOf(object))
}

func displayNameOf(object map[string]json.RawMessage) string {
	var name string
	_ = json.Unmarshal(object["displayName"], &name)
	return name
}

func (v Vocabulary) validate(displayName string) error {
	bad := func(message string) error { return fmt.Errorf("invalid vocabulary: %s", message) }
	if err := validTermList(v.Aliases, maxAliases, maxAliasRunes); err != nil {
		return bad("aliases " + err.Error())
	}
	if name := NormalizeTerm(displayName); name != "" {
		for _, alias := range v.Aliases {
			if NormalizeTerm(alias) == name {
				return bad("alias repeats the display name")
			}
		}
	}
	for label, list := range map[string][]string{"includes": v.Includes, "excludes": v.Excludes, "examples": v.Examples} {
		if err := validTextList(list, maxBoundaryItems, maxBoundaryRunes); err != nil {
			return bad(label + " " + err.Error())
		}
	}
	if len(v.ConfusableWith) > maxConfusions {
		return bad("too many confusable assets")
	}
	seen := map[string]bool{}
	for _, confusion := range v.ConfusableWith {
		if err := confusion.Ref.Validate(); err != nil || confusion.Ref.MemberID != "" {
			return bad("confusable reference must name an asset")
		}
		if seen[confusion.Ref.AssetID] {
			return bad("duplicate confusable asset")
		}
		seen[confusion.Ref.AssetID] = true
		if !validText(confusion.Distinction, maxDistinctionRune) {
			return bad("confusable asset requires a distinction")
		}
	}
	if len(v.SampleQuestions) > maxSampleQuestions {
		return bad("too many sample questions")
	}
	questions := map[string]bool{}
	for _, sample := range v.SampleQuestions {
		if !validText(sample.Question, maxQuestionRunes) {
			return bad("blank or oversized sample question")
		}
		key := NormalizeTerm(sample.Question)
		if questions[key] {
			return bad("duplicate sample question")
		}
		questions[key] = true
		if sample.Expects == nil {
			continue
		}
		if len(sample.Expects.Refs) == 0 || len(sample.Expects.Refs) > maxExpectedRefs {
			return bad("sample expectation requires 1 to 8 references")
		}
		for _, ref := range sample.Expects.Refs {
			if err := ref.Validate(); err != nil {
				return bad("invalid sample expectation reference")
			}
		}
	}
	return nil
}

// References lists every published asset the vocabulary points at.
func (v Vocabulary) References() []KnowledgeReference {
	refs := []KnowledgeReference{}
	for _, confusion := range v.ConfusableWith {
		refs = append(refs, confusion.Ref)
	}
	for _, sample := range v.SampleQuestions {
		if sample.Expects != nil {
			refs = append(refs, sample.Expects.Refs...)
		}
	}
	return refs
}

func validateMemberVocabulary(kind AssetType, member KnowledgeMember) error {
	if err := validTermList(member.Aliases, maxMemberAliases, maxAliasRunes); err != nil {
		return fmt.Errorf("member %s aliases %s", member.ID, err.Error())
	}
	name := NormalizeTerm(member.Name)
	for _, alias := range member.Aliases {
		if NormalizeTerm(alias) == name {
			return fmt.Errorf("member %s alias repeats its name", member.ID)
		}
	}
	if len(member.ValueAliases) == 0 {
		return nil
	}
	if kind != BusinessObject || (member.ValueType != "" && member.ValueType != "string") {
		return fmt.Errorf("member %s value aliases require a string business attribute", member.ID)
	}
	if len(member.ValueAliases) > maxValueAliases {
		return fmt.Errorf("member %s has too many value aliases", member.ID)
	}
	values, claimed := map[string]bool{}, map[string]string{}
	for _, entry := range member.ValueAliases {
		value := NormalizeTerm(entry.Value)
		if !validText(entry.Value, maxAliasRunes) || values[value] {
			return fmt.Errorf("member %s has a blank or duplicate canonical value", member.ID)
		}
		values[value] = true
		if len(entry.Aliases) == 0 {
			return fmt.Errorf("member %s value %q declares no aliases", member.ID, entry.Value)
		}
		if err := validTermList(entry.Aliases, maxMemberAliases, maxAliasRunes); err != nil {
			return fmt.Errorf("member %s value aliases %s", member.ID, err.Error())
		}
		for _, alias := range entry.Aliases {
			term := NormalizeTerm(alias)
			if term == value {
				return fmt.Errorf("member %s value alias repeats its value", member.ID)
			}
			if previous, ok := claimed[term]; ok && previous != value {
				return fmt.Errorf("member %s value alias %q maps to two values", member.ID, alias)
			}
			claimed[term] = value
		}
	}
	// A canonical value can never also be an alias of another value of the same member.
	for term, value := range claimed {
		if values[term] && term != value {
			return fmt.Errorf("member %s value alias shadows a canonical value", member.ID)
		}
	}
	return nil
}

// NormalizeTerm folds width and case and collapses whitespace so wording that a person would read as the same term
// compares equal.
func NormalizeTerm(value string) string {
	return strings.Join(strings.Fields(strings.ToLower(norm.NFKC.String(value))), " ")
}

func validText(value string, max int) bool {
	return utf8.ValidString(value) && strings.TrimSpace(value) != "" && utf8.RuneCountInString(value) <= max
}

func validTextList(values []string, limit, max int) error {
	if len(values) > limit {
		return fmt.Errorf("exceed %d items", limit)
	}
	for _, value := range values {
		if !validText(value, max) {
			return fmt.Errorf("contain a blank or oversized item")
		}
	}
	return nil
}

func validTermList(values []string, limit, max int) error {
	if err := validTextList(values, limit, max); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, value := range values {
		term := NormalizeTerm(value)
		if seen[term] {
			return fmt.Errorf("contain a duplicate %q", value)
		}
		seen[term] = true
	}
	return nil
}

// VocabularyEntry is one released asset as the conflict check sees it.
type VocabularyEntry struct {
	AssetID     string
	AssetType   AssetType
	DisplayName string
	Vocabulary  Vocabulary
}

// VocabularyConflict is one term that names two assets of the same type without either declaring the other
// confusable. Assets are ordered by asset ID so the result is stable.
type VocabularyConflict struct {
	Term      string
	AssetType AssetType
	Assets    [2]VocabularyClaim
}

type VocabularyClaim struct {
	AssetID     string
	DisplayName string
	// ByAlias is false when the term is the asset's display name.
	ByAlias bool
}

// VocabularyConflicts finds terms that would resolve to two different assets of one type. Two display names that
// collide are left to the existing naming rules; at least one side must be an alias. Declaring confusableWith on
// either side accepts the overlap, because retrieval can then ask which one was meant and say how they differ.
func VocabularyConflicts(entries []VocabularyEntry) []VocabularyConflict {
	type claim struct {
		entry   int
		byAlias bool
	}
	claims := map[string][]claim{}
	for index, entry := range entries {
		key := func(term string) string { return string(entry.AssetType) + "\x00" + NormalizeTerm(term) }
		if NormalizeTerm(entry.DisplayName) != "" {
			claims[key(entry.DisplayName)] = append(claims[key(entry.DisplayName)], claim{index, false})
		}
		for _, alias := range entry.Vocabulary.Aliases {
			claims[key(alias)] = append(claims[key(alias)], claim{index, true})
		}
	}
	confusable := func(left, right VocabularyEntry) bool {
		for _, pair := range [][2]VocabularyEntry{{left, right}, {right, left}} {
			for _, confusion := range pair[0].Vocabulary.ConfusableWith {
				if confusion.Ref.AssetID == pair[1].AssetID {
					return true
				}
			}
		}
		return false
	}
	conflicts := []VocabularyConflict{}
	for key, list := range claims {
		for i := 0; i < len(list); i++ {
			for j := i + 1; j < len(list); j++ {
				left, right := list[i], list[j]
				a, b := entries[left.entry], entries[right.entry]
				if a.AssetID == b.AssetID || (!left.byAlias && !right.byAlias) || confusable(a, b) {
					continue
				}
				if b.AssetID < a.AssetID {
					a, b = b, a
					left, right = right, left
				}
				conflicts = append(conflicts, VocabularyConflict{
					Term: key[strings.IndexByte(key, 0)+1:], AssetType: a.AssetType,
					Assets: [2]VocabularyClaim{{a.AssetID, a.DisplayName, left.byAlias}, {b.AssetID, b.DisplayName, right.byAlias}},
				})
			}
		}
	}
	sort.Slice(conflicts, func(i, j int) bool {
		x, y := conflicts[i], conflicts[j]
		if x.Term != y.Term {
			return x.Term < y.Term
		}
		if x.Assets[0].AssetID != y.Assets[0].AssetID {
			return x.Assets[0].AssetID < y.Assets[0].AssetID
		}
		return x.Assets[1].AssetID < y.Assets[1].AssetID
	})
	return conflicts
}

// VocabularyEntryFromContent builds a conflict-check entry from released revision content.
func VocabularyEntryFromContent(assetID string, kind AssetType, content json.RawMessage) (VocabularyEntry, error) {
	vocabulary, err := ParseVocabulary(content)
	if err != nil {
		return VocabularyEntry{}, err
	}
	var object map[string]json.RawMessage
	_ = json.Unmarshal(content, &object)
	return VocabularyEntry{AssetID: assetID, AssetType: kind, DisplayName: displayNameOf(object), Vocabulary: vocabulary}, nil
}

// ComputationalCore digests revision content without its vocabulary: top-level wording fields and member aliases
// and value aliases. Two revisions with equal cores mean and compute the same thing, so a reference pinned to one
// stays valid for the other. Every other change, including definition, scope and display name, alters the core.
func ComputationalCore(content json.RawMessage) (string, error) {
	var object map[string]any
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.UseNumber()
	if err := decoder.Decode(&object); err != nil || object == nil {
		return "", fmt.Errorf("invalid content")
	}
	for _, key := range strings.Fields(VocabularyFields) {
		delete(object, key)
	}
	if spec, ok := object["spec"].(map[string]any); ok {
		if members, ok := spec["members"].([]any); ok {
			for _, raw := range members {
				if member, ok := raw.(map[string]any); ok {
					delete(member, "aliases")
					delete(member, "valueAliases")
				}
			}
		}
	}
	encoded, err := json.Marshal(object)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}
