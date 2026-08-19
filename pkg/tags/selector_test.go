package tags

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// TestTemplateDelims covers the effective-delimiter fallback: a well-formed
// two-element, non-empty pair is honored; anything else (wrong length, or an
// empty component) falls back to the standard Go delimiters.
func TestTemplateDelims(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		delims    []string
		wantLeft  string
		wantRight string
	}{
		{name: "nil_falls_back_to_default", delims: nil, wantLeft: DefaultLeftDelim, wantRight: DefaultRightDelim},
		{name: "custom_pair_honored", delims: []string{"[[", "]]"}, wantLeft: "[[", wantRight: "]]"},
		{name: "wrong_length_falls_back_to_default", delims: []string{"[["}, wantLeft: DefaultLeftDelim, wantRight: DefaultRightDelim},
		{name: "empty_left_falls_back_to_default", delims: []string{"", "]]"}, wantLeft: DefaultLeftDelim, wantRight: DefaultRightDelim},
		{name: "empty_right_falls_back_to_default", delims: []string{"[[", ""}, wantLeft: DefaultLeftDelim, wantRight: DefaultRightDelim},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			left, right := TemplateDelims(tc.delims)
			if left != tc.wantLeft || right != tc.wantRight {
				t.Fatalf("TemplateDelims(%v) = (%q, %q), want (%q, %q)", tc.delims, left, right, tc.wantLeft, tc.wantRight)
			}
		})
	}
}

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
		{name: "templated_map_key", v: map[string]any{"{{ .vars.env }}": "true"}, want: true},
		{name: "yaml_function_map_key", v: map[string]any{"!env KEY": "true"}, want: true},
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
		{name: "with_scoped_component_rejected", v: `{{ with atmos }}{{ .Component "vpc" "dev" }}{{ end }}`, wantErr: true},
		{name: "variable_alias_component_rejected", v: `{{ $x := atmos }}{{ $x.Component "vpc" "dev" }}`, wantErr: true},
		{name: "range_scoped_store_rejected", v: `{{ range .items }}{{ .Store "ssm" "dev" "vpc" "id" }}{{ end }}`, wantErr: true},
		{name: "paren_chain_component_rejected", v: `{{ $x := atmos }}{{ ($x).Component "vpc" }}`, wantErr: true},
		{name: "ds_as_field_name_allowed", v: "{{ .vars.ds }}", wantErr: false},
		{name: "atmos_component_word_without_template_allowed", v: "see atmos.Component docs", wantErr: false},
		{name: "unparseable_template_allowed", v: "{{ not a template", wantErr: false},
		{name: "forbidden_in_slice", v: []any{"prod", "!store ssm env"}, wantErr: true},
		{name: "forbidden_in_map", v: map[string]any{"env": "!exec ./x.sh"}, wantErr: true},
		{name: "allowed_map", v: map[string]any{"env": "prod", "tier": "{{ .vars.tier }}"}, wantErr: false},
		{name: "template_reference_without_pipeline_allowed", v: `{{ template "x" }}`, wantErr: false},
		{name: "forbidden_call_in_else_branch_rejected", v: `{{ if .ok }}fine{{ else }}{{ ds "config" }}{{ end }}`, wantErr: true},
		{name: "if_without_else_allowed", v: `{{ if .cond }}fine{{ end }}`, wantErr: false},
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
		// atmos.Resolve would launder any YAML function through a template call.
		{name: "atmos_resolve_rejected", v: `{{ atmos.Resolve "!store ssm env" }}`, wantErr: true},
		{name: "atmos_resolve_of_allowed_function_still_rejected", v: `{{ atmos.Resolve "!env TIER" }}`, wantErr: true},
		// Gomplate datasource access beyond ds/datasource.
		{name: "gomplate_include_rejected", v: `{{ include "config" }}`, wantErr: true},
		{name: "gomplate_datasource_exists_rejected", v: `{{ if datasourceExists "config" }}a{{ end }}`, wantErr: true},
		{name: "gomplate_datasource_reachable_rejected", v: `{{ if datasourceReachable "config" }}a{{ end }}`, wantErr: true},
		{name: "gomplate_define_datasource_rejected", v: `{{ defineDatasource "cfg" "s3://b/k" }}`, wantErr: true},
		// Gomplate auth/network/nondeterministic namespaces.
		{name: "gomplate_aws_namespace_rejected", v: `{{ aws.EC2Tag "Name" }}`, wantErr: true},
		{name: "gomplate_gcp_namespace_rejected", v: `{{ gcp.Meta "instance" }}`, wantErr: true},
		{name: "gomplate_net_namespace_rejected", v: `{{ net.LookupIP "example.com" }}`, wantErr: true},
		{name: "gomplate_random_namespace_rejected", v: `{{ random.ASCII 5 }}`, wantErr: true},
		// Sprig network and nondeterministic functions: they would resolve to
		// a different value in the early gate than in full evaluation.
		{name: "sprig_get_host_by_name_rejected", v: `{{ getHostByName "example.com" }}`, wantErr: true},
		{name: "sprig_uuidv4_rejected", v: `{{ uuidv4 }}`, wantErr: true},
		{name: "sprig_rand_alpha_rejected", v: `{{ randAlpha 5 }}`, wantErr: true},
		{name: "sprig_rand_alpha_num_rejected", v: `{{ randAlphaNum 5 }}`, wantErr: true},
		{name: "sprig_rand_numeric_rejected", v: `{{ randNumeric 4 }}`, wantErr: true},
		{name: "sprig_rand_ascii_rejected", v: `{{ randAscii 5 }}`, wantErr: true},
		{name: "sprig_rand_bytes_rejected", v: `{{ randBytes 16 }}`, wantErr: true},
		{name: "sprig_now_rejected", v: `{{ now }}`, wantErr: true},
		// Namespace/function names as data fields stay allowed (only bare
		// identifiers are function calls).
		{name: "aws_as_field_name_allowed", v: "{{ .vars.aws }}", wantErr: false},
		{name: "now_as_field_name_allowed", v: "{{ .vars.now }}", wantErr: false},
		{name: "include_as_field_name_allowed", v: "{{ .vars.include }}", wantErr: false},
		// Documented conservatism: a capitalized field named like a forbidden
		// atmos method is rejected regardless of binding.
		{name: "component_as_field_name_rejected_by_design", v: "{{ .vars.Component }}", wantErr: true},
		// Map KEYS are rendered like values and must obey the same contract.
		{name: "forbidden_template_in_map_key", v: map[string]any{`{{ atmos.Store "ssm" .stack "vpc" "id" }}`: "x"}, wantErr: true},
		{name: "forbidden_function_in_map_key", v: map[string]any{"!store ssm owner": "x"}, wantErr: true},
		{name: "templated_map_key_allowed_when_pure", v: map[string]any{"{{ .vars.env }}": "true"}, wantErr: false},
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

// TestForbiddenSelectorFunctionsCompleteness guards the selector purity
// contract against drift: every YAML function registered in utils.AtmosYamlTags
// must be explicitly classified as either forbidden in selectors (requires
// authentication, process execution, or nondeterminism) or selector-safe.
// Adding a new YAML function without classifying it here fails this test,
// forcing a deliberate purity decision instead of a silent default-allow.
func TestForbiddenSelectorFunctionsCompleteness(t *testing.T) {
	t.Parallel()

	// Functions allowed to appear in metadata.tags/metadata.labels values:
	// resolvable without authentication, subprocesses, or nondeterminism.
	// They still resolve only during full evaluation (SelectorUnresolved
	// reports them as undecidable, deferring the scope decision), but their
	// presence is not a purity violation.
	selectorSafe := map[string]struct{}{
		u.AtmosYamlFuncTemplate:      {},
		u.AtmosYamlFuncEnv:           {},
		u.AtmosYamlFuncCEL:           {},
		u.AtmosYamlFuncGitRoot:       {},
		u.AtmosYamlFuncGitRootAlias:  {},
		u.AtmosYamlFuncGitSha:        {},
		u.AtmosYamlFuncGitBranch:     {},
		u.AtmosYamlFuncGitRef:        {},
		u.AtmosYamlFuncGitRepository: {},
		u.AtmosYamlFuncGitOwner:      {},
		u.AtmosYamlFuncGitName:       {},
		u.AtmosYamlFuncGitHost:       {},
		u.AtmosYamlFuncGitUrl:        {},
		u.AtmosYamlFuncAppend:        {},
		u.AtmosYamlFuncCwd:           {},
		u.AtmosYamlFuncUnset:         {},
		u.AtmosYamlFuncLiteral:       {},
		u.AtmosYamlFuncVersion:       {},
		u.AtmosYamlFuncTags:          {},
		u.AtmosYamlFuncLabels:        {},
		u.AtmosYamlFuncLabelsKeys:    {},
		u.AtmosYamlFuncLabelsValues:  {},
	}

	require.NotEmpty(t, u.AtmosYamlTags, "utils.AtmosYamlTags must list the registered YAML functions")
	for _, name := range u.AtmosYamlTags {
		_, forbidden := forbiddenSelectorFunctions[name]
		_, safe := selectorSafe[name]
		switch {
		case forbidden && safe:
			t.Errorf("%q is classified as both forbidden and selector-safe", name)
		case !forbidden && !safe:
			t.Errorf("%q is unclassified: add it to forbiddenSelectorFunctions if it requires auth/exec/nondeterminism, or to this test's selector-safe list otherwise", name)
		}
	}

	// Every deny-list and selector-safe entry must correspond to a registered
	// YAML function; a stale entry means the function was renamed or removed
	// upstream.
	registered := make(map[string]struct{}, len(u.AtmosYamlTags))
	for _, name := range u.AtmosYamlTags {
		registered[name] = struct{}{}
	}
	for name := range forbiddenSelectorFunctions {
		if _, ok := registered[name]; !ok {
			t.Errorf("deny-list entry %q is not registered in utils.AtmosYamlTags", name)
		}
	}
	for name := range selectorSafe {
		if _, ok := registered[name]; !ok {
			t.Errorf("selector-safe entry %q is not registered in utils.AtmosYamlTags", name)
		}
	}
}

// TestRequestedLabels covers narrowing raw label metadata to only the keys the
// current selector asks about, so unrelated (possibly templated) labels cannot
// affect scope decisions.
func TestRequestedLabels(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		raw    any
		filter map[string]string
		want   map[string]any
		wantOK bool
	}{
		{
			name:   "keeps_only_requested_keys",
			raw:    map[string]any{"env": "prod", "team": "{{ .vars.team }}"},
			filter: map[string]string{"env": "prod"},
			want:   map[string]any{"env": "prod"},
			wantOK: true,
		},
		{
			name:   "missing_requested_key_is_omitted",
			raw:    map[string]any{"env": "prod"},
			filter: map[string]string{"tier": "gold"},
			want:   map[string]any{},
			wantOK: true,
		},
		{
			name:   "nil_raw_yields_empty",
			raw:    nil,
			filter: map[string]string{"env": "prod"},
			want:   map[string]any{},
			wantOK: true,
		},
		{
			// A non-map labels value may be a templated scalar that
			// whole-manifest rendering re-parses into a map; the gate cannot
			// reproduce that, so narrowing is unsafe and the selector is
			// undecidable.
			name:   "non_map_raw_is_not_narrowable",
			raw:    "{{ toJson .vars.labels }}",
			filter: map[string]string{"env": "prod"},
			want:   nil,
			wantOK: false,
		},
		{
			name:   "non_map_slice_is_not_narrowable",
			raw:    []any{"env"},
			filter: map[string]string{"env": "prod"},
			want:   nil,
			wantOK: false,
		},
		{
			// A templated KEY may render to a requested key, so a literal-key
			// lookup cannot decide the match.
			name:   "templated_map_key_is_not_narrowable",
			raw:    map[string]any{"{{ .vars.env }}": "true", "team": "a"},
			filter: map[string]string{"prod": "true"},
			want:   nil,
			wantOK: false,
		},
		{
			name:   "yaml_function_map_key_is_not_narrowable",
			raw:    map[string]any{"!env KEY": "true"},
			filter: map[string]string{"env": "prod"},
			want:   nil,
			wantOK: false,
		},
		{
			name:   "empty_filter_yields_empty",
			raw:    map[string]any{"env": "prod"},
			filter: map[string]string{},
			want:   map[string]any{},
			wantOK: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := RequestedLabels(tt.raw, tt.filter, "")
			if ok != tt.wantOK {
				t.Fatalf("RequestedLabels(%v, %v) ok = %v, want %v", tt.raw, tt.filter, ok, tt.wantOK)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("RequestedLabels(%v, %v) = %v, want %v", tt.raw, tt.filter, got, tt.want)
			}
		})
	}
}

// TestResolveSelectorValue covers best-effort resolution of simple selector
// templates against raw component section data: resolvable templates produce
// exact values, anything else reports unresolved so callers fall back to the
// conservative match.
func TestResolveSelectorValue(t *testing.T) {
	t.Parallel()

	data := map[string]any{
		"vars":     map[string]any{"stage": "dev", "layer": "app"},
		"settings": map[string]any{"team": "platform"},
	}

	dataWithTemplated := map[string]any{
		"vars": map[string]any{"templated": "{{ .something }}", "stage": "dev", "layer": "app"},
	}

	tests := []struct {
		name string
		v    any
		// data overrides the default resolution context for this case.
		data     map[string]any
		want     any
		resolved bool
	}{
		{name: "plain_string", v: "prod", want: "prod", resolved: true},
		{name: "simple_template", v: "{{ .vars.stage }}", want: "dev", resolved: true},
		{name: "template_with_suffix", v: "{{ .vars.layer }}-dev1", want: "app-dev1", resolved: true},
		{name: "sprig_function", v: "{{ .vars.stage | upper }}", want: "DEV", resolved: true},
		{name: "missing_key_unresolved", v: "{{ .vars.nonexistent }}", resolved: false},
		{name: "yaml_function_unresolved", v: "!env TIER", resolved: false},
		{name: "nested_template_output_unresolved", v: "{{ .vars.templated }}", data: dataWithTemplated, resolved: false},
		{
			name:     "slice_resolves_elementwise",
			v:        []any{"static", "{{ .vars.stage }}"},
			want:     []any{"static", "dev"},
			resolved: true,
		},
		{
			name:     "map_resolves_values",
			v:        map[string]any{"layer": "{{ .vars.layer }}-dev1"},
			want:     map[string]any{"layer": "app-dev1"},
			resolved: true,
		},
		{name: "slice_with_unresolvable_element", v: []any{"ok", "{{ .missing }}"}, resolved: false},
		{name: "map_with_unresolvable_value", v: map[string]any{"layer": "{{ .missing }}"}, resolved: false},
		{name: "non_string_scalar", v: 42, want: 42, resolved: true},
		{name: "malformed_template_syntax_unresolved", v: "{{ .vars.stage", resolved: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			input := data
			if tc.data != nil {
				input = tc.data
			}
			got, resolved := ResolveSelectorValue(tc.v, input, "", "")
			if resolved != tc.resolved {
				t.Fatalf("ResolveSelectorValue(%v) resolved = %v, want %v", tc.v, resolved, tc.resolved)
			}
			if tc.resolved && !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("ResolveSelectorValue(%v) = %v, want %v", tc.v, got, tc.want)
			}
		})
	}
}

// TestResolveSelectorValueCustomDelims verifies resolution honors configured
// delimiters.
func TestResolveSelectorValueCustomDelims(t *testing.T) {
	t.Parallel()

	data := map[string]any{"vars": map[string]any{"stage": "dev"}}
	got, resolved := ResolveSelectorValue("[[ .vars.stage ]]", data, "[[", "]]")
	if !resolved || got != "dev" {
		t.Fatalf("ResolveSelectorValue custom delims = (%v, %v), want (dev, true)", got, resolved)
	}
}
