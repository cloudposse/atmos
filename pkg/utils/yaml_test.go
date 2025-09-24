package utils

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestYAMLToMapOfInterfaces(t *testing.T) {
	input := `---
hello: world`
	result, err := UnmarshalYAML[map[any]any](input)
	assert.Nil(t, err)
	assert.Equal(t, result["hello"], "world")
}

func TestYAMLToMapOfInterfacesRedPath(t *testing.T) {
	input := "Not YAML"
	_, err := UnmarshalYAML[map[any]any](input)
	assert.NotNil(t, err)
}

func TestUnmarshalYAMLFromFile(t *testing.T) {
	// This test requires a mock HTTP server to serve the remote file referenced in the test fixture.
	// The fixture at stacks/deploy/nonprod.yaml includes: http://localhost:8080/stacks/deploy/nonprod.yaml
	// Since this is a unit test without the CLI test infrastructure, we skip it.
	// The functionality is tested in the CLI integration tests which have the mock server.
	t.Skipf("Skipping test: requires mock HTTP server infrastructure from CLI tests")

	stacksPath := filepath.Join("..", "..", "tests", "fixtures", "scenarios", "atmos-include-yaml-function")
	file := filepath.Join(stacksPath, "stacks", "deploy", "nonprod.yaml")

	yamlFileContent, err := os.ReadFile(file)
	assert.Nil(t, err)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: stacksPath,
		Logs: schema.Logs{
			Level: "Info",
		},
	}

	manifest, err := UnmarshalYAMLFromFile[schema.AtmosSectionMapType](atmosConfig, string(yamlFileContent), file)
	assert.Nil(t, err)

	expected := `components:
    terraform:
        component-1:
            metadata:
                component: yaml-functions
            vars:
                boolean_var: true
                list_var:
                    - a
                    - b
                    - c
                map_var:
                    a: 1
                    b: 2
                    c: 3
                string_var: abc
        component-2:
            metadata:
                component: yaml-functions
            vars:
                boolean_var: true
                list_var:
                    - a
                    - b
                    - c
                map_var:
                    a: 1
                    b: 2
                    c: 3
                string_var: abc
        component-3:
            metadata:
                component: yaml-functions
            vars:
                boolean_var: true
                list_var:
                    - a
                    - b
                    - c
                map_var:
                    a: 1
                    b: 2
                    c: 3
                string_var: abc
        component-4:
            metadata:
                component: yaml-functions
            vars:
                boolean_var: true
                list_var:
                    - a
                    - b
                    - c
                map_var:
                    a: 1
                    b: 2
                    c: 3
                string_var: abc
import:
    - mixins/stage/nonprod
settings:
    config:
        a: component-1-a
        b: component-1-b
        c: component-1-c
`

	actual, err := ConvertToYAML(manifest, YAMLOptions{Indent: 4})
	assert.Nil(t, err)
	assert.Equal(t, expected, actual)
}
