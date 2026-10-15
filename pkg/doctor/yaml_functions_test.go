package doctor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	fntag "github.com/cloudposse/atmos/pkg/function/tag"
	"github.com/cloudposse/atmos/pkg/validation"
)

func TestYAMLFunctionNames(t *testing.T) {
	tests := []struct {
		name, source, unknown, help string
		line, column                int
	}{
		{"typo", "value: !terraform.staet app dev output\n", "!terraform.staet", "Did you mean !terraform.state?", 1, 8},
		{"quoted typo", "value: '!terraform.staet app dev output'\n", "!terraform.staet", "Did you mean !terraform.state?", 1, 8},
		{"unknown", "value: !not_a_function example\n", "!not_a_function", "Use a supported Atmos YAML function name", 1, 8},
		{"nested", "values:\n  - !terraform.staet app dev output\n", "!terraform.staet", "Did you mean !terraform.state?", 2, 5},
		{"tagged collection", "value: !not_a_function [a, b]\n", "!not_a_function", "Use a supported Atmos YAML function name", 1, 8},
		{"valid", "value: !terraform.state app dev output\n", "", "", 0, 0},
		{"quoted valid", "value: '!terraform.state app dev output'\n", "", "", 0, 0},
		{"plain data", "value: '!important'\nother: '!cancelled()'\n", "", "", 0, 0},
		{"literal scalar", "value: !literal '!terraform.staet example'\n", "", "", 0, 0},
		{"literal collection", "value: !literal {nested: !not_a_function example}\n", "", "", 0, 0},
		{"quoted literal", "value: '!literal !terraform.staet example'\n", "", "", 0, 0},
		{"standard tags", "value: !!str example\nnumber: !!int 3\n", "", "", 0, 0},
		{"explicit string", "value: !!str '!terraform.staet example'\n", "", "", 0, 0},
		{"verbatim string", "value: !<tag:yaml.org,2002:str> '!terraform.staet example'\n", "", "", 0, 0},
		{"application URI tag", "value: !<tag:example.com,2026:widget> '!terraform.staet example'\n", "", "", 0, 0},
		{"native types", "bool: !!bool true\nnull: !!null null\nfloat: !!float 1.2\nbinary: !!binary SGVsbG8=\ndate: !!timestamp 2026-10-14\nmap: !!map {}\nseq: !!seq []\n", "", "", 0, 0},
		{"function inside native collection", "value: !!seq\n  - !terraform.staet app dev output\n", "!terraform.staet", "Did you mean !terraform.state?", 2, 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var node yaml.Node
			require.NoError(t, yaml.Unmarshal([]byte(tt.source), &node))
			d := &document{kind: documentStack, path: "stacks/dev.yaml", node: &node}
			var report Report
			scanYAMLFunctions(d, &Options{}, &report)
			if tt.unknown == "" {
				require.Empty(t, report.Findings)
				return
			}
			require.Len(t, report.Findings, 1)
			f := report.Findings[0]
			require.Equal(t, ruleYAMLFunctions, f.RuleID)
			require.Equal(t, validation.SeverityError, f.Severity)
			require.Equal(t, "Unknown YAML function "+tt.unknown, f.Message)
			require.Equal(t, tt.help, f.Remediation)
			require.Equal(t, tt.line, f.Line)
			require.Equal(t, tt.column, f.Column)
			require.Equal(t, d.path, f.File)
			require.False(t, f.Fixable)
			require.Equal(t, statusManual, f.Status)
		})
	}
}

func TestYAMLFunctionsAcceptCanonicalCatalog(t *testing.T) {
	for _, name := range fntag.AllYAML() {
		t.Run(name, func(t *testing.T) {
			d := &document{kind: documentStack, node: &yaml.Node{Kind: yaml.ScalarNode, Tag: name, Value: "unevaluated"}}
			var r Report
			scanYAMLFunctions(d, &Options{}, &r)
			require.Empty(t, r.Findings)
		})
	}
	require.Empty(t, suggestYAMLFunction("!git.ru"), "ambiguous suggestions must not pick an arbitrary function")
}

func TestYAMLFunctionsSelectionAndAliases(t *testing.T) {
	var node yaml.Node
	require.NoError(t, yaml.Unmarshal([]byte("value: &value !terraform.staet app dev output\ncopy: *value\n"), &node))
	d := &document{kind: documentStack, node: &node}
	var r Report
	scanYAMLFunctions(d, &Options{Only: []string{ruleSchema}}, &r)
	require.Empty(t, r.Findings)
	scanYAMLFunctions(d, &Options{Only: []string{ruleYAMLFunctions}}, &r)
	require.Len(t, r.Findings, 1, "report the authored declaration exactly once")
	require.Equal(t, "value", r.Findings[0].Path)
	for _, kind := range []string{"workflow", "actionlint", "editorconfig"} {
		d.kind = kind
		r.Findings = nil
		scanYAMLFunctions(d, &Options{}, &r)
		require.Empty(t, r.Findings, "unrelated YAML dialects must not be checked")
	}
}

func TestDoctorReportsUnknownYAMLFunctionsWithoutExecuting(t *testing.T) {
	root := newProject(t)
	marker := filepath.Join(root, "executed")
	projectFile(t, root, "atmos.yaml", "stacks:\n  base_path: stacks\n  included_paths: ['**/*']\n  name_pattern: '{stage}'\n")
	source := fmt.Sprintf("vars:\n  stage: dev\n  typo: !terraform.staet app dev output\n  command: !exec touch %s\n  secret: !env DOCTOR_FUNCTION_MISSING\n", filepath.ToSlash(marker))
	path := projectFile(t, root, "stacks/dev.yaml", source)
	r := ecRun(t, &Options{Root: root, Offline: true, Only: []string{ruleYAMLFunctions}})
	found := ecFindings(&r, ruleYAMLFunctions)
	require.Len(t, found, 1)
	require.Equal(t, "stacks/dev.yaml", found[0].File)
	require.Equal(t, "vars.typo", found[0].Path)
	require.Equal(t, 3, found[0].Line)
	require.Equal(t, 9, found[0].Column)
	require.Equal(t, 1, r.ExitCode("error"))
	require.Equal(t, 0, r.ExitCode("none"))
	require.NoFileExists(t, marker)
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, source, string(content))
	for _, check := range r.Checks {
		if check.ID == ruleYAMLFunctions {
			require.Equal(t, statusFailed, check.Status)
		}
	}
	projectFile(t, root, "stacks/dev.yaml", strings.ReplaceAll(source, "terraform.staet", "terraform.state"))
	r = ecRun(t, &Options{Root: root, Offline: true, Only: []string{ruleYAMLFunctions}})
	require.Empty(t, ecFindings(&r, ruleYAMLFunctions))
	require.Equal(t, 0, r.ExitCode("error"))
	require.NoFileExists(t, marker)
}
