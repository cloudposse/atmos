package provisioner

import (
	"bytes"
	"encoding/json"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/function/yamlsource"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
)

func inspectionStack(components map[string]any) map[string]any {
	return map[string]any{"dev": map[string]any{"components": map[string]any{"terraform": components}}}
}

func TestBackendInspectionOutput(t *testing.T) {
	for _, format := range []string{"table", "json", "yaml"} {
		t.Run(format, func(t *testing.T) {
			var output bytes.Buffer
			backendInspectionOutput(t, &output)
			stacks := inspectionStack(map[string]any{
				"z":        map[string]any{"backend_type": "custom", "backend": map[string]any{"value": "!exec never"}},
				"a":        map[string]any{"backend_type": "custom", "backend": map[string]any{"value": "!exec never"}},
				"abstract": map[string]any{"metadata": map[string]any{"type": "abstract"}, "backend_type": "s3"},
				"none":     map[string]any{},
			})
			opts := &BackendInspectionOptions{Stack: "dev", Format: format, DescribeStacks: func(_ *schema.AtmosConfiguration, stack string) (map[string]any, error) {
				require.Equal(t, "dev", stack)
				return stacks, nil
			}}
			require.NoError(t, ListBackends(&schema.AtmosConfiguration{}, opts))
			if format == "table" {
				require.Contains(t, output.String(), "COMPONENT")
				require.Contains(t, output.String(), "custom")
				require.NotContains(t, output.String(), "abstract")
			} else {
				var actual []BackendConfiguration
				decodeBackendInspection(t, format, output.Bytes(), &actual)
				require.Equal(t, []BackendConfiguration{
					{Stack: "dev", Component: "a", BackendType: "custom", Backend: map[string]any{"value": "!exec never"}},
					{Stack: "dev", Component: "z", BackendType: "custom", Backend: map[string]any{"value": "!exec never"}},
				}, actual)
			}
			first := output.String()
			output.Reset()
			require.NoError(t, ListBackends(&schema.AtmosConfiguration{}, opts))
			require.Equal(t, first, output.String())
			output.Reset()
			require.NoError(t, DescribeBackend(&schema.AtmosConfiguration{}, "a", opts))
			if format == "table" {
				require.Contains(t, output.String(), "CONFIGURATION")
				require.Contains(t, output.String(), "!exec never")
			} else {
				var actual BackendConfiguration
				decodeBackendInspection(t, format, output.Bytes(), &actual)
				require.Equal(t, BackendConfiguration{Stack: "dev", Component: "a", BackendType: "custom", Backend: map[string]any{"value": "!exec never"}}, actual)
			}
		})
	}
}

func TestBackendInspectionValidation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		stack  string
		format string
		want   error
	}{
		{"invalid format", "dev", "xml", errUtils.ErrInvalidFlagValue},
		{"missing stack", "", "json", errUtils.ErrRequiredFlagNotProvided},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opts := &BackendInspectionOptions{Stack: tc.stack, Format: tc.format, DescribeStacks: func(*schema.AtmosConfiguration, string) (map[string]any, error) {
				t.Fatal("invalid options must not load configuration")
				return nil, nil
			}}
			require.ErrorIs(t, ListBackends(nil, opts), tc.want)
			require.ErrorIs(t, DescribeBackend(nil, "component", opts), tc.want)
		})
	}
	require.ErrorIs(t, ListBackends(nil, nil), errUtils.ErrInvalidConfig)
	require.ErrorIs(t, ListBackends(nil, &BackendInspectionOptions{Stack: "dev", Format: "json"}), errUtils.ErrInvalidConfig)
	require.ErrorIs(t, DescribeBackend(nil, "", nil), errUtils.ErrComponentRequired)
}

func TestBackendInspectionSelections(t *testing.T) {
	var output bytes.Buffer
	backendInspectionOutput(t, &output)
	for _, tc := range []struct {
		name      string
		stacks    map[string]any
		component string
		want      error
	}{
		{"missing stack", nil, "", errUtils.ErrStackNotFound},
		{"missing component", inspectionStack(map[string]any{}), "missing", errUtils.ErrInvalidComponent},
		{"no backend", inspectionStack(map[string]any{"none": map[string]any{}}), "none", errUtils.ErrBackendNotFound},
		{"abstract", inspectionStack(map[string]any{"base": map[string]any{"metadata": map[string]any{"type": "abstract"}}}), "base", errUtils.ErrInvalidComponent},
		{"empty stack", inspectionStack(map[string]any{}), "", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			output.Reset()
			opts := &BackendInspectionOptions{Stack: "dev", Format: "json", DescribeStacks: func(*schema.AtmosConfiguration, string) (map[string]any, error) { return tc.stacks, nil }}
			var err error
			if tc.component == "" {
				err = ListBackends(&schema.AtmosConfiguration{}, opts)
			} else {
				err = DescribeBackend(&schema.AtmosConfiguration{}, tc.component, opts)
			}
			if tc.want != nil {
				require.ErrorIs(t, err, tc.want)
			} else {
				require.NoError(t, err)
				require.JSONEq(t, "[]", output.String())
			}
		})
	}
}

func TestBackendInspectionErrors(t *testing.T) {
	opts := &BackendInspectionOptions{Stack: "dev", Format: "json", DescribeStacks: func(*schema.AtmosConfiguration, string) (map[string]any, error) { return nil, io.ErrUnexpectedEOF }}
	require.ErrorIs(t, ListBackends(&schema.AtmosConfiguration{}, opts), io.ErrUnexpectedEOF)
	require.ErrorIs(t, DescribeBackend(&schema.AtmosConfiguration{}, "a", opts), io.ErrUnexpectedEOF)
	backendInspectionOutput(t, backendInspectionFailWriter{})
	for _, format := range []string{"table", "json", "yaml"} {
		opts.Format = format
		opts.DescribeStacks = func(*schema.AtmosConfiguration, string) (map[string]any, error) {
			return inspectionStack(map[string]any{"a": map[string]any{"backend_type": "local"}}), nil
		}
		require.ErrorIs(t, ListBackends(&schema.AtmosConfiguration{}, opts), io.ErrClosedPipe)
		require.ErrorIs(t, DescribeBackend(&schema.AtmosConfiguration{}, "a", opts), io.ErrClosedPipe)
	}
}

func decodeBackendInspection(t *testing.T, format string, value []byte, target any) {
	t.Helper()
	if format == "json" {
		require.NoError(t, json.Unmarshal(value, target))
	} else {
		require.NoError(t, yaml.Unmarshal(value, target))
	}
}

func backendInspectionOutput(t *testing.T, output io.Writer) {
	t.Helper()
	ctx, err := iolib.NewContext(iolib.WithStreams(&backendInspectionStreams{output: output}))
	require.NoError(t, err)
	data.InitWriter(ctx)
	t.Cleanup(data.Reset)
}

type backendInspectionStreams struct{ output io.Writer }

func (s *backendInspectionStreams) Input() io.Reader     { return nil }
func (s *backendInspectionStreams) Output() io.Writer    { return s.output }
func (s *backendInspectionStreams) Error() io.Writer     { return io.Discard }
func (s *backendInspectionStreams) RawOutput() io.Writer { return s.output }
func (s *backendInspectionStreams) RawError() io.Writer  { return io.Discard }

type backendInspectionFailWriter struct{}

func (backendInspectionFailWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestBackendInspectionAmbiguousComponent(t *testing.T) {
	stacks := map[string]any{}
	for _, stack := range []string{"prod", "dev"} {
		stacks[stack] = map[string]any{"components": map[string]any{"terraform": map[string]any{
			"app": map[string]any{"backend_type": "local"},
		}}}
	}
	result, err := collectBackendConfigurations(stacks, "")
	require.NoError(t, err)
	require.Equal(t, "dev", result[0].Stack)
	require.Equal(t, "prod", result[1].Stack)
	_, err = collectBackendConfigurations(stacks, "app")
	require.ErrorIs(t, err, errUtils.ErrInvalidComponent)
	require.Contains(t, err.Error(), "multiple logical stacks")
	stacks["prod"].(map[string]any)["components"].(map[string]any)["terraform"].(map[string]any)["app"] = map[string]any{}
	_, err = collectBackendConfigurations(stacks, "app")
	require.ErrorIs(t, err, errUtils.ErrInvalidComponent, "missing backend in one match must not hide component ambiguity")
}

func TestBackendInspectionNativeFunctionOutput(t *testing.T) {
	native := (yamlsource.Source{Tag: "!exec", Argument: "noexec", File: "stack.yaml", Line: 4}).Encode()
	include := (yamlsource.Source{Tag: "!include", Argument: "nofetch"}).Encode()
	backendType := (yamlsource.Source{Tag: "!env", Argument: "BACKEND"}).Encode()
	backends := []BackendConfiguration{{Stack: "dev", Component: "app", BackendType: backendType, Backend: map[string]any{
		"native":  native,
		"quoted":  "!exec noexec",
		"literal": "{{ external.value }}",
		"nested":  []any{map[string]any{"include": include}, native, "!exec noexec"},
	}}}
	original, err := json.Marshal(backends)
	require.NoError(t, err)
	for _, format := range []string{"json", "yaml", "table"} {
		for _, describe := range []bool{false, true} {
			name := "list/" + format
			if describe {
				name = "describe/" + format
			}
			t.Run(name, func(t *testing.T) {
				var output bytes.Buffer
				backendInspectionOutput(t, &output)
				require.NoError(t, writeBackendConfigurations(backends, format, describe))
				first := output.String()
				require.NotContains(t, first, "!atmos-yaml-source:")
				switch format {
				case "json":
					assertBackendNativeJSON(t, output.Bytes(), describe)
				case "yaml":
					assertBackendNativeYAML(t, output.Bytes(), describe)
				default:
					require.Contains(t, first, "!env BACKEND")
					if describe {
						require.Contains(t, first, "!exec noexec")
						require.Contains(t, first, "!include nofetch")
					}
				}
				output.Reset()
				require.NoError(t, writeBackendConfigurations(backends, format, describe))
				require.Equal(t, first, output.String())
				after, err := json.Marshal(backends)
				require.NoError(t, err)
				require.Equal(t, original, after, "output must not alter stored native function identities")
			})
		}
	}
}

func assertBackendNativeJSON(t *testing.T, output []byte, describe bool) {
	t.Helper()
	var item BackendConfiguration
	if describe {
		require.NoError(t, json.Unmarshal(output, &item))
	} else {
		var items []BackendConfiguration
		require.NoError(t, json.Unmarshal(output, &items))
		require.Len(t, items, 1)
		item = items[0]
	}
	require.Equal(t, "!env BACKEND", item.BackendType)
	require.Equal(t, "!exec noexec", item.Backend["native"])
	require.Equal(t, "!exec noexec", item.Backend["quoted"])
	require.Equal(t, "{{ external.value }}", item.Backend["literal"])
	require.Equal(t, []any{map[string]any{"include": "!include nofetch"}, "!exec noexec", "!exec noexec"}, item.Backend["nested"])
}

func assertBackendNativeYAML(t *testing.T, output []byte, describe bool) {
	t.Helper()
	var doc yaml.Node
	require.NoError(t, yaml.Unmarshal(output, &doc))
	item := doc.Content[0]
	if !describe {
		require.Equal(t, yaml.SequenceNode, item.Kind)
		require.Len(t, item.Content, 1)
		item = item.Content[0]
	}
	backendType := backendYAMLField(t, item, "backend_type")
	require.Equal(t, "!env", backendType.Tag)
	require.Equal(t, "BACKEND", backendType.Value)
	backend := backendYAMLField(t, item, "backend")
	native := backendYAMLField(t, backend, "native")
	require.Equal(t, "!exec", native.Tag)
	require.Equal(t, "noexec", native.Value)
	quoted := backendYAMLField(t, backend, "quoted")
	require.Equal(t, "!!str", quoted.Tag)
	require.Equal(t, "!exec noexec", quoted.Value)
	literal := backendYAMLField(t, backend, "literal")
	require.Equal(t, "!!str", literal.Tag)
	require.Equal(t, "{{ external.value }}", literal.Value)
	nested := backendYAMLField(t, backend, "nested")
	require.Len(t, nested.Content, 3)
	include := backendYAMLField(t, nested.Content[0], "include")
	require.Equal(t, "!include", include.Tag)
	require.Equal(t, "nofetch", include.Value)
	require.Equal(t, "!exec", nested.Content[1].Tag)
	require.Equal(t, "!!str", nested.Content[2].Tag)
}

func backendYAMLField(t *testing.T, node *yaml.Node, key string) *yaml.Node {
	t.Helper()
	require.Equal(t, yaml.MappingNode, node.Kind)
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	t.Fatalf("missing YAML field %q", key)
	return nil
}
