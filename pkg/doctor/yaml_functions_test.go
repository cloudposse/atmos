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
		name, source, message, help string
		kind                        string
		severity                    validation.Severity
		line, column                int
	}{
		{name: "typo", source: "value: !terraform.staet app dev output\n", message: "Unknown YAML function !terraform.staet", help: "Did you mean !terraform.state?", severity: validation.SeverityError, line: 1, column: 8},
		{name: "quoted typo is text at runtime", source: "value: '!terraform.staet app dev output'\n", message: "!terraform.staet is kept as text at runtime", help: "Did you mean !terraform.state?", severity: validation.SeverityWarning, line: 1, column: 8},
		{name: "unknown", source: "value: !not_a_function example\n", message: "Unknown YAML function !not_a_function", help: "Use a supported Atmos YAML function name", severity: validation.SeverityError, line: 1, column: 8},
		{name: "nested", source: "values:\n  - !terraform.staet app dev output\n", message: "Unknown YAML function !terraform.staet", help: "Did you mean !terraform.state?", severity: validation.SeverityError, line: 2, column: 5},
		{name: "tagged collection", source: "value: !not_a_function [a, b]\n", message: "Unknown YAML function !not_a_function", help: "Use a supported Atmos YAML function name", severity: validation.SeverityError, line: 1, column: 8},
		{name: "valid", source: "value: !terraform.state app dev output\n"},
		{name: "quoted valid", source: "value: '!terraform.state app dev output'\n"},
		{name: "quoted valid with index boundary", source: "value: '!terraform.state[0]'\n"},
		{name: "prose that starts like a function", source: "note: '!git.branches are protected'\n", message: "!git.branches is kept as text at runtime", help: "Did you mean !git.branch?", severity: validation.SeverityWarning, line: 1, column: 7},
		{name: "plain data", source: "value: '!important'\nother: '!cancelled()'\n"},
		{name: "literal scalar", source: "value: !literal '!terraform.staet example'\n"},
		{name: "literal collection", source: "value: !literal {nested: !not_a_function example}\n"},
		{name: "quoted literal", source: "value: '!literal !terraform.staet example'\n"},
		{name: "standard tags", source: "value: !!str example\nnumber: !!int 3\n"},
		{name: "explicit string", source: "value: !!str '!terraform.staet example'\n"},
		{name: "verbatim string", source: "value: !<tag:yaml.org,2002:str> '!terraform.staet example'\n"},
		{name: "application URI tag", source: "value: !<tag:example.com,2026:widget> '!terraform.staet example'\n"},
		{name: "native types", source: "bool: !!bool true\nnull: !!null null\nfloat: !!float 1.2\nbinary: !!binary SGVsbG8=\ndate: !!timestamp 2026-10-14\nmap: !!map {}\nseq: !!seq []\n"},
		{name: "function inside native collection", source: "value: !!seq\n  - !terraform.staet app dev output\n", message: "Unknown YAML function !terraform.staet", help: "Did you mean !terraform.state?", severity: validation.SeverityError, line: 2, column: 5},
		{name: "config accepts supported function", kind: documentConfig, source: "value: !env HOME\n"},
		{name: "config rejects stack-only function", kind: documentConfig, source: "value: !terraform.state vpc dev vpc_id\n", message: "!terraform.state is not supported in atmos.yaml; supported: " + strings.Join(fntag.AtmosConfigYAML(), ", "), help: "Use a function atmos.yaml supports, or move this value into a stack manifest", severity: validation.SeverityError, line: 1, column: 8},
		{name: "config rejects store", kind: documentConfig, source: "value: !store prod/db password\n", message: "!store is not supported in atmos.yaml; supported: " + strings.Join(fntag.AtmosConfigYAML(), ", "), help: "Use a function atmos.yaml supports, or move this value into a stack manifest", severity: validation.SeverityError, line: 1, column: 8},
		{name: "config keeps unknown names unknown", kind: documentConfig, source: "value: !terraform.staet app dev output\n", message: "Unknown YAML function !terraform.staet", help: "Did you mean !terraform.state?", severity: validation.SeverityError, line: 1, column: 8},
		{name: "config never evaluates untagged strings", kind: documentConfig, source: "value: '!terraform.staet app dev output'\nnote: '!git.branches are protected'\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var node yaml.Node
			require.NoError(t, yaml.Unmarshal([]byte(tt.source), &node))
			kind := tt.kind
			if kind == "" {
				kind = documentStack
			}
			d := &document{kind: kind, path: "stacks/dev.yaml", node: &node}
			var report Report
			if kind == documentStack {
				scanFunctionNode(d, d.node, []string{"vars"}, &report)
			} else {
				scanYAMLFunctions(d, &Options{}, &report)
			}
			if tt.message == "" {
				require.Empty(t, report.Findings)
				return
			}
			require.Len(t, report.Findings, 1)
			f := report.Findings[0]
			require.Equal(t, ruleYAMLFunctions, f.RuleID)
			require.Equal(t, tt.severity, f.Severity)
			require.Equal(t, tt.message, f.Message)
			require.Equal(t, tt.help, f.Remediation)
			require.Equal(t, tt.line, f.Line)
			require.Equal(t, tt.column, f.Column)
			require.Equal(t, d.path, f.File)
			require.False(t, f.Fixable)
			require.Equal(t, statusManual, f.Status)
		})
	}
}

// A function-looking string only warns, so it never fails the default error threshold.
func TestYAMLFunctionProseWarningDoesNotFailRun(t *testing.T) {
	var node yaml.Node
	require.NoError(t, yaml.Unmarshal([]byte("note: '!git.branches are protected'\n"), &node))
	var report Report
	scanYAMLFunctions(&document{kind: documentStack, node: &node}, &Options{}, &report)
	require.Len(t, report.Findings, 1)
	require.Equal(t, 0, report.ExitCode("error"))
	require.Equal(t, 1, report.ExitCode("warning"))
}

func TestYAMLFunctionsAcceptCanonicalCatalog(t *testing.T) {
	for _, name := range fntag.AllYAML() {
		t.Run(name, func(t *testing.T) {
			kind := yaml.ScalarNode
			if name == "!append" {
				kind = yaml.SequenceNode
			}
			if name == "!cel" {
				return
			} // Conditions have their own field-specific dispatcher.
			d := &document{kind: documentStack, node: &yaml.Node{Kind: kind, Tag: name, Value: "unevaluated"}}
			var r Report
			scanFunctionNode(d, d.node, []string{"vars"}, &r)
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

func TestYAMLFunctionsExplicitEdition(t *testing.T) {
	for _, strict := range []bool{false, true} {
		var node yaml.Node
		require.NoError(t, yaml.Unmarshal([]byte(`import:
  - path: ci.yaml
    when:
      all:
        - "!cel ci"
commands:
  - name: example
    steps:
      - command: echo hello
        when: !cel "ci"
vars:
  when: "!cel literal"
  prose: "!terraform.staet is text"
`), &node))
		var report Report
		scanYAMLFunctions(&document{kind: documentConfig, node: &node}, &Options{}, &report, strict)
		if strict {
			require.Len(t, report.Findings, 1)
			require.Contains(t, report.Findings[0].Remediation, `!cel "expression"`)
		} else {
			require.Empty(t, report.Findings)
		}
	}
}
