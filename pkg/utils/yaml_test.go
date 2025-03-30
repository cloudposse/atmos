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

	actual, err := ConvertToYAML(manifest)
	assert.Nil(t, err)
	assert.Equal(t, expected, actual)
}
