package utils

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"text/template"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/github"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/tests/testhelpers/httpmock"
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
	// Mock rate limit check to always pass (no waiting).
	// This avoids the 24+ minute wait when GitHub rate limit is exhausted on CI.
	oldWaiter := github.RateLimitWaiter
	github.RateLimitWaiter = func(ctx context.Context, minRemaining int) error {
		return nil // Always pass, never wait.
	}
	t.Cleanup(func() { github.RateLimitWaiter = oldWaiter })

	// Create mock server to intercept GitHub requests.
	// This avoids network dependencies and GitHub rate limiting in CI.
	mock := httpmock.NewGitHubMockServer(t)

	// Register the remote file content that the fixture expects.
	// The fixture includes the remote component settings through the local mock URL.
	mock.RegisterFile("stack-templates-2/stacks/deploy/nonprod.yaml", `
components:
  terraform:
    component-1:
      settings:
        config:
          a: component-1-a
          b: component-1-b
          c: component-1-c
`)

	// Inject mock HTTP client for go-getter.
	// go-getter uses cleanhttp.DefaultClient() which doesn't respect http.DefaultTransport,
	// so we need to inject the client directly.
	oldClient := TestHTTPClient
	TestHTTPClient = mock.HTTPClient()
	t.Cleanup(func() { TestHTTPClient = oldClient })

	stacksPath := filepath.Join("..", "..", "tests", "fixtures", "scenarios", "atmos-include-yaml-function")
	file := filepath.Join(stacksPath, "stacks", "deploy", "nonprod.yaml.tmpl")

	yamlFileContent, err := os.ReadFile(file)
	require.NoError(t, err)

	// Unmarshalling starts after stack templates have been rendered.
	t.Setenv("ATMOS_TEST_GITHUB_MOCK_URL", mock.URL())
	stackTemplate, err := template.New(filepath.Base(file)).Funcs(template.FuncMap{"env": os.Getenv}).Parse(string(yamlFileContent))
	require.NoError(t, err)
	var rendered bytes.Buffer
	require.NoError(t, stackTemplate.Execute(&rendered, nil))

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: stacksPath,
		Logs: schema.Logs{
			Level: "Info",
		},
	}

	manifest, err := UnmarshalYAMLFromFile[schema.AtmosSectionMapType](atmosConfig, rendered.String(), file)
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
