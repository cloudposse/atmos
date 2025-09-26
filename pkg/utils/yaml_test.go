package utils

import (
	"net/http"
	"net/http/httptest"
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
	// Start a mock HTTP server to serve the remote file referenced in the test fixture
	// The fixture at stacks/deploy/nonprod.yaml includes: http://localhost:8080/stacks/deploy/nonprod.yaml
	remoteConfigPath := filepath.Join("..", "..", "tests", "fixtures", "scenarios", "remote-config")
	mockServer := httptest.NewServer(http.FileServer(http.Dir(remoteConfigPath)))
	defer mockServer.Close()

	// Set the environment variable so the code knows to replace localhost:8080 with the mock server URL
	os.Setenv("ATMOS_TEST_MOCK_SERVER_URL", mockServer.URL)
	defer os.Unsetenv("ATMOS_TEST_MOCK_SERVER_URL")

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
