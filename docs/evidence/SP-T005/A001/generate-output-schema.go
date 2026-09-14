// Command generates the runtime JSON Schema from the approved production contract.
package main

import (
	"encoding/json"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

func main() {
	raw, err := os.ReadFile("docs/specs/semantic-production/contracts/production.openapi.yaml")
	check(err)
	var doc map[string]any
	check(yaml.Unmarshal(raw, &doc))
	schemas := doc["components"].(map[string]any)["schemas"].(map[string]any)
	defs := map[string]any{}
	var convert func(any) any
	convert = func(value any) any {
		switch v := value.(type) {
		case []any:
			out := make([]any, len(v))
			for i, item := range v {
				out[i] = convert(item)
			}
			return out
		case map[string]any:
			out := map[string]any{}
			for key, item := range v {
				if key == "nullable" || key == "description" || strings.HasPrefix(key, "x-") {
					continue
				}
				if key == "$ref" {
					name := strings.TrimPrefix(item.(string), "#/components/schemas/")
					if _, exists := defs[name]; !exists {
						defs[name] = nil
						defs[name] = convert(schemas[name])
					}
					out[key] = "#/$defs/" + name
				} else {
					out[key] = convert(item)
				}
			}
			if v["nullable"] == true && out["type"] != nil {
				out["type"] = []any{out["type"], "null"}
			}
			return out
		default:
			return value
		}
	}
	root := convert(schemas["StructuredGenerationOutput"]).(map[string]any)
	root["$schema"] = "https://json-schema.org/draft/2020-12/schema"
	root["$id"] = "semlia.production-suggestions/v1"
	root["$defs"] = defs
	out, err := json.MarshalIndent(root, "", "  ")
	check(err)
	check(os.WriteFile("internal/application/governance/production_output.v1.schema.json", append(out, '\n'), 0644))
}

func check(err error) {
	if err != nil {
		panic(err)
	}
}
