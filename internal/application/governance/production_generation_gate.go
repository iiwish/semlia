package governance

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"sync"

	domain "github.com/iiwish/semlia/internal/domain/governance"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

//go:embed production_output.v1.schema.json
var productionOutputSchema []byte

var productionOutputOnce sync.Once
var productionOutputCompiled *jsonschema.Schema
var productionOutputError error

func GateProductionGenerationOutput(raw []byte) (domain.ProductionSuggestions, json.RawMessage, error) {
	invalid := func() (domain.ProductionSuggestions, json.RawMessage, error) {
		return domain.ProductionSuggestions{}, nil, &AIOutputInvalidError{Violations: []string{"output violates the production suggestions contract"}}
	}
	value, err := domain.ParseProductionJSON(raw)
	if err != nil {
		return invalid()
	}
	productionOutputOnce.Do(func() {
		var schema any
		schema, productionOutputError = jsonschema.UnmarshalJSON(bytes.NewReader(productionOutputSchema))
		if productionOutputError != nil {
			return
		}
		compiler := jsonschema.NewCompiler()
		productionOutputError = compiler.AddResource(domain.ProductionSuggestionsSchema, schema)
		if productionOutputError == nil {
			productionOutputCompiled, productionOutputError = compiler.Compile(domain.ProductionSuggestionsSchema)
		}
	})
	if productionOutputError != nil {
		return domain.ProductionSuggestions{}, nil, productionOutputError
	}
	if err := productionOutputCompiled.Validate(value); err != nil {
		return invalid()
	}
	canonical, err := json.Marshal(value)
	if err != nil || len(canonical) > 1<<20 {
		return invalid()
	}
	var output domain.ProductionSuggestions
	decoder := json.NewDecoder(bytes.NewReader(canonical))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&output); err != nil {
		return invalid()
	}
	if err := domain.ValidateTargetDeclarations(output.Targets); err != nil {
		return invalid()
	}
	if err := domain.ValidateProductionContentDeclarations(output.Targets); err != nil {
		return invalid()
	}
	output.Targets, _, err = domain.NormalizeProductionDeclarations(output.Targets, nil)
	if err != nil {
		return invalid()
	}
	return output, canonical, nil
}
