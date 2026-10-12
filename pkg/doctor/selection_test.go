package doctor

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

const legacyActionWorkflow = "on: push\njobs:\n  plan:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: cloudposse/github-action-atmos-affected-stacks@v6\n"

func modernizationFiles(t *testing.T, r *Report) []string {
	t.Helper()
	var files []string
	for i := range r.Findings {
		if r.Findings[i].RuleID == ruleModernization {
			files = append(files, r.Findings[i].File)
		}
	}
	return files
}

// Git reports affected paths relative to the worktree root, not the doctor root.
func TestAffectedPathsRebasedFromRepositoryRoot(t *testing.T) {
	repo := newProject(t)
	root := filepath.Join(repo, "infra")
	projectFile(t, root, "atmos.yaml", "{}\n")
	projectFile(t, root, ".github/workflows/changed.yaml", legacyActionWorkflow)
	projectFile(t, root, ".github/workflows/unchanged.yaml", legacyActionWorkflow)
	options := Options{Root: root, Offline: true, Only: []string{ruleModernization}, Affected: true}

	t.Run("repository-relative paths select the changed workflow", func(t *testing.T) {
		o := options
		o.Selected = []string{"infra/.github/workflows/changed.yaml"}
		o.SelectedRoot = repo
		r, err := testEngine(t).Run(context.Background(), o)
		require.NoError(t, err)
		require.Equal(t, []string{".github/workflows/changed.yaml"}, modernizationFiles(t, &r))
	})

	t.Run("paths outside the doctor root select nothing", func(t *testing.T) {
		o := options
		o.Selected = []string{"other/.github/workflows/changed.yaml"}
		o.SelectedRoot = repo
		r, err := testEngine(t).Run(context.Background(), o)
		require.NoError(t, err)
		require.Empty(t, modernizationFiles(t, &r))
	})

	t.Run("root-relative paths remain supported without a selection root", func(t *testing.T) {
		o := options
		o.Selected = []string{".github/workflows/unchanged.yaml"}
		r, err := testEngine(t).Run(context.Background(), o)
		require.NoError(t, err)
		require.Equal(t, []string{".github/workflows/unchanged.yaml"}, modernizationFiles(t, &r))
	})
}

func TestCustomSchemaChecksReportOnlyTheirOwnFindings(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", "schemas:\n  failing:\n    schema: failing.json\n    matches: [data/bad.yaml]\n  passing:\n    schema: passing.json\n    matches: [data/good.yaml]\n")
	projectFile(t, root, "failing.json", `{"type":"object","required":["name"]}`)
	projectFile(t, root, "passing.json", `{"type":"object"}`)
	projectFile(t, root, "data/bad.yaml", "other: value\n")
	projectFile(t, root, "data/good.yaml", "name: value\n")
	r, err := testEngine(t).Run(context.Background(), Options{Root: root, Offline: true, Only: []string{ruleSchema}})
	require.NoError(t, err)
	statuses := map[string]string{}
	for _, check := range r.Checks {
		statuses[check.ID] = check.Status
	}
	require.Equal(t, "failed", statuses["schema/failing"])
	require.Equal(t, statusPassed, statuses["schema/passing"])
}

// Atmos renders stack manifests with configured delimiters, but vendor manifests and
// custom commands always use the Go defaults, and GitHub workflows are not Atmos templates.
func TestTemplateDetectionFollowsEachDocumentKindsDelimiters(t *testing.T) {
	const custom = "templates:\n  settings:\n    delimiters: [\"[[\", \"]]\"]\n"
	for _, tc := range []struct {
		name, config, file, content string
		probed                      bool
	}{
		{"stack source with configured delimiters", custom, "stacks/dev.yaml", "components:\n  terraform:\n    app:\n      source: \"github.com/example/repo.git?ref=[[ .vars.ref ]]\"\n", false},
		{"stack source with literal default delimiters", custom, "stacks/dev.yaml", "components:\n  terraform:\n    app:\n      source: \"github.com/example/repo.git?ref={{v1}}\"\n", true},
		{"stack source with stack-level delimiters", "{}\n", "stacks/dev.yaml", "settings:\n  templates:\n    settings:\n      delimiters: [\"<%\", \"%>\"]\ncomponents:\n  terraform:\n    app:\n      source: \"github.com/example/repo.git?ref=<% .vars.ref %>\"\n", false},
		{"vendor source with default delimiters", custom, "vendor.yaml", "spec:\n  sources:\n    - source: \"github.com/example/repo.git?ref={{.Version}}\"\n", false},
		{"vendor source ignores configured delimiters", custom, "vendor.yaml", "spec:\n  sources:\n    - source: \"github.com/example/repo.git?ref=[[.Version]]\"\n", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := newProject(t)
			projectFile(t, root, "atmos.yaml", tc.config)
			projectFile(t, root, tc.file, tc.content)
			s := &repositoryStub{health: RepositoryHealth{"healthy", "healthy"}}
			e := testEngine(t)
			e.Repositories = s
			r, err := e.Run(context.Background(), Options{Root: root, Only: []string{"sources"}})
			require.NoError(t, err)
			if tc.probed {
				require.Equal(t, []string{"https://github.com/example/repo"}, s.calls)
				require.Empty(t, r.Findings)
				return
			}
			require.Empty(t, s.calls)
			require.Len(t, r.Findings, 1)
			require.Equal(t, repositoryInconclusive, r.Findings[0].Reason)
		})
	}
}

func hookEventStatus(t *testing.T, r *Report) string {
	t.Helper()
	for i := range r.Findings {
		if r.Findings[i].RuleID == ruleHookEvent {
			return r.Findings[i].Status
		}
	}
	t.Fatalf("no hook-event finding in %+v", r.Findings)
	return ""
}

// Literal default-delimiter text is data when other delimiters are configured, and GitHub
// expressions are never Atmos templates; neither may block an otherwise verifiable repair.
func TestLiteralDelimiterTextDoesNotBlockRepairs(t *testing.T) {
	const stack = "vars:\n  chart_values: \"replicas: {{ .Values.replicas }}\"\nhooks:\n  notify:\n    events: [after-terraform-apply]\n    command: store\n"
	for _, tc := range []struct {
		name, config, workflow, want string
	}{
		{"configured delimiters make default text literal", "templates:\n  settings:\n    delimiters: [\"[[\", \"]]\"]\n", "", "available"},
		{"default delimiters make the stack dynamic", "{}\n", "", statusVerificationIncomplete},
		{"GitHub expressions are not templates", "templates:\n  settings:\n    delimiters: [\"[[\", \"]]\"]\n", "on: push\njobs:\n  x:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo ${{ github.ref }}\n", "available"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := newProject(t)
			projectFile(t, root, "atmos.yaml", tc.config)
			projectFile(t, root, "stacks/dev.yaml", stack)
			if tc.workflow != "" {
				projectFile(t, root, ".github/workflows/ci.yaml", tc.workflow)
			}
			r, err := testEngine(t).Run(context.Background(), Options{Root: root, Offline: true, Fix: true, DryRun: true, Only: []string{ruleHookEvent}})
			require.NoError(t, err)
			require.Equal(t, tc.want, hookEventStatus(t, &r))
		})
	}
}
