package tags

import "testing"

// TestSelectorUnresolved covers detection of unresolved Go template markers
// and unprocessed Atmos YAML-function markers in metadata.tags/metadata.labels
// values of various shapes.
func TestSelectorUnresolved(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		v    any
		want bool
	}{
		{name: "nil", v: nil, want: false},
		{name: "plain_string", v: "prod", want: false},
		{name: "templated_string", v: "{{ .vars.stage }}", want: true},
		{name: "yaml_function_string", v: "!env DEPLOY_TIER", want: true},
		{name: "yaml_function_string_leading_space", v: "  !template '{a}'", want: true},
		{name: "bang_only_mid_string_is_plain", v: "not!afunction", want: false},
		{name: "plain_slice", v: []any{"prod", "team-a"}, want: false},
		{name: "templated_slice_element", v: []any{"prod", "{{ .vars.stage }}"}, want: true},
		{name: "yaml_function_slice_element", v: []any{"prod", "!env DEPLOY_TIER"}, want: true},
		{name: "plain_map", v: map[string]any{"env": "prod"}, want: false},
		{name: "templated_map_value", v: map[string]any{"env": "{{ .vars.stage }}"}, want: true},
		{name: "yaml_function_map_value", v: map[string]any{"env": "!store ssm env"}, want: true},
		{name: "non_string_scalar", v: 42, want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := SelectorUnresolved(tc.v); got != tc.want {
				t.Fatalf("SelectorUnresolved(%v) = %v, want %v", tc.v, got, tc.want)
			}
		})
	}
}
