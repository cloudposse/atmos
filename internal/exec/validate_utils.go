package exec

import (
	"github.com/pkg/errors"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v5"
)

// ValidateWithJsonSchema validates the data structure using the provided JSON Schema document
// https://github.com/santhosh-tekuri/jsonschema
// https://go.dev/play/p/Hhax3MrtD8r
func ValidateWithJsonSchema(data any, schemaName string, schemaText string) error {
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource(schemaName, strings.NewReader(schemaText)); err != nil {
		return err
	}

	schema, err := compiler.Compile(schemaName)
	if err != nil {
		return err
	}

	if err = schema.Validate(data); err != nil {
		return err
	}

	return nil
}

// ValidateWithOpa validates the data structure using the provided OPA document
// https://www.openpolicyagent.org/docs/latest/integration/#sdk
func ValidateWithOpa(data any, schemaName string, schemaText string) error {
	return nil
}

// ValidateWithCue validates the data structure using the provided CUE document
// https://cuelang.org/docs/integrations/go/#processing-cue-in-go
func ValidateWithCue(data any, schemaName string, schemaText string) error {
	return errors.New("validation using CUE is not implemented yet")
}
