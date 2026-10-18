package doctor

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/validation"
)

func TestDependencyMigrationAdoptsRequiredSemantics(t *testing.T) {
	for _, target := range []string{"present", "missing", "disabled", "abstract", "self", "cycle"} {
		for _, dryRun := range []bool{false, true} {
			t.Run(target+"/dry="+map[bool]string{true: "yes", false: "no"}[dryRun], func(t *testing.T) {
				root := newProject(t)
				projectFile(t, root, "atmos.yaml", "{}\n")
				content := "components:\n  terraform:\n    app:\n      settings:\n        # dependency note\n        depends_on: # keep this\n          - component: database # target note\n        other: untouched\n      vars: {stage: dev}\n"
				switch target {
				case "present":
					content += "    database:\n      vars: {stage: dev}\n"
				case "disabled":
					content += "    database:\n      metadata: {enabled: false}\n"
				case "abstract":
					content += "    database:\n      metadata: {type: abstract}\n"
				case "self":
					content = strings.ReplaceAll(content, "component: database", "component: app")
				case "cycle":
					content += "    database:\n      dependencies:\n        components:\n          - component: app\n"
				}
				path := projectFile(t, root, "stacks/dev.yaml", content)
				e := vtStacksEngine(t, vtProductionValidator{})
				r, err := e.Run(context.Background(), Options{Root: root, Offline: true, Fix: true, DryRun: dryRun, Only: []string{categoryDeprecations}})
				require.NoError(t, err)
				findings := vtFindingsByRule(&r, ruleComponentDependencies)
				require.Len(t, findings, 1, "%+v", r)
				after, err := os.ReadFile(path)
				require.NoError(t, err)
				if dryRun {
					require.Equal(t, content, string(after))
					require.Empty(t, r.Backup)
					require.Empty(t, r.ChangedFiles)
					require.Equal(t, "Candidate verified; dry run left the source unchanged", findings[0].Reason)
				} else {
					require.Equal(t, statusFixed, findings[0].Status, "%+v", r)
					require.Equal(t, []string{"stacks/dev.yaml"}, r.ChangedFiles)
					require.NotEmpty(t, r.Backup)
					require.NotContains(t, string(after), "depends_on")
					require.Contains(t, string(after), "dependencies:")
					for _, preserved := range []string{"# dependency note", "# keep this", "# target note", "other: untouched", "stage: dev"} {
						require.Contains(t, string(after), preserved)
					}
				}
				errors := vtFindingsByRule(&r, ruleDependencies)
				if target == "present" {
					require.Empty(t, errors)
					require.Zero(t, r.ExitCode("error"))
				} else {
					require.Len(t, errors, 1, "%+v", r)
					require.Equal(t, validation.SeverityError, errors[0].Severity)
					require.Equal(t, 1, r.ExitCode("error"))
					if dryRun {
						require.Contains(t, errors[0].Reason, "proposed migration")
					}
				}
				if !dryRun {
					again, err := e.Run(context.Background(), Options{Root: root, Offline: true, Fix: true})
					require.NoError(t, err)
					require.Empty(t, again.ChangedFiles)
					require.Len(t, vtFindingsByRule(&again, ruleDependencies), len(errors))
				}
			})
		}
	}
}

func TestDependencyMigrationCrossStackUsesLogicalNames(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", "stacks:\n  name_pattern: '{stage}'\n")
	projectFile(t, root, "stacks/application.yaml", "components:\n  terraform:\n    app:\n      vars: {stage: dev}\n      settings:\n        depends_on:\n          - component: database\n            stack: prod\n")
	projectFile(t, root, "stacks/storage.yaml", "components:\n  terraform:\n    database:\n      vars: {stage: prod}\n")
	r, err := vtStacksEngine(t, vtProductionValidator{}).Run(context.Background(), Options{Root: root, Offline: true, Fix: true, Only: []string{categoryDeprecations}})
	require.NoError(t, err)
	require.Equal(t, []string{"stacks/application.yaml"}, r.ChangedFiles, "%+v", r)
	require.Empty(t, vtFindingsByRule(&r, ruleDependencies))
}

func TestDependencyMigrationRefusesAmbiguousDeclarations(t *testing.T) {
	for _, suffix := range []string{
		"            stage: prod\n",
		"      dependencies:\n        components: []\n",
		"      depends_on: [other]\n",
	} {
		t.Run(suffix, func(t *testing.T) {
			root := newProject(t)
			projectFile(t, root, "atmos.yaml", "{}\n")
			content := "components:\n  terraform:\n    app:\n      settings:\n        depends_on:\n          - component: database\n" + suffix
			path := projectFile(t, root, "stacks/dev.yaml", content)
			r, err := vtStacksEngine(t, vtProductionValidator{}).Run(context.Background(), Options{Root: root, Offline: true, Fix: true, Only: []string{categoryDeprecations}})
			require.NoError(t, err)
			require.Empty(t, r.ChangedFiles)
			after, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, content, string(after))
		})
	}
}

func TestDependencyMigrationVerificationRejectsOtherChanges(t *testing.T) {
	content := []byte("components:\n  terraform:\n    app:\n      settings:\n        depends_on:\n          - component: database\n            stack: dev\n        other: unchanged\n")
	n, err := parse(content)
	require.NoError(t, err)
	d := &document{path: "stack.yaml", kind: documentStack, content: content, node: n}
	path := []string{"components", "terraform", "app"}
	change := dependencyCandidate(d, nodeAt(rootNode(n), path), path)
	require.NotNil(t, change)
	after, err := applyEdit(content, change)
	require.NoError(t, err)
	require.NoError(t, verifyDocument(content, after, []edit{*change}))
	for _, pair := range [][2]string{{"database", "wrong"}, {"stack: dev", "stack: prod"}, {"component: database", "component: database\n            required: false"}, {"unchanged", "changed"}} {
		changed := []byte(strings.ReplaceAll(string(after), pair[0], pair[1]))
		require.Error(t, verifyDocument(content, changed, []edit{*change}), "%s", changed)
	}
}

func TestDependencyMigrationKeepsSchemaAndInputGuards(t *testing.T) {
	for _, mode := range []string{"schema", "template", "import", "profile"} {
		t.Run(mode, func(t *testing.T) {
			root := newProject(t)
			projectFile(t, root, "atmos.yaml", "{}\n")
			content := "components:\n  terraform:\n    app:\n      settings:\n        depends_on:\n          - component: database\n"
			o := Options{Root: root, Offline: true, Fix: true, Only: []string{categoryDeprecations}}
			switch mode {
			case "schema":
				projectFile(t, root, "atmos.yaml", "schemas:\n  atmos:\n    manifest: schema.json\n")
				projectFile(t, root, "schema.json", `{"type":"object","properties":{"components":{"properties":{"terraform":{"properties":{"app":{"properties":{"settings":{"required":["depends_on"]}}}}}}}}}`)
			case "template":
				content += "      vars: {name: '{{ .name }}'}\n"
			case "import":
				content = "import: [base]\n" + content
				projectFile(t, root, "stacks/base.yaml", "vars: {}\n")
			case "profile":
				o.Profiles = []string{"test"}
			}
			path := projectFile(t, root, "stacks/dev.yaml", content)
			r, err := vtStacksEngine(t, vtProductionValidator{}).Run(context.Background(), o)
			require.NoError(t, err)
			require.Empty(t, r.ChangedFiles, "%+v", r)
			require.Empty(t, r.Backup)
			f := vtFindingsByRule(&r, ruleComponentDependencies)
			require.Len(t, f, 1)
			require.False(t, f[0].Fixable)
			after, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, content, string(after))
		})
	}
}

func TestDependencyDiagnosisHasOneActionableFinding(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", "{}\n")
	projectFile(t, root, "stacks/dev.yaml", "components:\n  terraform:\n    app:\n      settings:\n        depends_on:\n          - component: database\n")
	r, err := vtStacksEngine(t, vtProductionValidator{}).Run(context.Background(), Options{Root: root, Offline: true, Only: []string{categoryDeprecations}})
	require.NoError(t, err)
	require.Len(t, r.Findings, 1)
	require.Equal(t, ruleComponentDependencies, r.Findings[0].RuleID)
	require.True(t, r.Findings[0].Fixable)
	require.Contains(t, r.Findings[0].Reason, "required dependencies")
	require.Equal(t, 6, r.Findings[0].Line)
	require.Equal(t, "components.terraform.app.settings.depends_on", r.Findings[0].Path)
}
