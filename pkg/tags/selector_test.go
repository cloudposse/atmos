package tags

import (
	"errors"
	"strings"
	"testing"

	errUtils "github.com/cloudposse/atmos/errors"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// TestSelectorUnresolved covers detection of unresolved Go template markers
// and unprocessed Atmos YAML-function markers in metadata.tags/metadata.labels
// values of various shapes.
func TestSelectorUnresolved(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		v         any
		leftDelim string
		want      bool
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
		{name: "custom_delims_detected", v: "[[ .vars.stage ]]", leftDelim: "[[", want: true},
		{name: "custom_delims_default_marker_is_plain", v: "{{ .vars.stage }}", leftDelim: "[[", want: false},
		{name: "empty_delim_falls_back_to_default", v: "{{ .vars.stage }}", leftDelim: "", want: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := SelectorUnresolved(tc.v, tc.leftDelim); got != tc.want {
				t.Fatalf("SelectorUnresolved(%v, %q) = %v, want %v", tc.v, tc.leftDelim, got, tc.want)
			}
		})
	}
}

// TestValidateSelectorValue covers the selector purity contract: forbidden
// YAML functions and forbidden template calls error; plain strings, simple
// templates, and cheap local functions pass.
func TestValidateSelectorValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		v          any
		leftDelim  string
		rightDelim string
		wantErr    bool
	}{
		{name: "nil", v: nil, wantErr: false},
		{name: "plain_string", v: "prod", wantErr: false},
		{name: "simple_template", v: "{{ .vars.stage }}", wantErr: false},
		{name: "sprig_template", v: "{{ .vars.stage | upper }}", wantErr: false},
		{name: "env_function_allowed", v: "!env DEPLOY_TIER", wantErr: false},
		{name: "git_function_allowed", v: "!git.branch", wantErr: false},
		{name: "include_function_allowed", v: "!include ./tags.yaml", wantErr: false},
		{name: "literal_function_allowed", v: "!literal prod", wantErr: false},
		{name: "unknown_function_allowed", v: "!important", wantErr: false},
		{name: "exec_rejected", v: "!exec ./get-tier.sh", wantErr: true},
		{name: "secret_rejected", v: "!secret op://vault/item", wantErr: true},
		{name: "store_rejected", v: "!store ssm env", wantErr: true},
		{name: "store_get_rejected", v: "!store.get ssm env", wantErr: true},
		{name: "terraform_output_rejected", v: "!terraform.output vpc dev id", wantErr: true},
		{name: "terraform_state_rejected", v: "!terraform.state vpc dev id", wantErr: true},
		{name: "aws_account_id_rejected", v: "!aws.account_id", wantErr: true},
		{name: "emulator_rejected", v: "!emulator aws endpoint", wantErr: true},
		{name: "random_rejected", v: "!random uuid", wantErr: true},
		{name: "leading_space_still_rejected", v: "  !store ssm env", wantErr: true},
		{name: "atmos_component_call_rejected", v: `{{ (atmos.Component "vpc" .stack).outputs.id }}`, wantErr: true},
		{name: "atmos_store_call_rejected", v: `{{ atmos.Store "ssm" .stack "vpc" "id" }}`, wantErr: true},
		{name: "atmos_gomplate_datasource_rejected", v: `{{ atmos.GomplateDatasource "config" }}`, wantErr: true},
		{name: "ds_call_rejected", v: `{{ ds "config" }}`, wantErr: true},
		{name: "datasource_call_rejected", v: `{{ (datasource "config").tier }}`, wantErr: true},
		{name: "ds_in_if_rejected", v: `{{ if ds "config" }}a{{ else }}b{{ end }}`, wantErr: true},
		{name: "ds_as_field_name_allowed", v: "{{ .vars.ds }}", wantErr: false},
		{name: "atmos_component_word_without_template_allowed", v: "see atmos.Component docs", wantErr: false},
		{name: "unparseable_template_allowed", v: "{{ not a template", wantErr: false},
		{name: "forbidden_in_slice", v: []any{"prod", "!store ssm env"}, wantErr: true},
		{name: "forbidden_in_map", v: map[string]any{"env": "!exec ./x.sh"}, wantErr: true},
		{name: "allowed_map", v: map[string]any{"env": "prod", "tier": "{{ .vars.tier }}"}, wantErr: false},
		{
			name:       "custom_delims_forbidden_call_rejected",
			v:          `[[ ds "config" ]]`,
			leftDelim:  "[[",
			rightDelim: "]]",
			wantErr:    true,
		},
		{
			name:       "custom_delims_default_marker_is_plain_text",
			v:          `{{ ds "config" }}`,
			leftDelim:  "[[",
			rightDelim: "]]",
			wantErr:    false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateSelectorValue("metadata.tags", tc.v, tc.leftDelim, tc.rightDelim)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ValidateSelectorValue(%v) = nil, want ErrForbiddenSelectorFunction", tc.v)
				}
				if !errors.Is(err, errUtils.ErrForbiddenSelectorFunction) {
					t.Fatalf("ValidateSelectorValue(%v) = %v, want ErrForbiddenSelectorFunction", tc.v, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ValidateSelectorValue(%v) = %v, want nil", tc.v, err)
			}
		})
	}
}

// TestValidateSelectorValueDeterministicMapOrder asserts the first offending
// key in sorted order is reported, keeping errors stable across runs.
func TestValidateSelectorValueDeterministicMapOrder(t *testing.T) {
	t.Parallel()

	v := map[string]any{
		"zeta":  "!exec ./z.sh",
		"alpha": "!store ssm a",
	}
	for range 10 {
		err := ValidateSelectorValue("metadata.labels", v, "", "")
		if err == nil {
			t.Fatal("expected error")
		}
		if got := err.Error(); !strings.Contains(got, "!store") {
			t.Fatalf("expected error for sorted-first key value %q, got: %v", "!store", got)
		}
	}
}

// TestForbiddenSelectorFunctionsMatchConstants cross-checks the literal
// deny-list in this package against the canonical AtmosYamlFunc* constants in
// pkg/utils, so a renamed function constant fails this test instead of
// silently drifting.
func TestForbiddenSelectorFunctionsMatchConstants(t *testing.T) {
	t.Parallel()

	expected := []string{
		u.AtmosYamlFuncExec,
		u.AtmosYamlFuncSecret,
		u.AtmosYamlFuncStore,
		u.AtmosYamlFuncStoreGet,
		u.AtmosYamlFuncTerraformOutput,
		u.AtmosYamlFuncTerraformState,
		u.AtmosYamlFuncAwsAccountID,
		u.AtmosYamlFuncAwsCallerIdentityArn,
		u.AtmosYamlFuncAwsCallerIdentityUserID,
		u.AtmosYamlFuncAwsRegion,
		u.AtmosYamlFuncAwsOrganizationID,
		u.AtmosYamlFuncEmulator,
		u.AtmosYamlFuncRandom,
	}
	if len(expected) != len(forbiddenSelectorFunctions) {
		t.Fatalf("deny-list has %d entries, constants list has %d", len(forbiddenSelectorFunctions), len(expected))
	}
	for _, name := range expected {
		if _, ok := forbiddenSelectorFunctions[name]; !ok {
			t.Fatalf("deny-list is missing %q", name)
		}
	}
}
