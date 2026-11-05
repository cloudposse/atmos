package doctor

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	fntag "github.com/cloudposse/atmos/pkg/function/tag"
)

func TestYAMLFunctionScopeAcrossEditions(t *testing.T) {
	cases := []struct {
		name, kind, source string
		legacy, strict     int
	}{
		{"quoted stack typo", documentStack, "vars: {value: '!terraform.staet app'}", 1, 0},
		{"native stack typo", documentStack, "vars: {value: !terraform.staet app}", 1, 1},
		{"quoted vendor data", documentVendor, "spec: {source: '!env SOURCE'}", 1, 0},
		{"native vendor function", documentVendor, "spec: {source: !env SOURCE}", 1, 1},
		{"quoted workflow data", documentAtmosWorkflow, "workflows: {demo: {steps: [{command: '!env COMMAND'}]}}", 1, 0},
		{"native workflow function", documentAtmosWorkflow, "workflows: {demo: {steps: [{command: !env COMMAND}]}}", 1, 1},
		{"native config condition", documentConfig, "import: [{path: ci.yaml, when: !cel 'true'}]", 0, 0},
		{"quoted config condition", documentConfig, "import: [{path: ci.yaml, when: {all: ['!cel true']}}]", 0, 1},
		{"quoted workflow condition", documentAtmosWorkflow, "workflows: {demo: {steps: [{continue: {any: ['!cel true']}}]}}", 0, 1},
		{"quoted hook condition", documentStack, "components: {terraform: {app: {hooks: {demo: {when: '!cel true'}}}}}", 0, 1},
		{"native config data", documentConfig, "other: !cel 'true'", 1, 1},
		{"ordinary condition-like data", documentStack, "vars: {when: '!cel true', steps: {data: {when: '!cel true'}}}", 0, 0},
		{"explicit string data", documentStack, "vars: {value: !!str '!terraform.staet app'}", 0, 0},
		{"literal subtree", documentStack, "vars: {value: !literal {nested: !terraform.staet app}}", 0, 0},
		{"native alias crosses scope", documentConfig, "ai:\n  skills:\n    demo:\n      ref: &ref !version tool\nother: *ref\n", 1, 1},
		{"quoted alias crosses scope", documentAtmosWorkflow, "workflows:\n  demo:\n    steps:\n      - prompt: &text '!env PROMPT'\n        command: *text\n", 1, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, strict := range []bool{false, true} {
				var node yaml.Node
				require.NoError(t, yaml.Unmarshal([]byte(tc.source), &node))
				var report Report
				scanYAMLFunctions(&document{kind: tc.kind, node: &node}, &Options{}, &report, strict)
				expected := tc.legacy
				if strict {
					expected = tc.strict
				}
				require.Len(t, report.Findings, expected, "strict=%t: %+v", strict, report.Findings)
			}
		})
	}
}

func TestConfigConditionCatalogAcceptsNativeCEL(t *testing.T) {
	support := fntag.Scope(fntag.CEL, fntag.ConfigConditionContext)
	require.True(t, support.NativeTag)
	require.True(t, support.StringForm, "legacy editions still accept quoted CEL conditions")
}

func TestStrictConditionAliasReportsUseLocation(t *testing.T) {
	var node yaml.Node
	require.NoError(t, yaml.Unmarshal([]byte("workflows:\n  demo:\n    description: &text '!cel true'\n    steps:\n      - when: *text\n"), &node))
	var report Report
	scanYAMLFunctions(&document{kind: documentAtmosWorkflow, node: &node}, &Options{}, &report, true)
	require.Len(t, report.Findings, 1)
	require.Equal(t, 5, report.Findings[0].Line)
	require.Equal(t, "workflows.demo.steps[0].when", report.Findings[0].Path)
	require.Contains(t, report.Findings[0].Message, "Quoted !cel")
}
