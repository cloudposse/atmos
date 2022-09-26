package exec

import (
	"bytes"
	"context"
	"fmt"
	"github.com/pkg/errors"
	"strings"

	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/open-policy-agent/opa/sdk"
	opaTestServer "github.com/open-policy-agent/opa/sdk/test"
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
	// The OPA SDK does not support map[any]any data types
	// ast: interface conversion: json: unsupported type: map[interface {}]interface {}
	// Convert the data to JSON and back to Go map
	dataJson, err := u.ConvertToJSONFast(data)
	if err != nil {
		return err
	}

	dataFromJson, err := u.ConvertFromJSON(dataJson)
	if err != nil {
		return err
	}

	ctx := context.Background()

	// '/bundles/' prefix is required by the OPA SDK
	bundleSchemaName := "/bundles/" + schemaName

	// Create a bundle server
	server, err := opaTestServer.NewServer(opaTestServer.MockBundle(bundleSchemaName, map[string]string{schemaName: schemaText}))
	if err != nil {
		return err
	}

	defer server.Stop()

	// Provide the OPA configuration which specifies fetching policy bundles from the mock server and logging decisions locally to the console
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
		},
		"decision_logs": {
			"console": false
		}
	}`, server.URL(), bundleSchemaName))

	// Create an instance of the OPA object
	opa, err := sdk.New(ctx, sdk.Options{
		Config: bytes.NewReader(config),
	})
	if err != nil {
		return err
	}

	defer opa.Stop(ctx)

	var result *sdk.DecisionResult
	// Get the named policy decision for the specified input
	if result, err = opa.Decision(ctx, sdk.DecisionOptions{
		Path:  "/validate/allow",
		Input: dataFromJson,
	}); err != nil {
		return err
	} else if decision, ok := result.Result.(bool); !ok || !decision {
	}

	fmt.Println(result.Result)
	return nil
}

// ValidateWithCue validates the data structure using the provided CUE document
// https://cuelang.org/docs/integrations/go/#processing-cue-in-go
func ValidateWithCue(data any, schemaName string, schemaText string) error {
	return errors.New("validation using CUE is not implemented yet")
}
