package doctor

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/utils"
)

func TestYAMLFunctionScopes(t *testing.T) {
	cases := []struct {
		name, kind, source string
		errors             int
	}{
		{"structural component", documentStack, "components: {terraform: {app: {metadata: {component: !env COMPONENT}}}}", 1},
		{"structural inherits", documentStack, "components: {terraform: {app: {metadata: {inherits: [!env BASE]}}}}", 1},
		{"structural literal", documentStack, "components: {terraform: {app: {metadata: {component: !literal app}}}}", 0},
		{"copied mock strings", documentStack, "components: {terraform: {app: {mocks: {output: '!env HOME'}}}}", 0},
		{"copied mock native functions", documentStack, "components: {terraform: {app: {mocks: {output: !env HOME}}}}", 1},
		{"unknown load directive", documentStack, "extension: !include file.yaml", 0},
		{"type defaults", documentStack, "terraform: {vars: {region: !env REGION}}", 0},
		{"component overrides", documentStack, "components: {terraform: {app: {overrides: {vars: {region: !env REGION}}}}}", 0},
		{"mapping key is structural", documentStack, "vars: {!env NAME: value}", 1},
		{"skill ref", documentConfig, "ai: {skills: {example: {ref: !version tool}}}", 0},
		{"wrong ref", documentConfig, "ai: {skills: {example: {source: !version tool}}}", 1},
		{"non scalar ref", documentConfig, "ai: {skills: {example: {ref: !version [tool]}}}", 1},
		{"config append", documentConfig, "commands: !append [{name: example}]", 0},
		{"scalar append", documentStack, "vars: {names: !append hello}", 1},
		{"function collection", documentStack, "vars: {names: !env [HOME]}", 1},
		{"vendor native", documentVendor, "spec: {sources: [{source: !env HOME}]}", 1},
		{"vendor quoted", documentVendor, "spec: {sources: [{source: '!env HOME'}]}", 1},
		{"vendor literal is not an escape", documentVendor, "spec: {sources: [{source: !literal '!env HOME'}]}", 1},
		{"stack imports", documentStack, "import: [!env STACK_PATH]", 1},
		{"stack values", documentStack, "vars: {cwd: !cwd '', random: !random 1 2}", 0},
		{"stack ordinary cel", documentStack, "vars: {enabled: !cel 'true'}", 1},
		{"workflow interactive", documentAtmosWorkflow, "workflows: {example: {steps: [{default: !env HOME, options: ['!exec echo example']}]}}", 0},
		{"workflow nested", documentAtmosWorkflow, "workflows: {example: {steps: [{steps: [{prompt: !env PROMPT}]}]}}", 0},
		{"workflow command", documentAtmosWorkflow, "workflows: {example: {steps: [{command: !env COMMAND}]}}", 1},
		{"workflow metadata", documentAtmosWorkflow, "workflows: {example: {description: '!env DESCRIPTION'}}", 1},
		{"workflow unsupported interactive", documentAtmosWorkflow, "workflows: {example: {steps: [{default: !store account id}]}}", 1},
		{"workflow conditions", documentAtmosWorkflow, "workflows: {example: {steps: [{when: !cel 'true', continue: {any: ['!cel true']}}]}}", 0},
		{"data named when", documentStack, "vars: {steps: {data: {when: !cel true}}}", 1},
		{"scaffold env", documentScaffold, "name: !env TEMPLATE example", 0},
		{"scaffold exec", documentScaffold, "name: !exec echo example", 1},
		{"scaffold quoted env is data", documentScaffold, "name: !!str '!env HOME'", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var n yaml.Node
			require.NoError(t, yaml.Unmarshal([]byte(tc.source), &n))
			var r Report
			scanYAMLFunctions(&document{kind: tc.kind, path: "fixture.yaml", node: &n}, &Options{}, &r)
			require.Len(t, r.Findings, tc.errors)
			for _, f := range r.Findings {
				require.Positive(t, f.Line)
				require.NotEmpty(t, f.Path)
				require.False(t, f.Fixable)
			}
		})
	}
}

func TestDoctorDiscoversAtmosWorkflowsSeparately(t *testing.T) {
	for _, directory := range []string{"workflows", "stacks/workflows"} {
		t.Run(directory, func(t *testing.T) {
			root := newProject(t)
			projectFile(t, root, "atmos.yaml", "workflows:\n  base_path: "+directory+"\nstacks:\n  base_path: stacks\n")
			projectFile(t, root, filepath.Join(directory, "example.yaml"), "workflows: {example: {steps: [{command: !env COMMAND}]}}")
			projectFile(t, root, ".github/workflows/ci.yaml", "name: !env NOT_AN_ATMOS_FUNCTION\n")
			r := ecRun(t, &Options{Root: root, Offline: true, Only: []string{ruleYAMLFunctions}})
			found := ecFindings(&r, ruleYAMLFunctions)
			require.Len(t, found, 1)
			require.Contains(t, found[0].Message, "workflow fields")
			require.Equal(t, directory+"/example.yaml", found[0].File)
		})
	}
}

func TestDoctorDynamicWorkflowDirectoryIsIncomplete(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", "workflows: {base_path: !env WORKFLOW_DIR}\n")
	r := ecRun(t, &Options{Root: root, Offline: true, Only: []string{ruleYAMLFunctions}})
	found := ecFindings(&r, ruleYAMLFunctions)
	require.Len(t, found, 1)
	require.Equal(t, statusVerificationIncomplete, found[0].Status)
	for _, check := range r.Checks {
		if check.ID == ruleYAMLFunctions {
			require.Equal(t, "incomplete", check.Status)
		}
	}
}

func TestDoctorPreservesRegisteredForeignTags(t *testing.T) {
	utils.RegisterForeignTagRewriter("!doctor-test-ref", func(*yaml.Node) error { t.Fatal("doctor executed a foreign tag rewriter"); return nil })
	var n yaml.Node
	require.NoError(t, yaml.Unmarshal([]byte("vars: {reference: !doctor-test-ref example}"), &n))
	var r Report
	scanYAMLFunctions(&document{kind: documentStack, node: &n}, &Options{}, &r)
	require.Empty(t, r.Findings)
}

func TestDoctorChecksAliasAtDifferentFunctionScope(t *testing.T) {
	var n yaml.Node
	require.NoError(t, yaml.Unmarshal([]byte("ai:\n  skills:\n    example:\n      ref: &ref !version tool\nother: *ref\n"), &n))
	var r Report
	scanYAMLFunctions(&document{kind: documentConfig, node: &n}, &Options{}, &r)
	require.Len(t, r.Findings, 1)
	require.Equal(t, "other", r.Findings[0].Path)
	require.Equal(t, 5, r.Findings[0].Line)
}

func TestDoctorTemplatedImportLimitsFunctionCoverage(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", "stacks: {base_path: stacks}\n")
	projectFile(t, root, "stacks/dev.yaml", "import: ['{{ .vars.source }}']\n")
	r := ecRun(t, &Options{Root: root, Offline: true, Only: []string{ruleYAMLFunctions}})
	found := ecFindings(&r, ruleYAMLFunctions)
	require.Len(t, found, 1)
	require.Equal(t, statusVerificationIncomplete, found[0].Status)
	for _, check := range r.Checks {
		if check.ID == ruleYAMLFunctions {
			require.Equal(t, "incomplete", check.Status)
		}
	}
}

func TestDoctorUnknownStackConsumerIsIncomplete(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", "stacks: {base_path: stacks}\n")
	projectFile(t, root, "stacks/dev.yaml", "extension: !env VALUE\ncomponents: {terraform: {app: {extension: !env VALUE}}}\n")
	r := ecRun(t, &Options{Root: root, Offline: true, Only: []string{ruleYAMLFunctions}})
	found := ecFindings(&r, ruleYAMLFunctions)
	require.Len(t, found, 2)
	for _, f := range found {
		require.Equal(t, statusVerificationIncomplete, f.Status)
		require.Positive(t, f.Line)
		require.NotEmpty(t, f.Path)
		require.Contains(t, f.Reason, "unclassified stack fields")
	}
	for _, check := range r.Checks {
		if check.ID == ruleYAMLFunctions {
			require.Equal(t, "incomplete", check.Status)
		}
	}
}
