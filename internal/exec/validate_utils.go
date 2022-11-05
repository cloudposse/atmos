package exec

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/pkg/errors"
	"strings"
	"time"

	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/open-policy-agent/opa/sdk"
	opaTestServer "github.com/open-policy-agent/opa/sdk/test"
	"github.com/santhosh-tekuri/jsonschema/v5"
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
// https://www.openpolicyagent.org/docs/latest/integration/#sdk
func ValidateWithOpa(data any, schemaName string, schemaText string) (bool, error) {
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
	ctx, cancelFunc := context.WithTimeout(context.TODO(), time.Second*7)
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

	ers, ok := result.Result.([]interface{})
	if ok && len(ers) > 0 {
		return false, errors.New(strings.Join(u.SliceOfInterfacesToSliceOdStrings(ers), "\n"))
	}

	return true, nil
}

// ValidateWithCue validates the data structure using the provided CUE document
// https://cuelang.org/docs/integrations/go/#processing-cue-in-go
func ValidateWithCue(data any, schemaName string, schemaText string) (bool, error) {
	return false, errors.New("validation using CUE is not implemented yet")
}
