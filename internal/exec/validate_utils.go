package exec

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/open-policy-agent/opa/loader"
	"github.com/open-policy-agent/opa/rego"
	"github.com/open-policy-agent/opa/sdk"
	opaTestServer "github.com/open-policy-agent/opa/sdk/test"
	"github.com/pkg/errors"
	"github.com/santhosh-tekuri/jsonschema/v5"

	u "github.com/cloudposse/atmos/pkg/utils"
)

// ValidateWithJsonSchema validates the data structure using the provided JSON Schema document
// https://github.com/santhosh-tekuri/jsonschema
// https://go.dev/play/p/Hhax3MrtD8r
func ValidateWithJsonSchema(data any, schemaName string, schemaText string) (bool, error) {
	// Convert the data to JSON and back to Go map to prevent the error:
	// jsonschema: invalid jsonType: map[interface {}]interface {}
	dataJson, err := u.ConvertToJSONFast(data)
	if err != nil {
		return false, err
	}

	dataFromJson, err := u.ConvertFromJSON(dataJson)
	if err != nil {
		return false, err
	}

	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource(schemaName, strings.NewReader(schemaText)); err != nil {
		return false, err
	}

	compiler.Draft = jsonschema.Draft2020

	schema, err := compiler.Compile(schemaName)
	if err != nil {
		return false, err
	}

	if err = schema.Validate(dataFromJson); err != nil {
		switch e := err.(type) {
		case *jsonschema.ValidationError:
			b, err2 := json.MarshalIndent(e.BasicOutput(), "", "  ")
			if err2 != nil {
				return false, err2
			}
			return false, errors.New(string(b))
		default:
			return false, err
		}
	}

	return true, nil
}

// ValidateWithOpa validates the data structure using the provided OPA document
// https://github.com/open-policy-agent/opa/blob/main/rego/example_test.go
// https://github.com/open-policy-agent/opa/blob/main/rego/rego_test.go
// https://www.openpolicyagent.org/docs/latest/integration/#sdk
func ValidateWithOpa(
	data any,
	schemaPath string,
	modulePaths []string,
	timeoutSeconds int,
) (bool, error) {
	// Set timeout for schema validation
	if timeoutSeconds == 0 {
		timeoutSeconds = 20
	}

	timeoutErrorMessage := "Timeout evaluating the OPA policy. Please check the following:\n" +
		"1. Rego syntax\n" +
		"2. If 're_match' function is used and the regex pattern contains a backslash to escape special chars, the backslash itself must be escaped with another backslash"

	invalidRegoPolicyErrorMessage := fmt.Sprintf("invalid Rego policy in the file '%s'", schemaPath)

	// https://stackoverflow.com/questions/17573190/how-to-multiply-duration-by-integer
	ctx, cancelFunc := context.WithTimeout(context.TODO(), time.Second*time.Duration(timeoutSeconds))
	defer cancelFunc()

	// Load the input document
	j, err := u.ConvertToJSON(data)
	if err != nil {
		return false, err
	}

	var input any
	dec := json.NewDecoder(bytes.NewBufferString(j))
	dec.UseNumber()
	if err = dec.Decode(&input); err != nil {
		return false, err
	}

	// Construct a Rego object that can be prepared or evaluated.
	r := rego.New(
		rego.Query("data.atmos.errors"),
		rego.Load(append([]string{schemaPath}, modulePaths...),
			loader.GlobExcludeName("*_test.rego", 0),
		),
	)

	// Create a prepared query that can be evaluated
	query, err := r.PrepareForEval(ctx)
	if err != nil {
		return false, err
	}

	// Execute the prepared query
	rs, err := query.Eval(ctx, rego.EvalInput(input))
	if err != nil {
		if err.Error() == "context deadline exceeded" {
			err = errors.New(timeoutErrorMessage)
		}
		return false, err
	}

	if len(rs) < 1 {
		return false, errors.New(invalidRegoPolicyErrorMessage)
	}

	if len(rs[0].Expressions) < 1 {
		return false, errors.New(invalidRegoPolicyErrorMessage)
	}

	// Check the query evaluation result (if the `errors` output array has any items)
	ers, ok := rs[0].Expressions[0].Value.([]any)
	if !ok {
		return false, errors.New(invalidRegoPolicyErrorMessage)
	}
	if len(ers) > 0 {
		return false, errors.New(strings.Join(u.SliceOfInterfacesToSliceOdStrings(ers), "\n"))
	}

	return true, nil
}

// ValidateWithOpaLegacy validates the data structure using the provided OPA document
// https://www.openpolicyagent.org/docs/latest/integration/#sdk
func ValidateWithOpaLegacy(
	data any,
	schemaName string,
	schemaText string,
	timeoutSeconds int,
) (bool, error) {
	// The OPA SDK does not support map[any]any data types (which can be part of 'data' input)
	// ast: interface conversion: json: unsupported type: map[interface {}]interface {}
	// To fix the issue, convert the data to JSON and back to Go map
	dataJson, err := u.ConvertToJSONFast(data)
	if err != nil {
		return false, err
	}

	dataFromJson, err := u.ConvertFromJSON(dataJson)
	if err != nil {
		return false, err
	}

	// Set timeout for schema validation
	if timeoutSeconds == 0 {
		timeoutSeconds = 20
	}

	// https://stackoverflow.com/questions/17573190/how-to-multiply-duration-by-integer
	ctx, cancelFunc := context.WithTimeout(context.TODO(), time.Second*time.Duration(timeoutSeconds))
	defer cancelFunc()

	// '/bundles/' prefix is required by the OPA SDK
	bundleSchemaName := "/bundles/validate"

	// Create a bundle server
	server, err := opaTestServer.NewServer(opaTestServer.MockBundle(bundleSchemaName, map[string]string{schemaName: schemaText}))
	if err != nil {
		return false, err
	}

	defer server.Stop()

	// Provide the OPA configuration which specifies fetching policy bundles
	config := []byte(fmt.Sprintf(`{
		"services": {
			"validate": {
				"url": %q
			}
		},
		"bundles": {
			"validate": {
				"resource": %s
			}
		}
	}`, server.URL(), bundleSchemaName))

	timeoutErrorMessage := "Timeout evaluating the OPA policy. Please check the following:\n" +
		"1. Rego syntax\n" +
		"2. If 're_match' function is used and the regex pattern contains a backslash to escape special chars, the backslash itself must be escaped with another backslash"

	// Create an instance of the OPA object
	opa, err := sdk.New(ctx, sdk.Options{
		Config: bytes.NewReader(config),
	})
	if err != nil {
		if err.Error() == "context deadline exceeded" {
			err = errors.New(timeoutErrorMessage)
		}
		return false, err
	}

	defer opa.Stop(ctx)

	var result *sdk.DecisionResult
	if result, err = opa.Decision(ctx, sdk.DecisionOptions{
		Path:  "/atmos/errors",
		Input: dataFromJson,
	}); err != nil {
		if err.Error() == "context deadline exceeded" {
			err = errors.New(timeoutErrorMessage)
		}
		return false, err
	}

	ers, ok := result.Result.([]any)
	if ok && len(ers) > 0 {
		return false, errors.New(strings.Join(u.SliceOfInterfacesToSliceOdStrings(ers), "\n"))
	}

	return true, nil
}

// ValidateWithCue validates the data structure using the provided CUE document
// https://cuelang.org/docs/integrations/go/#processing-cue-in-go
func ValidateWithCue(data any, schemaName string, schemaText string) (bool, error) {
	return false, errors.New("validation using CUE is not supported yet")
}
