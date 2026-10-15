package doctor

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/dependency"
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
				content = "import: ['{{ .base }}']\n" + content
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

const legacyDependencyStack = "vars:\n  stage: dev\ncomponents:\n  terraform:\n    database: {}\n    app:\n      settings:\n        depends_on:\n          - component: database\n"

// stacks.name_template that only reads stack sections with pure functions is evaluated by the stack
// validator, so it must not keep the dependency repair from running.
func TestDependencyMigrationWorksWithPureNameTemplate(t *testing.T) {
	for _, tt := range []struct {
		name, template string
		fixed          bool
	}{
		{"name_pattern", "", true},
		{"plain field", `{{ .vars.stage }}`, true},
		{"pure functions", `{{ .vars.stage | upper | lower }}-{{ printf "%s" .vars.stage }}`, true},
		{"environment access", `{{ env "HOME" }}-{{ .vars.stage }}`, false},
		{"component lookup", `{{ (atmos.Component "x" "y").outputs.id }}`, false},
		{"whole-root access", `{{ index . "vars" }}`, false},
		{"unknown section", `{{ .secrets.stage }}`, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := newProject(t)
			config := "stacks:\n  name_pattern: '{stage}'\n"
			if tt.template != "" {
				config = "stacks:\n  name_template: \"" + strings.ReplaceAll(tt.template, `"`, `\"`) + "\"\n"
			}
			projectFile(t, root, "atmos.yaml", config)
			path := projectFile(t, root, "stacks/dev.yaml", legacyDependencyStack)

			r, err := vtStacksEngine(t, vtProductionValidator{}).Run(context.Background(), Options{Root: root, Offline: true, Fix: true, Only: []string{categoryDeprecations}})

			require.NoError(t, err)
			findings := vtFindingsByRule(&r, ruleComponentDependencies)
			require.Len(t, findings, 1, "%+v", r)
			after, readErr := os.ReadFile(path)
			require.NoError(t, readErr)
			if tt.fixed {
				require.Equal(t, statusFixed, findings[0].Status, "%+v", findings[0])
				require.NotContains(t, string(after), "depends_on")
				return
			}
			require.Equal(t, statusVerificationIncomplete, findings[0].Status)
			require.False(t, findings[0].Fixable)
			require.Contains(t, findings[0].Reason, "atmos.yaml:2", "the reason must name the template that blocks the repair")
			require.Equal(t, legacyDependencyStack, string(after))
		})
	}
}

// The runtime prefers dependencies.components over settings.depends_on, so migrating only the abstract base
// would make app depend on vpc instead of the db it declares itself. The repair must be rejected.
const mixedInheritanceStack = `vars:
  stage: dev
components:
  terraform:
    vpc:
      vars: {}
    db:
      vars: {}
    # Abstract base with list-form legacy dependencies.
    base:
      metadata:
        type: abstract
      settings:
        depends_on:
          - component: vpc
    # Concrete component inherits base but declares its own legacy map form.
    app:
      metadata:
        component: app
        inherits: [base]
      settings:
        depends_on:
          1:
            component: db
`

func TestDependencyMigrationRejectsChangesToTheEffectiveGraph(t *testing.T) {
	for _, dryRun := range []bool{false, true} {
		t.Run(map[bool]string{true: "dry run", false: "fix"}[dryRun], func(t *testing.T) {
			root := newProject(t)
			projectFile(t, root, "atmos.yaml", "stacks:\n  base_path: stacks\n  included_paths: ['deploy/**/*']\n  name_pattern: '{stage}'\n")
			path := projectFile(t, root, "stacks/deploy/dev.yaml", mixedInheritanceStack)

			r, err := vtStacksEngine(t, vtProductionValidator{}).Run(context.Background(), Options{Root: root, Offline: true, Fix: true, DryRun: dryRun, Only: []string{"deprecations"}})

			require.NoError(t, err)
			require.Empty(t, r.ChangedFiles, "%+v", r)
			require.Empty(t, r.Backup)
			after, readErr := os.ReadFile(path)
			require.NoError(t, readErr)
			require.Equal(t, mixedInheritanceStack, string(after), "the rejected repair must leave the source untouched")
			findings := vtFindingsByRule(&r, ruleComponentDependencies)
			require.NotEmpty(t, findings)
			for _, f := range findings {
				require.Equal(t, statusVerificationFailed, f.Status, "%+v", f)
				require.False(t, f.Fixable)
				require.Contains(t, f.Reason, "changes the effective dependencies of app in dev: db → vpc")
			}
			if !dryRun {
				require.Equal(t, 1, r.ExitCode("error"), "a requested repair that failed verification exits 1")
			}
		})
	}
}

// Negative path: when every declaration moves together the effective graph is unchanged and the repair proceeds.
func TestDependencyMigrationAllowsRepairsThatKeepTheGraph(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", "stacks:\n  name_pattern: '{stage}'\n")
	consistent := strings.Replace(mixedInheritanceStack, "      settings:\n        depends_on:\n          1:\n            component: db\n", "      settings:\n        depends_on:\n          - component: db\n", 1)
	path := projectFile(t, root, "stacks/dev.yaml", consistent)

	r, err := vtStacksEngine(t, vtProductionValidator{}).Run(context.Background(), Options{Root: root, Offline: true, Fix: true, Only: []string{"deprecations"}})

	require.NoError(t, err)
	require.Equal(t, []string{"stacks/dev.yaml"}, r.ChangedFiles, "%+v", r)
	after, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	require.NotContains(t, string(after), "depends_on")
	for _, f := range vtFindingsByRule(&r, ruleComponentDependencies) {
		require.Equal(t, statusFixed, f.Status, "%+v", f)
	}
}

func TestCompareDependencyTargets(t *testing.T) {
	app := dependency.ComponentRef{Component: "app", Stack: "dev"}
	ref := func(component, stack string) dependency.ComponentRef {
		return dependency.ComponentRef{Component: component, Stack: stack}
	}
	for _, tc := range []struct {
		name          string
		before, after map[dependency.ComponentRef][]dependency.ComponentRef
		want          string
	}{
		{"same", map[dependency.ComponentRef][]dependency.ComponentRef{app: {ref("db", "dev")}}, map[dependency.ComponentRef][]dependency.ComponentRef{app: {ref("db", "dev")}}, ""},
		{"replaced", map[dependency.ComponentRef][]dependency.ComponentRef{app: {ref("db", "dev")}}, map[dependency.ComponentRef][]dependency.ComponentRef{app: {ref("vpc", "dev")}}, "changes the effective dependencies of app in dev: db → vpc"},
		{"dropped", map[dependency.ComponentRef][]dependency.ComponentRef{app: {ref("db", "dev")}}, map[dependency.ComponentRef][]dependency.ComponentRef{app: nil}, "app in dev: db → none"},
		{"other stack", map[dependency.ComponentRef][]dependency.ComponentRef{app: {ref("db", "dev")}}, map[dependency.ComponentRef][]dependency.ComponentRef{app: {ref("db", "prod")}}, "db → db (prod)"},
		{"component only on one side", map[dependency.ComponentRef][]dependency.ComponentRef{app: {ref("db", "dev")}}, map[dependency.ComponentRef][]dependency.ComponentRef{}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := compareDependencyTargets(tc.before, tc.after)
			if tc.want == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, ErrVerification)
			require.Contains(t, err.Error(), tc.want)
		})
	}
}

// Every graph error is a finding of its own, located at the dependency that causes it.
func TestDependencyDiagnosisReportsEveryGraphErrorWithLocations(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", "stacks:\n  base_path: stacks\n  name_pattern: '{stage}'\n")
	projectFile(t, root, "stacks/dev.yaml", "vars:\n  stage: dev\ncomponents:\n  terraform:\n    vpc: {}\n    app:\n      dependencies:\n        components:\n          - component: vpc\n")
	prod := projectFile(t, root, "stacks/prod.yaml", `vars:
  stage: prod
components:
  terraform:
    app:
      dependencies:
        components:
          - component: missing-a
          - component: missing-b
    web:
      dependencies:
        components:
          - component: missing-c
    off:
      metadata:
        enabled: false
    worker:
      settings:
        depends_on:
          - component: off
`)
	_ = prod

	r, err := vtStacksEngine(t, vtProductionValidator{}).Run(context.Background(), Options{Root: root, Offline: true, Only: []string{ruleDependencies}})

	require.NoError(t, err)
	findings := vtFindingsByRule(&r, ruleDependencies)
	require.Len(t, findings, 3, "%+v", findings)
	want := map[string]int{"missing-a": 8, "missing-b": 9, "missing-c": 13}
	for _, f := range findings {
		require.Equal(t, "stacks/prod.yaml", f.File)
		matched := false
		for target, line := range want {
			if strings.Contains(f.Message, "requires "+target+" in prod") {
				require.Equal(t, line, f.Line, f.Message)
				require.Contains(t, f.Path, "dependencies.components")
				matched = true
			}
		}
		require.True(t, matched, "unexpected finding %q", f.Message)
		require.NotContains(t, f.Message, "target_component=", "the message must not carry key=value detail")
	}
}

func TestDependencyDiagnosisLocatesCyclesAndSelfDependencies(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", "{}\n")
	projectFile(t, root, "stacks/dev.yaml", `components:
  terraform:
    a:
      dependencies:
        components:
          - component: b
    b:
      dependencies:
        components:
          - component: a
    solo:
      settings:
        depends_on:
          - component: solo
`)

	r, err := vtStacksEngine(t, vtProductionValidator{}).Run(context.Background(), Options{Root: root, Offline: true, Only: []string{ruleDependencies}})

	require.NoError(t, err)
	findings := vtFindingsByRule(&r, ruleDependencies)
	require.Len(t, findings, 2, "%+v", findings)
	var cycle, self *Finding
	for i := range findings {
		switch {
		case strings.HasPrefix(findings[i].Message, "Dependency cycle"):
			cycle = &findings[i]
		case strings.Contains(findings[i].Message, "depends on itself"):
			self = &findings[i]
		}
	}
	require.NotNil(t, cycle, "%+v", findings)
	require.NotNil(t, self, "%+v", findings)
	require.Equal(t, "stacks/dev.yaml", cycle.File)
	require.Positive(t, cycle.Line)
	require.Equal(t, "stacks/dev.yaml", self.File)
	require.Equal(t, 14, self.Line)
}
