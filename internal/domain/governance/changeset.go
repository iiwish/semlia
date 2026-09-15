package governance

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// ApplyChangeSet resolves a proposal change-set against the current JSON
// object content. It is the single application rule shared by the release
// publish path (semantic-asset revision content and governed-object content
// alike): add requires the field to be absent, update requires it to exist,
// remove deletes it, and the result re-encodes canonically so the stored
// content digest is reproducible.
func ApplyChangeSet(content json.RawMessage, items []ChangeSetItem) (json.RawMessage, error) {
	steps := make([]patchStep, 0, len(items))
	for _, item := range items {
		switch item.Op {
		case ChangeAdd:
			steps = append(steps, patchStep{path: splitFieldPath(item.FieldPath), forbid: true, setValue: item.AfterValue})
		case ChangeUpdate:
			steps = append(steps, patchStep{path: splitFieldPath(item.FieldPath), require: true, setValue: item.AfterValue})
		case ChangeRemove:
			steps = append(steps, patchStep{path: splitFieldPath(item.FieldPath), require: true, remove: true})
		default:
			return nil, fmt.Errorf("%w: change op %q", ErrInvalidArgument, item.Op)
		}
	}
	return applyPatch(content, steps)
}

// ApplyInverseChangeSet resolves the rollback counterpart of a change-set
// against the current JSON object content: updates restore the before value,
// adds remove the field, and removes restore the removed before value as a
// new write. The inverse is applied as a fresh mutation (a new asset revision
// pointer or a new governed-object version), never by mutating history.
func ApplyInverseChangeSet(content json.RawMessage, items []ChangeSetItem) (json.RawMessage, error) {
	steps := make([]patchStep, 0, len(items))
	for _, item := range items {
		switch item.Op {
		case ChangeUpdate:
			steps = append(steps, patchStep{path: splitFieldPath(item.FieldPath), require: true, setValue: item.BeforeValue})
		case ChangeAdd:
			steps = append(steps, patchStep{path: splitFieldPath(item.FieldPath), require: true, remove: true})
		case ChangeRemove:
			steps = append(steps, patchStep{path: splitFieldPath(item.FieldPath), setValue: item.BeforeValue})
		default:
			return nil, fmt.Errorf("%w: change op %q", ErrInvalidArgument, item.Op)
		}
	}
	return applyPatch(content, steps)
}

type patchStep struct {
	path     []string
	require  bool
	forbid   bool
	remove   bool
	setValue json.RawMessage
}

func applyPatch(content json.RawMessage, steps []patchStep) (json.RawMessage, error) {
	var document map[string]any
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.UseNumber()
	if err := decoder.Decode(&document); err != nil || document == nil {
		return nil, fmt.Errorf("%w: patched content must be a JSON object", ErrInvalidArgument)
	}
	if err := decoder.Decode(&struct{}{}); err == nil {
		return nil, fmt.Errorf("%w: patched content has trailing JSON", ErrInvalidArgument)
	}
	for _, step := range steps {
		if err := step.apply(document); err != nil {
			return nil, err
		}
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		return nil, fmt.Errorf("%w: canonical patched content", ErrInvalidArgument)
	}
	return encoded, nil
}

func (step patchStep) apply(document map[string]any) error {
	container, key, err := resolveContainer(document, step.path)
	if err != nil {
		return err
	}
	_, exists := container[key]
	if step.require && !exists {
		return fmt.Errorf("%w: field path %q does not exist", ErrInvalidArgument, strings.Join(step.path, "."))
	}
	if step.forbid && exists {
		return fmt.Errorf("%w: field path %q already exists", ErrInvalidArgument, strings.Join(step.path, "."))
	}
	if step.remove {
		delete(container, key)
		return nil
	}
	value, err := decodePatchValue(step.setValue)
	if err != nil {
		return err
	}
	container[key] = value
	return nil
}

func resolveContainer(document map[string]any, path []string) (map[string]any, string, error) {
	if len(path) == 0 {
		return nil, "", fmt.Errorf("%w: empty field path", ErrInvalidArgument)
	}
	container := document
	for _, segment := range path[:len(path)-1] {
		next, exists := container[segment]
		if !exists {
			return nil, "", fmt.Errorf("%w: field path segment %q does not exist", ErrInvalidArgument, segment)
		}
		nested, isObject := next.(map[string]any)
		if !isObject {
			return nil, "", fmt.Errorf("%w: field path segment %q is not an object", ErrInvalidArgument, segment)
		}
		container = nested
	}
	return container, path[len(path)-1], nil
}

func splitFieldPath(value string) []string {
	return strings.Split(value, ".")
}

func decodePatchValue(raw json.RawMessage) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, fmt.Errorf("%w: patch value", ErrInvalidArgument)
	}
	return value, nil
}
