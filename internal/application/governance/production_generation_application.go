package governance

import (
	"bytes"
	"context"
	"encoding/json"
	"sort"

	domain "github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
)

type productionGenerationApplicationRepository interface {
	ReplaceDraftWithGenerationTx(context.Context, identity.ProductionOperationID, domain.ProductionVersion, []domain.ProductionTarget, []domain.ProductionCandidateLink, domain.ProductionCommand, []domain.Proposal, identity.AgentRunID) error
	ReadProductionGenerationHistory(context.Context, identity.WorkspaceID, identity.PrincipalID, identity.ProductionOperationID, int) ([]string, []domain.ProductionGenerationApplication, error)
}

func (s *ProductionService) GenerationHistory(ctx context.Context, w identity.WorkspaceID, principal identity.PrincipalID, op identity.ProductionOperationID, version int) ([]string, []domain.ProductionGenerationApplication, error) {
	if repo, ok := s.repo.(productionGenerationApplicationRepository); ok {
		return repo.ReadProductionGenerationHistory(ctx, w, principal, op, version)
	}
	return []string{}, []domain.ProductionGenerationApplication{}, nil
}

func ProductionGenerationDelta(original, applied []domain.TargetDeclaration) ([]domain.ProductionGenerationDelta, json.RawMessage, error) {
	index := func(targets []domain.TargetDeclaration) (map[string]map[string]json.RawMessage, error) {
		result := map[string]map[string]json.RawMessage{}
		for _, t := range targets {
			raw, err := json.Marshal(t)
			if err != nil {
				return nil, err
			}
			var value map[string]json.RawMessage
			if json.Unmarshal(raw, &value) != nil {
				return nil, domain.ErrInvalidArgument
			}
			result[t.LocalKey] = value
		}
		return result, nil
	}
	a, err := index(original)
	if err != nil {
		return nil, nil, err
	}
	b, err := index(applied)
	if err != nil {
		return nil, nil, err
	}
	keys := map[string]bool{}
	for k := range a {
		keys[k] = true
	}
	for k := range b {
		keys[k] = true
	}
	ordered := make([]string, 0, len(keys))
	for k := range keys {
		ordered = append(ordered, k)
	}
	sort.Strings(ordered)
	delta := []domain.ProductionGenerationDelta{}
	appendChange := func(key, path string, before, after json.RawMessage) {
		x, _ := domain.CanonicalJSON(before)
		y, _ := domain.CanonicalJSON(after)
		if bytes.Equal(x, y) {
			return
		}
		op := "update"
		if before == nil {
			op = "add"
		}
		if after == nil {
			op = "remove"
		}
		delta = append(delta, domain.ProductionGenerationDelta{LocalKey: key, Change: domain.TargetChangeInput{FieldPath: path, Op: op, BeforeValue: before, AfterValue: after}})
	}
	for _, key := range ordered {
		before, oldOK := a[key]
		after, newOK := b[key]
		if !oldOK || !newOK {
			var x, y json.RawMessage
			if oldOK {
				x, _ = json.Marshal(before)
			}
			if newOK {
				y, _ = json.Marshal(after)
			}
			appendChange(key, "$target", x, y)
			continue
		}
		fields := map[string]bool{}
		for k := range before {
			fields[k] = true
		}
		for k := range after {
			fields[k] = true
		}
		names := []string{}
		for k := range fields {
			names = append(names, k)
		}
		sort.Strings(names)
		for _, field := range names {
			if field != "content" {
				appendChange(key, field, before[field], after[field])
				continue
			}
			var x, y map[string]json.RawMessage
			if json.Unmarshal(before[field], &x) != nil || json.Unmarshal(after[field], &y) != nil {
				return nil, nil, domain.ErrInvalidArgument
			}
			contentFields := map[string]bool{}
			for k := range x {
				contentFields[k] = true
			}
			for k := range y {
				contentFields[k] = true
			}
			contentNames := []string{}
			for k := range contentFields {
				contentNames = append(contentNames, k)
			}
			sort.Strings(contentNames)
			for _, name := range contentNames {
				appendChange(key, "content."+name, x[name], y[name])
			}
		}
	}
	if len(delta) > 256 {
		return nil, nil, domain.ErrLimitExceeded
	}
	raw, err := json.Marshal(delta)
	if err != nil {
		return nil, nil, err
	}
	raw, err = domain.CanonicalJSON(raw)
	if err != nil {
		return nil, nil, err
	}
	if len(raw) > 1<<20 {
		return nil, nil, domain.ErrLimitExceeded
	}
	return delta, raw, nil
}
