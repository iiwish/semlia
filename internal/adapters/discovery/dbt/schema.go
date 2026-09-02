package dbt

import (
	"bytes"
	"compress/gzip"
	"embed"
	"fmt"
	"io"
	"sync"

	"github.com/iiwish/semlia/internal/domain/discovery"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

//go:embed schemas/*.json.gz
var schemaFiles embed.FS

var (
	schemasOnce      sync.Once
	manifestSchema   *jsonschema.Schema
	catalogSchema    *jsonschema.Schema
	schemaCompileErr error
)

func validatePublishedSchemas(manifestJSON, catalogJSON []byte) error {
	schemasOnce.Do(compileSchemas)
	if schemaCompileErr != nil {
		return fmt.Errorf("compile embedded dbt schemas: %w", schemaCompileErr)
	}
	for name, validation := range map[string]struct {
		content []byte
		schema  *jsonschema.Schema
	}{
		"manifest.json": {manifestJSON, manifestSchema},
		"catalog.json":  {catalogJSON, catalogSchema},
	} {
		document, err := jsonschema.UnmarshalJSON(bytes.NewReader(validation.content))
		if err != nil || validation.schema.Validate(document) != nil {
			return fmt.Errorf("%w: %s does not match its published dbt schema", discovery.ErrInvalidInput, name)
		}
	}
	return nil
}

func compileSchemas() {
	manifestSchema, schemaCompileErr = compileSchema("schemas/manifest-v12.json.gz", manifestSchemaV12)
	if schemaCompileErr != nil {
		return
	}
	catalogSchema, schemaCompileErr = compileSchema("schemas/catalog-v1.json.gz", catalogSchemaV1)
}

func compileSchema(path, schemaURL string) (*jsonschema.Schema, error) {
	compressed, err := schemaFiles.ReadFile(path)
	if err != nil {
		return nil, err
	}
	reader, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	content, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(content))
	if err != nil {
		return nil, err
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource(schemaURL, document); err != nil {
		return nil, err
	}
	return compiler.Compile(schemaURL)
}
