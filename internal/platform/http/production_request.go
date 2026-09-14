package httpapi

import (
	"encoding/json"
	"fmt"
	"strings"

	domain "github.com/iiwish/semlia/internal/domain/governance"
)

func validProductionIdempotencyKey(key string) bool {
	return len(key) >= 8 && len(key) <= 128 && strings.IndexFunc(key, func(r rune) bool { return r < 33 || r > 126 }) < 0
}

func validateProductionRequestShape(raw []byte) error {
	value, err := domain.ParseProductionJSON(raw)
	if err != nil {
		return err
	}
	root, ok := value.(map[string]any)
	if !ok {
		return domain.ErrInvalidArgument
	}
	input, ok := root["input"].(map[string]any)
	if !ok {
		return domain.ErrInvalidArgument
	}
	for _, key := range []string{"snapshots", "candidates", "evidence", "dependencies"} {
		if _, ok := input[key].([]any); !ok {
			return fmt.Errorf("%w: input.%s must be an array", domain.ErrInvalidArgument, key)
		}
	}
	targets, ok := root["targets"].([]any)
	if !ok {
		return domain.ErrInvalidArgument
	}
	for _, rawTarget := range targets {
		target, ok := rawTarget.(map[string]any)
		if !ok {
			return domain.ErrInvalidArgument
		}
		for _, key := range []string{"changes", "evidenceIds"} {
			if _, ok := target[key].([]any); !ok {
				return fmt.Errorf("%w: target.%s must be an array", domain.ErrInvalidArgument, key)
			}
		}
		if _, ok := target["content"].(map[string]any); !ok {
			return domain.ErrInvalidArgument
		}
		if changes := target["changes"].([]any); len(changes) > 0 {
			for _, rawChange := range changes {
				change, ok := rawChange.(map[string]any)
				if !ok {
					return domain.ErrInvalidArgument
				}
				keys := []string{"fieldPath", "op"}
				if change["op"] != "add" {
					keys = append(keys, "beforeValue")
				}
				if change["op"] != "remove" {
					keys = append(keys, "afterValue")
				}
				for _, key := range keys {
					if _, exists := change[key]; !exists {
						return fmt.Errorf("%w: change.%s is required", domain.ErrInvalidArgument, key)
					}
				}
			}
		}
	}
	for _, dependency := range input["dependencies"].([]any) {
		rawDependency, err := json.Marshal(dependency)
		if err != nil {
			return err
		}
		if _, err := domain.ParseProductionPublishedReference(rawDependency); err != nil {
			return err
		}
	}
	return nil
}
