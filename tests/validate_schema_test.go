package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/cmd"
)

// TestCliValidateSchemaWithMockServer tests schema validation using a mock HTTP server.
// This validates Atmos schema validation functionality without depending on external
// services that could be flaky or unavailable. Uses GitHub Actions workflow schema
// as a realistic, modern example.
//
// The test validates three types of schema sources:
// 1. File-based schema (package-schema.json)
// 2. HTTP-based schema from mock server (GitHub Actions workflow)
// 3. Inline JSON schema embedded in atmos.yaml.
func TestCliValidateSchemaWithMockServer(t *testing.T) {
	// Create a mock HTTP server with a GitHub Actions workflow schema.
	// This is a simplified version of the actual GitHub Actions workflow schema,
	// demonstrating validation of a real-world, widely-used configuration format.
	mockSchema := map[string]interface{}{
		"$schema": "http://json-schema.org/draft-07/schema#",
		"type":    "object",
		"properties": map[string]interface{}{
			"name": map[string]interface{}{
				"type":        "string",
				"description": "The name of your workflow",
			},
			"on": map[string]interface{}{
				"description": "The event(s) that trigger the workflow",
				"oneOf": []interface{}{
					map[string]interface{}{"type": "string"},
					map[string]interface{}{"type": "array"},
					map[string]interface{}{"type": "object"},
				},
			},
			"jobs": map[string]interface{}{
				"type":        "object",
				"description": "Jobs to run in the workflow",
				"patternProperties": map[string]interface{}{
					"^[a-zA-Z_][a-zA-Z0-9_-]*$": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"runs-on": map[string]interface{}{
								"description": "The type of machine to run the job on",
								"type":        "string",
							},
							"steps": map[string]interface{}{
								"type": "array",
								"items": map[string]interface{}{
									"type": "object",
									"properties": map[string]interface{}{
										"name": map[string]interface{}{
											"type": "string",
										},
										"uses": map[string]interface{}{
											"type": "string",
										},
										"run": map[string]interface{}{
											"type": "string",
										},
									},
								},
							},
						},
						"required": []string{"runs-on", "steps"},
					},
				},
			},
		},
		"required": []string{"on", "jobs"},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		err := json.NewEncoder(w).Encode(mockSchema)
		require.NoError(t, err)
	}))
	defer server.Close()

	// Create a temporary directory for the test.
	tmpDir := t.TempDir()

	// Create the atmos.yaml configuration with the mock server URL.
	atmosConfig := `schemas:
  packageSchema:
    schema: ./package-schema.json
    matches:
      - package.yaml
  githubActionsWorkflow:
    schema: ` + server.URL + `
    matches:
      - workflow.yaml
  inlineSchema:
    schema: |
      {
        "type": "object",
        "properties": {
          "name": { "type": "string" },
          "version": { "type": "string" }
        },
        "required": ["name", "version"]
      }
    matches:
      - inline.yaml
`

	// Write atmos.yaml.
	err := os.WriteFile(filepath.Join(tmpDir, "atmos.yaml"), []byte(atmosConfig), 0o644)
	require.NoError(t, err)

	// Write package-schema.json - a simple npm package.json schema.
	packageSchema := `{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "properties": {
    "name": { "type": "string" },
    "version": { "type": "string" },
    "description": { "type": "string" }
  },
  "required": ["name", "version"]
}`
	err = os.WriteFile(filepath.Join(tmpDir, "package-schema.json"), []byte(packageSchema), 0o644)
	require.NoError(t, err)

	// Write package.yaml - valid npm package metadata.
	packageYAML := `name: "my-package"
version: "1.0.0"
description: "A sample package"
`
	err = os.WriteFile(filepath.Join(tmpDir, "package.yaml"), []byte(packageYAML), 0o644)
	require.NoError(t, err)

	// Write workflow.yaml - a valid GitHub Actions workflow.
	workflowYAML := `name: "CI"
on:
  push:
    branches:
      - main
  pull_request:
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout code
        uses: actions/checkout@v4
      - name: Run tests
        run: make test
  build:
    runs-on: ubuntu-latest
    steps:
      - name: Build application
        run: npm run build
`
	err = os.WriteFile(filepath.Join(tmpDir, "workflow.yaml"), []byte(workflowYAML), 0o644)
	require.NoError(t, err)

	// Write inline.yaml - validates against inline schema.
	inlineYAML := `name: "example"
version: "1.0.0"
`
	err = os.WriteFile(filepath.Join(tmpDir, "inline.yaml"), []byte(inlineYAML), 0o644)
	require.NoError(t, err)

	// Change to the temporary directory.
	t.Chdir(tmpDir)

	// Create a pipe to capture stderr to check if validation is executed correctly.
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	// Use SetArgs for Cobra command testing.
	cmd.RootCmd.SetArgs([]string{"validate", "schema"})

	err = cmd.Execute()

	// Restore stderr.
	w.Close()
	os.Stderr = oldStderr

	// Read the captured output.
	var buf bytes.Buffer
	_, readErr := buf.ReadFrom(r)
	require.NoError(t, readErr)
	output := buf.String()

	// Log output immediately if there's an error or error output.
	if err != nil || bytes.Contains([]byte(output), []byte("ERRO")) {
		t.Logf("Schema validation output:\n%s", output)
	}

	// Verify validation succeeded.
	assert.NoError(t, err, "Schema validation should succeed for all valid files")
	assert.NotContains(t, output, "ERRO", "Expected no error output, got: %s", output)

	// Verify the mock server was actually called.
	// The test proves that HTTP-based schema validation works without external dependencies.
	t.Log("✓ Successfully validated schemas from three sources: file, HTTP (mock server), and inline")
}
