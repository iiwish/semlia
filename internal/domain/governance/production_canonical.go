package governance

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// ParseProductionJSON rejects ambiguous input before encoding can discard it.
func ParseProductionJSON(raw json.RawMessage) (any, error) {
	if len(raw) > 1<<20 {
		return nil, ErrLimitExceeded
	}
	if !utf8.Valid(raw) {
		return nil, ErrInvalidArgument
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var read func(int) (any, error)
	read = func(depth int) (any, error) {
		token, err := d.Token()
		if err != nil {
			return nil, fmt.Errorf("%w: malformed JSON", ErrInvalidArgument)
		}
		switch t := token.(type) {
		case json.Delim:
			if depth >= 32 {
				return nil, ErrLimitExceeded
			}
			if t == '{' {
				object := map[string]any{}
				for d.More() {
					keyToken, err := d.Token()
					if err != nil {
						return nil, ErrInvalidArgument
					}
					key, ok := keyToken.(string)
					if !ok {
						return nil, ErrInvalidArgument
					}
					if _, exists := object[key]; exists {
						return nil, fmt.Errorf("%w: duplicate JSON key", ErrInvalidArgument)
					}
					value, err := read(depth + 1)
					if err != nil {
						return nil, err
					}
					object[key] = value
				}
				if end, err := d.Token(); err != nil || end != json.Delim('}') {
					return nil, ErrInvalidArgument
				}
				return object, nil
			}
			if t == '[' {
				array := []any{}
				for d.More() {
					value, err := read(depth + 1)
					if err != nil {
						return nil, err
					}
					array = append(array, value)
				}
				if end, err := d.Token(); err != nil || end != json.Delim(']') {
					return nil, ErrInvalidArgument
				}
				return array, nil
			}
			return nil, ErrInvalidArgument
		case json.Number:
			n, err := strconv.ParseFloat(string(t), 64)
			if err != nil || math.IsInf(n, 0) || math.IsNaN(n) {
				return nil, fmt.Errorf("%w: number outside supported range", ErrInvalidArgument)
			}
		}
		return token, nil
	}
	value, err := read(0)
	if err != nil {
		return nil, err
	}
	if _, err := d.Token(); err != io.EOF {
		return nil, fmt.Errorf("%w: trailing JSON", ErrInvalidArgument)
	}
	return value, nil
}

func CanonicalProductionInput(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 {
		return raw, nil
	}
	value, err := ParseProductionJSON(raw)
	if err != nil {
		return nil, err
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, ErrInvalidArgument
	}
	for _, set := range []struct{ name, identity string }{{"snapshots", "snapshotId"}, {"candidates", "candidateId"}, {"evidence", "evidenceId"}, {"dependencies", "targetId"}} {
		itemsValue, exists := object[set.name]
		if !exists {
			continue
		}
		items, ok := itemsValue.([]any)
		if !ok {
			return nil, ErrInvalidArgument
		}
		seen := map[string]bool{}
		for _, item := range items {
			entry, ok := item.(map[string]any)
			if !ok {
				return nil, ErrInvalidArgument
			}
			key, ok := entry[set.identity].(string)
			if !ok || key == "" || seen[key] {
				return nil, fmt.Errorf("%w: missing or duplicate input identity", ErrInvalidArgument)
			}
			seen[key] = true
			if set.name == "snapshots" {
				if source, ok := entry["sourceId"].(string); !ok || source == "" {
					return nil, fmt.Errorf("%w: snapshot sourceId must be a nonempty string", ErrInvalidArgument)
				}
			}
			for _, field := range []string{"coverageKeys", "targetKeys"} {
				if nested, exists := entry[field]; exists {
					array, ok := nested.([]any)
					if !ok {
						return nil, ErrInvalidArgument
					}
					stringsSet := make([]string, len(array))
					for j, v := range array {
						stringsSet[j], ok = v.(string)
						if !ok {
							return nil, ErrInvalidArgument
						}
					}
					sorted, err := productionStringSet(stringsSet)
					if err != nil {
						return nil, err
					}
					entry[field] = sorted
				}
			}
		}
		sort.Slice(items, func(i, j int) bool {
			a, b := items[i].(map[string]any), items[j].(map[string]any)
			if set.name == "snapshots" && a["sourceId"].(string) != b["sourceId"].(string) {
				return a["sourceId"].(string) < b["sourceId"].(string)
			}
			return a[set.identity].(string) < b[set.identity].(string)
		})
	}
	return json.Marshal(object)
}

// NormalizeProductionDeclarations copies caller-owned slices before sorting sets.
// Change arrays and business content keep their original order and text.
func NormalizeProductionDeclarations(targets []TargetDeclaration, candidates []CandidateDeclaration) ([]TargetDeclaration, []CandidateDeclaration, error) {
	ts := append([]TargetDeclaration(nil), targets...)
	cs := append([]CandidateDeclaration(nil), candidates...)
	for i := range ts {
		ts[i].Changes = append([]TargetChangeInput{}, ts[i].Changes...)
		ts[i].Title = strings.TrimSpace(ts[i].Title)
		if len(ts[i].Content) != 0 {
			value, err := ParseProductionJSON(ts[i].Content)
			if err != nil {
				return nil, nil, err
			}
			ts[i].Content, err = json.Marshal(value)
			if err != nil {
				return nil, nil, err
			}
		}
		var err error
		ts[i].EvidenceIDs, err = productionStringSet(ts[i].EvidenceIDs)
		if err != nil {
			return nil, nil, err
		}
	}
	for i := range cs {
		var err error
		cs[i].TargetKeys, err = productionStringSet(cs[i].TargetKeys)
		if err != nil {
			return nil, nil, err
		}
	}
	sort.Slice(ts, func(i, j int) bool { return ts[i].LocalKey < ts[j].LocalKey })
	sort.Slice(cs, func(i, j int) bool { return cs[i].CandidateID < cs[j].CandidateID })
	for i := 1; i < len(ts); i++ {
		if ts[i-1].LocalKey == ts[i].LocalKey {
			return nil, nil, fmt.Errorf("%w: duplicate localKey", ErrInvalidArgument)
		}
	}
	for i := 1; i < len(cs); i++ {
		if cs[i-1].CandidateID == cs[i].CandidateID {
			return nil, nil, fmt.Errorf("%w: duplicate candidate", ErrInvalidArgument)
		}
	}
	return ts, cs, nil
}

func productionStringSet(values []string) ([]string, error) {
	result := append([]string{}, values...)
	sort.Strings(result)
	for i := 1; i < len(result); i++ {
		if result[i] == result[i-1] {
			return nil, fmt.Errorf("%w: duplicate set member", ErrInvalidArgument)
		}
	}
	return result, nil
}

func ValidateProductionCanonicalBudget(input json.RawMessage, targets []TargetDeclaration) error {
	data, err := json.Marshal(struct {
		Input   json.RawMessage     `json:"input"`
		Targets []TargetDeclaration `json:"targets"`
	}{input, targets})
	if err != nil {
		return fmt.Errorf("%w: malformed production content", ErrInvalidArgument)
	}
	canonical, err := CanonicalJSON(data)
	if err != nil {
		return err
	}
	if len(canonical) > MaxCanonicalInputBytes {
		return fmt.Errorf("%w: canonical input and targets exceed 768 KiB", ErrLimitExceeded)
	}
	return nil
}
