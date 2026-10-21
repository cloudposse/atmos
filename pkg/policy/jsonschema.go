package policy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/santhosh-tekuri/jsonschema/v5"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// JSONSchemaEngine selects effects when the input fails schema validation.
const JSONSchemaEngine = "jsonschema"

// SchemaConfig holds the entrypoint URI and all materialized local references.
type SchemaConfig struct {
	Entry     string                     `json:"entry"`
	Resources map[string]json.RawMessage `json:"resources"`
}

// JSONSchema evaluates schemas without filesystem or network access.
type JSONSchema struct{}

// Evaluate implements the boolean adapter interface; true means invalid input.
func (j *JSONSchema) Evaluate(ctx context.Context, evaluation Evaluation) (bool, error) {
	defer perf.Track(nil, "policy.JSONSchema.Evaluate")()
	result, err := j.EvaluateDetailed(ctx, evaluation)
	return result.Matched, err
}

// EvaluateDetailed distinguishes validation failures from broken schemas.
func (j *JSONSchema) EvaluateDetailed(ctx context.Context, evaluation Evaluation) (EvaluationResult, error) {
	defer perf.Track(nil, "policy.JSONSchema.EvaluateDetailed")()
	if err := validateRequest(ctx, evaluation.Request); err != nil {
		return EvaluationResult{}, err
	}
	schema, err := compileSchema(evaluation.Schema)
	if err != nil {
		return EvaluationResult{}, err
	}
	input, err := decodeDocument(evaluation.Request.Input)
	if err != nil {
		return EvaluationResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return EvaluationResult{}, err
	}
	err = schema.Validate(input)
	if contextErr := ctx.Err(); contextErr != nil {
		return EvaluationResult{}, contextErr
	}
	if err == nil {
		return EvaluationResult{}, nil
	}
	var invalid *jsonschema.ValidationError
	if !errors.As(err, &invalid) {
		return EvaluationResult{}, fmt.Errorf("%w: JSON Schema: %w", errUtils.ErrPolicyEvaluation, err)
	}
	return EvaluationResult{Matched: true, Diagnostics: schemaDiagnostics(invalid)}, nil
}

func compileSchema(config *SchemaConfig) (*jsonschema.Schema, error) {
	if config == nil || config.Entry == "" || len(config.Resources) == 0 {
		return nil, fmt.Errorf("%w: JSON Schema requires materialized schema resources", errUtils.ErrPolicyInvalid)
	}
	compiler := jsonschema.NewCompiler()
	compiler.Draft = jsonschema.Draft2020
	compiler.LoadURL = func(uri string) (io.ReadCloser, error) {
		return nil, fmt.Errorf("%w: schema resource %q was not materialized", errUtils.ErrPolicyInvalid, uri)
	}
	total := 0
	for uri, raw := range config.Resources {
		total += len(raw)
		if total > maxPolicyDocumentBytes {
			return nil, fmt.Errorf("%w: schema resources exceed size limit", errUtils.ErrPolicyInvalid)
		}
		if err := compiler.AddResource(uri, bytes.NewReader(raw)); err != nil {
			return nil, fmt.Errorf("%w: schema resource: %w", errUtils.ErrPolicyInvalid, err)
		}
	}
	schema, err := compiler.Compile(config.Entry)
	if err != nil {
		return nil, fmt.Errorf("%w: compile JSON Schema: %w", errUtils.ErrPolicyInvalid, err)
	}
	return schema, nil
}

func schemaDiagnostics(err *jsonschema.ValidationError) []Diagnostic {
	if len(err.Causes) == 0 {
		return []Diagnostic{{Message: err.Message, Path: err.InstanceLocation, SchemaPath: err.KeywordLocation}}
	}
	var result []Diagnostic
	for _, cause := range err.Causes {
		result = append(result, schemaDiagnostics(cause)...)
	}
	return result
}
