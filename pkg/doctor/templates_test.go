package doctor

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// Stack manifests follow the runtime renderer: stack-level delimiters replace the atmos.yaml
// delimiters, which replace the Go defaults. A replaced "{{" is literal text, not a template.
func TestTemplateMarkersFollowRuntimeDelimiterPrecedence(t *testing.T) {
	const override = "settings:\n  templates:\n    settings:\n      delimiters: [\"<%\", \"%>\"]\n"
	for _, tc := range []struct {
		name   string
		config map[string]any
		stack  string
		want   []string
	}{
		{name: "Go defaults", config: map[string]any{}, stack: "vars: {}\n", want: []string{"{{"}},
		{
			name:   "atmos.yaml delimiters replace the defaults",
			config: map[string]any{"templates": map[string]any{"settings": map[string]any{"delimiters": []any{"[[", "]]"}}}},
			stack:  "vars: {}\n",
			want:   []string{"[["},
		},
		{name: "stack-level delimiters replace the defaults", config: map[string]any{}, stack: override, want: []string{"<%"}},
		{
			name:   "a component-level override adds its marker beside the defaults",
			config: map[string]any{},
			stack:  "components:\n  terraform:\n    app:\n      " + strings.ReplaceAll(strings.TrimSuffix(override, "\n"), "\n", "\n      ") + "\n",
			want:   []string{"{{", "<%"},
		},
		{
			name:   "a component-level override adds its marker beside the stack-level one",
			config: map[string]any{},
			stack:  "settings:\n  templates:\n    settings:\n      delimiters: [\"[[\", \"]]\"]\ncomponents:\n  terraform:\n    app:\n      " + strings.ReplaceAll(strings.TrimSuffix(override, "\n"), "\n", "\n      ") + "\n",
			want:   []string{"[[", "<%"},
		},
		{
			name:   "stack-level delimiters replace atmos.yaml delimiters",
			config: map[string]any{"templates": map[string]any{"settings": map[string]any{"delimiters": []any{"[[", "]]"}}}},
			stack:  override,
			want:   []string{"<%"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var node yaml.Node
			require.NoError(t, yaml.Unmarshal([]byte(tc.stack), &node))
			p := &project{config: tc.config}
			require.Equal(t, tc.want, p.templateMarkers(&document{kind: "stack", node: &node}))
		})
	}
}

func TestStackDelimiterOverrideMakesDefaultTextLiteral(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", "{}\n")
	projectFile(t, root, "stacks/dev.yaml", "settings:\n  templates:\n    settings:\n      delimiters: [\"<%\", \"%>\"]\n"+
		"vars:\n  chart_values: \"replicas: {{ .Values.replicas }}\"\nhooks:\n  notify:\n    events: [after-terraform-apply]\n    command: store\n")
	r, err := testEngine(t).Run(context.Background(), Options{Root: root, Offline: true, Fix: true, DryRun: true, Only: []string{ruleHookEvent}})
	require.NoError(t, err)
	require.Equal(t, "available", hookEventStatus(t, &r))
}

// A component-level override applies only to that component: a sibling that still uses the
// stack-wide delimiter keeps its template, so its source stays inconclusive instead of being probed.
func TestComponentDelimiterOverrideKeepsSiblingTemplates(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", "{}\n")
	projectFile(t, root, "stacks/dev.yaml", "components:\n  terraform:\n"+
		"    app:\n      settings:\n        templates:\n          settings:\n            delimiters: [\"<%\", \"%>\"]\n"+
		"    sibling:\n      source: \"github.com/example/{{ .vars.repo }}.git?ref=v1\"\n")
	stub := &repositoryStub{health: RepositoryHealth{"healthy", "healthy"}}
	e := testEngine(t)
	e.Repositories = stub
	r, err := e.Run(context.Background(), Options{Root: root, Only: []string{ruleSourceRepository}})
	require.NoError(t, err)
	require.Empty(t, stub.calls, "the sibling's templated repository must not be probed")
	require.Len(t, r.Findings, 1)
	require.Equal(t, repositoryInconclusive, r.Findings[0].Reason)
}
