package rc

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRender_ProviderInstallationOrderedMethods(t *testing.T) {
	// provider_installation is a list of single-key method maps. List order is
	// precedence and must be preserved: network_mirror then direct.
	rc := map[string]any{
		"provider_installation": []any{
			map[string]any{"network_mirror": map[string]any{"url": "https://mirror.example.com/"}},
			map[string]any{"direct": map[string]any{"exclude": []any{"registry.terraform.io/hashicorp/*"}}},
		},
	}

	out, err := Render(rc)
	require.NoError(t, err)
	s := string(out)

	assert.Contains(t, s, "provider_installation {")
	assert.Contains(t, s, "network_mirror {")
	assert.Contains(t, s, `url = "https://mirror.example.com/"`)
	assert.Contains(t, s, "direct {")
	assert.Contains(t, s, `exclude = ["registry.terraform.io/hashicorp/*"]`)

	// network_mirror must come before direct (precedence ordering preserved).
	assert.Less(t, strings.Index(s, "network_mirror {"), strings.Index(s, "direct {"))
}

func TestRender_EmptyMethodBlock(t *testing.T) {
	// `direct: {}` (or null) renders an empty block, not an attribute.
	rc := map[string]any{
		"provider_installation": []any{
			map[string]any{"network_mirror": map[string]any{"url": "http://127.0.0.1:5000/"}},
			map[string]any{"direct": nil},
		},
	}

	out, err := Render(rc)
	require.NoError(t, err)
	s := string(out)
	assert.Contains(t, s, "direct {")
	assert.NotContains(t, s, "direct =")
}

func TestRender_LabeledHostBlock(t *testing.T) {
	rc := map[string]any{
		"host": map[string]any{
			"registry.terraform.io": map[string]any{
				"services": map[string]any{
					"modules.v1": "https://modules.example.com/v1/modules/",
				},
			},
		},
	}

	out, err := Render(rc)
	require.NoError(t, err)
	s := string(out)
	assert.Contains(t, s, `host "registry.terraform.io" {`)
	assert.Contains(t, s, `"modules.v1" = "https://modules.example.com/v1/modules/"`)
}

func TestRender_CredentialsBlock(t *testing.T) {
	rc := map[string]any{
		"credentials": map[string]any{
			"app.terraform.io": map[string]any{"token": "xxxx"},
		},
	}
	out, err := Render(rc)
	require.NoError(t, err)
	s := string(out)
	assert.Contains(t, s, `credentials "app.terraform.io" {`)
	assert.Contains(t, s, `token = "xxxx"`)
}

func TestRender_TopLevelScalars(t *testing.T) {
	rc := map[string]any{
		"plugin_cache_dir":   "/home/user/.terraform.d/plugin-cache",
		"disable_checkpoint": true,
	}
	out, err := Render(rc)
	require.NoError(t, err)
	// hclwrite aligns '=' with padding when multiple attributes are present, so
	// normalize runs of spaces before asserting.
	s := strings.Join(strings.Fields(string(out)), " ")
	assert.Contains(t, s, `plugin_cache_dir = "/home/user/.terraform.d/plugin-cache"`)
	assert.Contains(t, s, "disable_checkpoint = true")
}

func TestRender_Deterministic(t *testing.T) {
	rc := map[string]any{
		"plugin_cache_dir":   "/cache",
		"disable_checkpoint": true,
		"host": map[string]any{
			"registry.terraform.io": map[string]any{
				"services": map[string]any{"modules.v1": "https://m/"},
			},
		},
	}
	first, err := Render(rc)
	require.NoError(t, err)
	for i := 0; i < 5; i++ {
		again, err := Render(rc)
		require.NoError(t, err)
		assert.Equal(t, string(first), string(again), "Render must be deterministic")
	}
}

func TestRender_CredentialsHelperBlock(t *testing.T) {
	// credentials_helper is a labeled block keyed by helper name.
	rc := map[string]any{
		"credentials_helper": map[string]any{
			"osascript": map[string]any{
				"args": []any{"--mode", "store"},
			},
		},
	}
	out, err := Render(rc)
	require.NoError(t, err)
	s := string(out)
	assert.Contains(t, s, `credentials_helper "osascript" {`)
	assert.Contains(t, s, `args = ["--mode", "store"]`)
}

func TestRender_UnknownMapDirective(t *testing.T) {
	// A map directive that is not provider_installation or a labeled block type
	// renders as a best-effort unlabeled block of attributes (forward-compatible).
	rc := map[string]any{
		"some_future_block": map[string]any{
			"enabled": true,
			"name":    "x",
		},
	}
	out, err := Render(rc)
	require.NoError(t, err)
	s := string(out)
	assert.Contains(t, s, "some_future_block {")
	assert.Contains(t, s, "enabled = true")
	assert.Contains(t, s, `name    = "x"`)
}

func TestRender_ProviderInstallationBareMap(t *testing.T) {
	// provider_installation accepts a bare map of methods (not only a list).
	rc := map[string]any{
		"provider_installation": map[string]any{
			"direct": map[string]any{"exclude": []any{"registry.terraform.io/hashicorp/*"}},
		},
	}
	out, err := Render(rc)
	require.NoError(t, err)
	s := string(out)
	assert.Contains(t, s, "provider_installation {")
	assert.Contains(t, s, "direct {")
	assert.Contains(t, s, `exclude = ["registry.terraform.io/hashicorp/*"]`)
}

func TestRender_ProviderInstallationNilAndEmpty(t *testing.T) {
	// A nil or empty provider_installation renders an empty block, not an error.
	for _, tt := range []struct {
		name  string
		value any
	}{
		{name: "nil", value: nil},
		{name: "empty list", value: []any{}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			out, err := Render(map[string]any{"provider_installation": tt.value})
			require.NoError(t, err)
			assert.Contains(t, string(out), "provider_installation {")
		})
	}
}

func TestRender_NestedStructures(t *testing.T) {
	// Exercise toCty/sliceToCty/mapToCty: an object holding a list and a nested object.
	rc := map[string]any{
		"host": map[string]any{
			"registry.terraform.io": map[string]any{
				"list_attr":   []any{"a", "b"},
				"nested_attr": map[string]any{"k": "v"},
			},
		},
	}
	out, err := Render(rc)
	require.NoError(t, err)
	// Normalize hclwrite alignment padding before asserting.
	s := strings.Join(strings.Fields(string(out)), " ")
	assert.Contains(t, s, `list_attr = ["a", "b"]`)
	assert.Contains(t, s, "nested_attr = {")
	assert.Contains(t, s, `k = "v"`)
}

func TestRender_MapAnyAnyShape(t *testing.T) {
	// Some YAML decoders produce map[any]any; the renderer must coerce it through
	// asStringMap/toCty at every level (labeled block, nested object).
	rc := map[string]any{
		"host": map[any]any{
			"registry.terraform.io": map[any]any{
				"services": map[any]any{"modules.v1": "https://m/"},
			},
		},
	}
	out, err := Render(rc)
	require.NoError(t, err)
	s := string(out)
	assert.Contains(t, s, `host "registry.terraform.io" {`)
	assert.Contains(t, s, `"modules.v1" = "https://m/"`)
}

func TestRender_ScalarTypes(t *testing.T) {
	// Cover every scalarToCty branch via top-level attributes.
	rc := map[string]any{
		"int_attr":    5,
		"int64_attr":  int64(9000000000),
		"float_attr":  1.5,
		"bool_attr":   false,
		"string_attr": "hello",
		"null_attr":   nil,
	}
	out, err := Render(rc)
	require.NoError(t, err)
	// Normalize hclwrite '=' alignment padding before asserting.
	s := strings.Join(strings.Fields(string(out)), " ")
	assert.Contains(t, s, "int_attr = 5")
	assert.Contains(t, s, "int64_attr = 9000000000")
	assert.Contains(t, s, "float_attr = 1.5")
	assert.Contains(t, s, "bool_attr = false")
	assert.Contains(t, s, `string_attr = "hello"`)
	assert.Contains(t, s, "null_attr = null")
}

func TestRender_EmptyAndNilCollections(t *testing.T) {
	// Exercises asStringMap(nil), sliceToCty(empty)→EmptyTupleVal, and
	// mapToCty(empty)→EmptyObjectVal through nested labeled-block attributes.
	rc := map[string]any{
		"host": map[string]any{
			"nil-body": nil, // asStringMap(nil) → empty block body.
			"registry.terraform.io": map[string]any{
				"empty_list": []any{},
				"empty_obj":  map[string]any{},
			},
		},
	}
	out, err := Render(rc)
	require.NoError(t, err)
	s := strings.Join(strings.Fields(string(out)), " ")
	assert.Contains(t, s, `host "nil-body" { }`)
	assert.Contains(t, s, "empty_list = []")
	assert.Contains(t, s, "empty_obj = {}")
}

func TestRender_NestedErrorPropagation(t *testing.T) {
	// Every case threads an unsupported scalar (struct{}) or a bad type through a
	// distinct nesting path so the error surfaces from the corresponding helper.
	bad := struct{}{}
	tests := []struct {
		name string
		rc   map[string]any
	}{
		{
			name: "provider_installation list element not a map",
			rc:   map[string]any{"provider_installation": []any{"not-a-map"}},
		},
		{
			name: "provider_installation method body not a map",
			rc:   map[string]any{"provider_installation": []any{map[string]any{"direct": "not-a-map"}}},
		},
		{
			name: "provider_installation method attribute unsupported",
			rc:   map[string]any{"provider_installation": []any{map[string]any{"direct": map[string]any{"k": bad}}}},
		},
		{
			name: "labeled block value not a map",
			rc:   map[string]any{"host": "not-a-map"},
		},
		{
			name: "labeled block attribute unsupported",
			rc:   map[string]any{"host": map[string]any{"r": map[string]any{"k": bad}}},
		},
		{
			name: "list attribute element unsupported",
			rc:   map[string]any{"host": map[string]any{"r": map[string]any{"l": []any{bad}}}},
		},
		{
			name: "nested map attribute unsupported",
			rc:   map[string]any{"host": map[string]any{"r": map[string]any{"m": map[string]any{"k": bad}}}},
		},
		{
			name: "unknown block attribute unsupported",
			rc:   map[string]any{"some_future_block": map[string]any{"k": bad}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Render(tt.rc)
			require.Error(t, err)
		})
	}
}

func TestRender_Errors(t *testing.T) {
	tests := []struct {
		name string
		rc   map[string]any
	}{
		{
			name: "provider_installation wrong type",
			rc:   map[string]any{"provider_installation": "not-a-list"},
		},
		{
			name: "labeled block value not a map",
			rc:   map[string]any{"host": map[string]any{"x": "not-a-map"}},
		},
		{
			name: "unsupported scalar type",
			rc:   map[string]any{"plugin_cache_dir": struct{}{}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Render(tt.rc)
			assert.Error(t, err)
		})
	}
}
