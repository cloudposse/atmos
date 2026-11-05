package doctor

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/validation"
	"github.com/cloudposse/atmos/pkg/validator"
)

// Compile-time sentinels: a rename of these schema fields must fail the build here.
var (
	_ = schema.Terraform{InitRunReconfigure: true}
	_ = schema.AtmosConfiguration{BasePath: "."}
)

func TestEditorConfigPathsOnlyIncludeExistingFiles(t *testing.T) {
	for _, tt := range []struct {
		name      string
		files     map[string]string
		config    map[string]any
		wantBases []string
		available bool
	}{
		{"nothing", nil, nil, nil, false},
		{"editorconfig only", map[string]string{".editorconfig": "root = true\n"}, nil, nil, true},
		{"ecrc only", map[string]string{".ecrc": "{}\n"}, nil, []string{".ecrc"}, true},
		{"both defaults", map[string]string{".ecrc": "{}\n", ".editorconfig-checker.json": "{}\n"}, nil, []string{".editorconfig-checker.json", ".ecrc"}, true},
		{"configured and present", map[string]string{"ec/custom.json": "{}\n"}, map[string]any{"validate": map[string]any{"editorconfig": map[string]any{"config_file_paths": []any{"ec/custom.json", "ec/missing.json"}}}}, []string{"ec/custom.json"}, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := newProject(t)
			for name, content := range tt.files {
				projectFile(t, root, name, content)
			}
			p := &project{root: root, config: map[string]any{}}
			if tt.config != nil {
				p.config = tt.config
			}
			paths, available, err := editorConfigPaths(p)
			require.NoError(t, err)
			require.Equal(t, tt.available, available)
			var want []string
			for _, base := range tt.wantBases {
				want = append(want, filepath.Join(root, filepath.FromSlash(base)))
			}
			require.Equal(t, want, paths)
		})
	}
}

func TestEditorConfigWithoutCheckerConfigurationRuns(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", "{}\n")
	projectFile(t, root, ".editorconfig", vtSpaceConfig)
	projectFile(t, root, "notes.txt", vtTabbedText)
	r, err := testEngine(t).Run(context.Background(), Options{Root: root, Offline: true})
	require.NoError(t, err)
	require.Empty(t, r.OperationalError)
	check := vtCheckByID(t, &r, ruleEditorConfig)
	require.Equal(t, "failed", check.Status, "the tab-indented file violates the EditorConfig")
	require.NotEmpty(t, vtFiles(&r, ruleEditorConfig))
}

func TestCheckFailureIsRecordedWhileOtherChecksRun(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", "{}\n")
	projectFile(t, root, ".editorconfig", vtSpaceConfig)
	projectFile(t, root, ".editorconfig-checker.json", "{ not json")
	projectFile(t, root, "stacks/dev.yaml", "components:\n  terraform:\n    app:\n      source: github.com/example/repo.git//modules/app?ref=v1\n")
	r, err := testEngine(t).Run(context.Background(), Options{Root: root, Offline: true})
	require.Error(t, err)
	require.Contains(t, r.OperationalError, ruleEditorConfig)
	require.Equal(t, 2, r.ExitCode("none"), "an operational failure cannot be suppressed")

	failed := vtCheckByID(t, &r, ruleEditorConfig)
	require.Equal(t, statusFailed, failed.Status)
	require.NotEmpty(t, failed.Reason)
	for _, id := range []string{ruleSchema, ruleStacks, ruleGitHubActions, ruleSourceRepository} {
		require.NotEqual(t, statusFailed, vtCheckByID(t, &r, id).Status, id)
	}
	require.Len(t, vtFindingsByRule(&r, ruleSourceRepository), 1, "checks after the failure still ran")
}

func TestCancellationDuringAProjectCheckIsNotRecordedAsAFailure(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", "{}\n")
	projectFile(t, root, "stacks/dev.yaml", "vars: {}\n")
	ctx, cancel := context.WithCancel(context.Background())
	e := vtStacksEngine(t, &vtStackValidator{onValidate: cancel, err: context.Canceled})
	r, err := e.Run(ctx, Options{Root: root, Offline: true})
	require.ErrorIs(t, err, context.Canceled)
	for _, c := range r.Checks {
		require.NotEqual(t, statusFailed, c.Status, c.ID)
	}
}

func TestQuotedBooleanIsAcceptedLikeTheRuntime(t *testing.T) {
	for _, value := range []string{`"false"`, `"true"`, `"yes"`, `false`} {
		t.Run(value, func(t *testing.T) {
			root := newProject(t)
			projectFile(t, root, "atmos.yaml", "components:\n  terraform:\n    init_run_reconfigure: "+value+"\nstacks:\n  base_path: stacks\n")
			projectFile(t, root, "stacks/dev.yaml", "vars:\n  a: 1\n")
			r, err := vtStacksEngine(t, vtProductionValidator{}).Run(context.Background(), Options{Root: root, Offline: true})
			require.NoError(t, err, "a weakly typed boolean must not abort the run")
			require.LessOrEqual(t, r.ExitCode("error"), 1)
			for _, f := range ecFindings(&r, ruleSchema) {
				require.NotEmpty(t, f.Path, f.Message)
				require.Positive(t, f.Line, f.Message)
				require.Positive(t, f.Column, f.Message)
			}
			if value == `"false"` {
				schemaFindings := ecFindings(&r, ruleSchema)
				require.NotEmpty(t, schemaFindings)
				for _, f := range schemaFindings {
					require.Equal(t, "components.terraform.init_run_reconfigure", f.Path)
					require.Equal(t, 3, f.Line)
				}
				require.NotEqual(t, "incomplete", vtCheckByID(t, &r, ruleStacks).Status, "the lenient decoder accepts the quoted boolean")
			}
		})
	}
}

func TestDecodeConfigFallsBackToLenientTyping(t *testing.T) {
	var strict schema.AtmosConfiguration
	require.NoError(t, decodeConfig(map[string]any{"components": map[string]any{"terraform": map[string]any{"base_path": "tf"}}}, &strict))
	require.Equal(t, "tf", strict.Components.Terraform.BasePath)

	var lenient schema.AtmosConfiguration
	require.NoError(t, decodeConfig(map[string]any{"components": map[string]any{"terraform": map[string]any{"init_run_reconfigure": "false", "base_path": "tf"}}}, &lenient))
	require.False(t, lenient.Components.Terraform.InitRunReconfigure)
	require.Equal(t, "tf", lenient.Components.Terraform.BasePath, "fields decoded before the lenient fallback must survive")

	var broken schema.AtmosConfiguration
	require.Error(t, decodeConfig(map[string]any{"components": "not a mapping"}, &broken))
}

func TestStackCheckReportsUndecodableConfigurationAsIncomplete(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", "components: not-a-mapping\n")
	projectFile(t, root, "stacks/dev.yaml", "vars: {}\n")
	r, err := vtStacksEngine(t, vtProductionValidator{}).Run(context.Background(), Options{Root: root, Offline: true, Only: []string{ruleStacks}})
	require.NoError(t, err)
	check := vtCheckByID(t, &r, ruleStacks)
	require.Equal(t, "incomplete", check.Status)
	require.Contains(t, check.Reason, "could not be decoded")
}

func TestLocateFieldResolvesInstancePaths(t *testing.T) {
	var doc yaml.Node
	content := "components:\n  terraform:\n    base_path: x\n    list:\n      - a\n      - b: 1\nodd.key:\n  inner: 2\n"
	require.NoError(t, yaml.Unmarshal([]byte(content), &doc))
	for _, tt := range []struct {
		field string
		line  int
	}{
		{schemaRootField, 1},
		{"", 1},
		{"components", 1},
		{"components.terraform", 2},
		{"components.terraform.base_path", 3},
		{"components.terraform.list.1.b", 6},
		{"components.terraform.list.0", 5},
		{"odd.key.inner", 8},
		{"components.terraform.missing.deeper", 2},
		{"components.terraform.list.9", 4},
	} {
		t.Run(tt.field, func(t *testing.T) {
			line, column := locateField(&doc, tt.field)
			require.Equal(t, tt.line, line)
			require.Positive(t, column)
		})
	}
	line, _ := locateField(nil, "a.b")
	require.Zero(t, line)
}

func TestImportsResolveLikeTheRuntime(t *testing.T) {
	for _, tt := range []struct {
		name      string
		imports   string
		files     map[string]string
		wantRead  []string
		wantFinds int
	}{
		{"relative to the importing file", "import:\n  - ./_defaults\n", map[string]string{"stacks/orgs/acme/_defaults.yaml": ecBroken}, []string{"stacks/orgs/acme/_defaults.yaml"}, 0},
		{"stacks-relative with yml extension", "import:\n  - catalog/vpc\n", map[string]string{"stacks/catalog/vpc.yml": ecBroken}, []string{"stacks/catalog/vpc.yml"}, 0},
		{"parent-relative", "import:\n  - ../shared\n", map[string]string{"stacks/orgs/shared.yaml": ecBroken}, []string{"stacks/orgs/shared.yaml"}, 0},
		{"glob", "import:\n  - catalog/*\n", map[string]string{"stacks/catalog/a.yaml": ecBroken, "stacks/catalog/b.yml": ecBroken}, []string{"stacks/catalog/a.yaml", "stacks/catalog/b.yml"}, 0},
		{"missing", "import:\n  - nope\n", nil, nil, 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := newProject(t)
			projectFile(t, root, "atmos.yaml", "{}\n")
			projectFile(t, root, "stacks/orgs/acme/dev.yaml", tt.imports)
			for name, content := range tt.files {
				projectFile(t, root, name, content)
			}
			r := ecRun(t, &Options{Root: root, Offline: true, Only: []string{ruleSchema, ruleImports}})
			unresolved := 0
			for _, f := range ecFindings(&r, ruleImports) {
				if f.Status == statusManual {
					unresolved++
				}
			}
			require.Equal(t, tt.wantFinds, unresolved)
			require.Equal(t, append([]string{}, tt.wantRead...), ecLoaded(&r))
		})
	}
}

func TestTemplateImportsAreNotParsedAsYAML(t *testing.T) {
	for _, imp := range []string{"catalog/tmpl", "./local", "catalog/tmpl.yaml"} {
		t.Run(imp, func(t *testing.T) {
			root := newProject(t)
			projectFile(t, root, "atmos.yaml", "{}\n")
			projectFile(t, root, "stacks/dev.yaml", "import:\n  - "+imp+"\n")
			const rendered = "vars:\n  enabled: {{ .enabled }}\n  map: {{ .map }}\n"
			projectFile(t, root, "stacks/catalog/tmpl.yaml.tmpl", rendered)
			projectFile(t, root, "stacks/local.yml.tmpl", rendered)
			r := ecRun(t, &Options{Root: root, Offline: true})
			require.Empty(t, ecLoaded(&r), "template files are never parsed as YAML")
			for _, f := range r.Findings {
				if f.RuleID == ruleSchema {
					require.NotContains(t, f.File, ".tmpl", f.Message)
				}
			}
			var notices []Finding
			for _, f := range ecFindings(&r, ruleImports) {
				if f.Status == statusVerificationIncomplete && f.Reason != "" && f.File == "stacks/dev.yaml" {
					notices = append(notices, f)
				}
			}
			require.NotEmpty(t, notices)
			require.Contains(t, notices[0].Reason, "Template import not evaluated")
			require.Contains(t, notices[0].Reason, ".tmpl")
			require.Equal(t, 2, notices[0].Line)
			require.Equal(t, validation.SeverityWarning, notices[0].Severity)
		})
	}
}

func TestDirectoriesInsideConfiguredRootsAreNeverSkipped(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", "components:\n  terraform:\n    base_path: build/components\nstacks:\n  base_path: stacks\n")
	for _, path := range []string{
		"stacks/orgs/acme/build/broken.yaml", "stacks/dist/other.yaml",
		"build/components/app/component.yaml", "build/components/app/dist/vendor.yaml",
	} {
		projectFile(t, root, path, ecBroken)
	}
	for _, path := range []string{"build/unrelated/vendor.yaml", "dist/vendor.yaml", "stacks/node_modules/dep.yaml", "other/node_modules/vendor.yaml"} {
		projectFile(t, root, path, ecBroken)
	}
	r := ecRun(t, &Options{Root: root, Offline: true, Only: []string{ruleSchema, ruleImports}})
	require.Equal(t, []string{
		"build/components/app/component.yaml", "build/components/app/dist/vendor.yaml", "build/unrelated/vendor.yaml",
		"stacks/dist/other.yaml", "stacks/orgs/acme/build/broken.yaml",
	}, ecLoaded(&r), "a root-level build directory that holds a configured component root is project input")
}

func TestDiscoveryFindingsRespectRuleSelection(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", "{}\n")
	projectFile(t, root, "stacks/broken.yaml", ecBroken)
	projectFile(t, root, "stacks/dev.yaml", "import:\n  - missing\n  - '{{ .dynamic }}'\n")
	projectFile(t, root, ".github/workflows/ci.yaml", "name: ci\non: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo ok\n")

	for _, tt := range []struct {
		only        []string
		wantSchema  bool
		wantImports bool
		wantNotice  bool
	}{
		{nil, true, true, true},
		{[]string{ruleSchema}, true, false, false},
		{[]string{ruleImports}, false, true, true},
		{[]string{ruleModernization, "sources"}, false, false, false},
		{[]string{categoryDeprecations}, false, false, true},
	} {
		r := ecRun(t, &Options{Root: root, Offline: true, Only: tt.only})
		var schemaFound, importsFound, noticeFound bool
		for _, f := range r.Findings {
			switch {
			case f.RuleID == ruleSchema && f.File == "stacks/broken.yaml":
				schemaFound = true
			case f.RuleID == ruleImports && f.Status == statusManual:
				importsFound = true
			case f.RuleID == ruleImports && f.Status == statusVerificationIncomplete:
				noticeFound = true
			}
		}
		require.Equal(t, tt.wantSchema, schemaFound, "schema %v", tt.only)
		require.Equal(t, tt.wantImports, importsFound, "imports %v", tt.only)
		require.Equal(t, tt.wantNotice, noticeFound, "notice %v", tt.only)
	}
	r := ecRun(t, &Options{Root: root, Offline: true, Only: []string{ruleModernization}})
	require.Zero(t, r.ExitCode("warning"), "unrelated discovery problems must not fail a limited run")
}

func TestDiscoveryFindingsRespectAffectedSelection(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", "{}\n")
	projectFile(t, root, "stacks/broken.yaml", ecBroken)
	projectFile(t, root, ".github/workflows/ci.yaml", "name: ci\non: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo ok\n")

	t.Run("a changed workflow does not surface an unrelated stack error", func(t *testing.T) {
		r := ecRun(t, &Options{Root: root, Offline: true, Affected: true, Selected: []string{".github/workflows/ci.yaml"}})
		require.Empty(t, ecLoaded(&r))
		require.Zero(t, r.ExitCode("error"))
	})

	t.Run("a changed stack surfaces its own error", func(t *testing.T) {
		r := ecRun(t, &Options{Root: root, Offline: true, Affected: true, Selected: []string{"stacks/broken.yaml"}})
		require.Equal(t, []string{"stacks/broken.yaml"}, ecLoaded(&r))
	})

	t.Run("a changed configuration file widens the selection", func(t *testing.T) {
		r := ecRun(t, &Options{Root: root, Offline: true, Affected: true, Selected: []string{"atmos.yaml"}})
		require.Equal(t, []string{"stacks/broken.yaml"}, ecLoaded(&r))
	})
}

func TestFilterDiscoveryFindingsKeepsGlobalNotices(t *testing.T) {
	root := newProject(t)
	p := &project{root: root}
	selectedFile := filepath.Join(root, "a.yaml")
	o := &Options{Affected: true, Selected: []string{"a.yaml"}}
	r := &Report{Findings: []Finding{{File: ""}, {File: "a.yaml"}, {File: selectedFile}, {File: "b.yaml"}}}
	filterDiscoveryFindings(o, p, r)
	require.Equal(t, []string{"", "a.yaml", selectedFile}, []string{r.Findings[0].File, r.Findings[1].File, r.Findings[2].File})
	require.Len(t, r.Findings, 3)

	all := &Report{Findings: []Finding{{File: "b.yaml"}}}
	filterDiscoveryFindings(&Options{}, p, all)
	require.Len(t, all.Findings, 1, "without --affected nothing is filtered")
}

// A value that fits none of the alternatives of a deeply nested union must be reported once, at the
// value itself, not again by every enclosing union.
func TestMeaningfulFailuresReportsTheOffendingValueOnce(t *testing.T) {
	const content = "components:\n  terraform:\n    app:\n      settings:\n        depends_on: 5\n"
	results, err := validator.NewYAMLSchemaValidator(&schema.AtmosConfiguration{}).ValidateYAMLContent(stackSchema, []byte(content))
	require.NoError(t, err)
	require.Greater(t, len(results), 1, "the fixture must cascade through several enclosing unions")

	kept := meaningfulFailures(results)

	require.NotEmpty(t, kept)
	require.Less(t, len(kept), len(results))
	for _, failure := range kept {
		require.Equal(t, "components.terraform.app.settings.depends_on", failure.Field(), failure.String())
	}
}

func TestMeaningfulFailuresKeepsIndependentFailures(t *testing.T) {
	const content = "components:\n  terraform:\n    app:\n      settings:\n        depends_on: 5\n      vars: 7\n"
	results, err := validator.NewYAMLSchemaValidator(&schema.AtmosConfiguration{}).ValidateYAMLContent(stackSchema, []byte(content))
	require.NoError(t, err)

	fields := map[string]bool{}
	for _, failure := range meaningfulFailures(results) {
		fields[failure.Field()] = true
	}

	require.True(t, fields["components.terraform.app.settings.depends_on"], "%v", fields)
	require.True(t, fields["components.terraform.app.vars"], "%v", fields)
}

// The scheduler executes the list form of the deprecated depends_on, so the manifest schema must accept it.
func TestManifestSchemaAcceptsListFormDependsOn(t *testing.T) {
	v := validator.NewYAMLSchemaValidator(&schema.AtmosConfiguration{})
	for _, tt := range []struct {
		name, depends string
		valid         bool
	}{
		{"strings and objects", "[vpc, {component: db, stack: prod}]", true},
		{"map form", "{1: {component: vpc}}", true},
		{"entry without a component", "[{stack: prod}]", false},
		{"unknown entry key", "[{component: db, tier: x}]", false},
		{"scalar", "5", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			content := "components:\n  terraform:\n    app:\n      settings:\n        depends_on: " + tt.depends + "\n"
			results, err := v.ValidateYAMLContent(stackSchema, []byte(content))
			require.NoError(t, err)
			require.Equal(t, tt.valid, len(results) == 0, "%v", results)
		})
	}
}

func templateImportNotices(r *Report) []Finding {
	var notices []Finding
	findings := ecFindings(r, ruleImports)
	for i := range findings {
		if findings[i].Status == statusVerificationIncomplete && strings.HasPrefix(findings[i].Reason, "Template import not evaluated") {
			notices = append(notices, findings[i])
		}
	}
	return notices
}

// One template imported by many stacks is one notice, located at the first importer, that names the others.
func TestTemplateImportNoticeIsReportedOncePerTemplate(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", "{}\n")
	projectFile(t, root, "stacks/catalog/shared.yaml.tmpl", "vars:\n  a: {{ .a }}\n")
	projectFile(t, root, "stacks/catalog/other.yaml.tmpl", "vars:\n  b: {{ .b }}\n")
	for i := 1; i <= 17; i++ {
		projectFile(t, root, fmt.Sprintf("stacks/s%02d.yaml", i), "import:\n  - catalog/shared\nvars: {}\n")
	}
	projectFile(t, root, "stacks/t01.yaml", "vars: {}\nimport:\n  - catalog/other\n")

	r := ecRun(t, &Options{Root: root, Offline: true, Only: []string{ruleImports}})

	notices := templateImportNotices(&r)
	require.Len(t, notices, 2, "one notice per template file: %+v", notices)
	byTarget := map[string]Finding{}
	for i := range notices {
		byTarget[strings.SplitN(notices[i].Reason, " (", 2)[0]] = notices[i]
	}
	shared := byTarget["Template import not evaluated: stacks/catalog/shared.yaml.tmpl"]
	require.Equal(t, "stacks/s01.yaml", shared.File, "located at the first importer")
	require.Equal(t, 2, shared.Line)
	require.Equal(t, "Template import not evaluated: stacks/catalog/shared.yaml.tmpl (imported by 17 files: stacks/s01.yaml, stacks/s02.yaml, stacks/s03.yaml, … +14 more)", shared.Reason)
	other := byTarget["Template import not evaluated: stacks/catalog/other.yaml.tmpl"]
	require.Equal(t, "stacks/t01.yaml", other.File)
	require.Equal(t, "Template import not evaluated: stacks/catalog/other.yaml.tmpl", other.Reason, "a single importer needs no list")
}

func TestTemplateImportNoticeListsFewImportersInFull(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", "{}\n")
	projectFile(t, root, "stacks/shared.yaml.tmpl", "vars:\n  a: {{ .a }}\n")
	projectFile(t, root, "stacks/a.yaml", "import: [shared]\n")
	projectFile(t, root, "stacks/b.yaml", "import: [shared]\n")

	r := ecRun(t, &Options{Root: root, Offline: true, Only: []string{ruleImports}})

	notices := templateImportNotices(&r)
	require.Len(t, notices, 1)
	require.Equal(t, "Template import not evaluated: stacks/shared.yaml.tmpl (imported by 2 files: stacks/a.yaml, stacks/b.yaml)", notices[0].Reason)
}

// Running only the repair categories still shows the notices, once per template.
func TestTemplateImportNoticeIsVisibleToRepairRunsOnce(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", "{}\n")
	projectFile(t, root, "stacks/shared.yaml.tmpl", "vars:\n  a: {{ .a }}\n")
	projectFile(t, root, "stacks/a.yaml", "import: [shared]\n")
	projectFile(t, root, "stacks/b.yaml", "import: [shared]\n")

	r := ecRun(t, &Options{Root: root, Offline: true, Only: []string{categoryDeprecations}})

	require.Len(t, templateImportNotices(&r), 1)
}
