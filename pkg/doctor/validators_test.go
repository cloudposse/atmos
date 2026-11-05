package doctor

import (
	"context"
	"io/fs"
	"path/filepath"
	"testing"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/filesystem"
	"github.com/cloudposse/atmos/pkg/validation"
	"github.com/cloudposse/atmos/pkg/validator"
)

const (
	vtGoodWorkflow = "on: push\njobs:\n  x:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo hi\n"
	vtBadWorkflow  = "on: push\njobs:\n  x:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo ${{ nope.value }}\n"
	vtSpaceConfig  = "root = true\n[*]\nindent_style = space\nindent_size = 2\n"
	vtTabbedText   = "line\n\tindented\n"
)

func vtRun(t *testing.T, root string, only string, mutate func(*Options)) Report {
	t.Helper()
	o := Options{Root: root, Offline: true, Only: []string{only}}
	if mutate != nil {
		mutate(&o)
	}
	r, err := testEngine(t).Run(context.Background(), o)
	require.NoError(t, err)
	return r
}

func vtFiles(r *Report, rule string) []string {
	var files []string
	findings := vtFindingsByRule(r, rule)
	for i := range findings {
		files = append(files, filepath.ToSlash(findings[i].File))
	}
	return files
}

func TestGitHubActionsValidation(t *testing.T) {
	for _, tc := range []struct {
		name      string
		workflows map[string]string
		mutate    func(*Options)
		status    string
		files     int
		findings  []string
	}{
		{name: "no workflows", status: "skipped"},
		{name: "valid workflow", workflows: map[string]string{"good.yaml": vtGoodWorkflow}, status: statusPassed, files: 1},
		{name: "invalid workflow", workflows: map[string]string{"bad.yaml": vtBadWorkflow, "good.yaml": vtGoodWorkflow}, status: "failed", files: 2, findings: []string{".github/workflows/bad.yaml"}},
		{
			name: "affected selection narrows to changed workflow", workflows: map[string]string{"bad.yaml": vtBadWorkflow, "good.yaml": vtGoodWorkflow},
			mutate: func(o *Options) { o.Affected, o.Selected = true, []string{".github/workflows/good.yaml"} },
			status: statusPassed, files: 1,
		},
		{
			name: "affected selection reports selected invalid workflow", workflows: map[string]string{"bad.yaml": vtBadWorkflow, "good.yaml": vtGoodWorkflow},
			mutate: func(o *Options) { o.Affected, o.Selected = true, []string{".github/workflows/bad.yaml"} },
			status: "failed", files: 1, findings: []string{".github/workflows/bad.yaml"},
		},
		{
			name: "affected selection without workflows skips", workflows: map[string]string{"bad.yaml": vtBadWorkflow},
			mutate: func(o *Options) { o.Affected, o.Selected = true, []string{"unrelated.txt"} },
			status: "skipped",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := newProject(t)
			projectFile(t, root, "atmos.yaml", "{}\n")
			for name, content := range tc.workflows {
				projectFile(t, root, filepath.Join(".github", "workflows", name), content)
			}
			r := vtRun(t, root, ruleGitHubActions, tc.mutate)
			check := vtCheckByID(t, &r, ruleGitHubActions)
			require.Equal(t, tc.status, check.Status)
			require.Equal(t, tc.files, check.Files)
			require.Equal(t, tc.findings, vtFiles(&r, ruleGitHubActions))
			for _, f := range vtFindingsByRule(&r, ruleGitHubActions) {
				require.Equal(t, validation.SeverityError, f.Severity)
				require.Equal(t, 6, f.Line)
				require.Contains(t, f.Message, `undefined variable "nope"`)
			}
		})
	}
}

func TestGitHubActionsDisabledByOnlySelector(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", "{}\n")
	projectFile(t, root, ".github/workflows/bad.yaml", vtBadWorkflow)
	r := vtRun(t, root, ruleSchema, nil)
	require.Empty(t, vtFindingsByRule(&r, ruleGitHubActions))
	for _, c := range r.Checks {
		require.NotEqual(t, ruleGitHubActions, c.ID)
	}
}

// Cancellation observed between validators must stop the run before later checks start.
func TestValidatorsStopWhenCancelledBetweenChecks(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", "{}\n")
	projectFile(t, root, ".editorconfig", vtSpaceConfig)
	projectFile(t, root, ".editorconfig-checker.json", "{}\n")
	projectFile(t, root, ".github/workflows/good.yaml", vtGoodWorkflow)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	e := testEngine(t)
	e.Observe = func(event Event) {
		if event.Label == "GitHub Actions" && event.Done {
			cancel()
		}
	}
	r, err := e.Run(ctx, Options{Root: root, Offline: true, Only: []string{"validation"}})
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, statusPassed, vtCheckByID(t, &r, ruleGitHubActions).Status)
	for _, c := range r.Checks {
		require.NotEqual(t, ruleEditorConfig, c.ID, "editorconfig must not run after cancellation")
	}
}

func TestEditorConfigValidation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		files  map[string]string
		mutate func(*Options)
		status string
		// git initializes a Git repository in the project before the run.
		git bool
		// failing lists files expected to carry an editorconfig error, with their lines.
		failing map[string]int
		checked int
	}{
		{name: "skipped without any configuration", files: map[string]string{"a.txt": vtTabbedText}, status: "skipped"},
		{
			name:   "indentation violation is an error finding",
			files:  map[string]string{".editorconfig": vtSpaceConfig, ".editorconfig-checker.json": "{}\n", "a.txt": vtTabbedText, "ok.txt": "line\n  indented\n"},
			status: "failed", failing: map[string]int{"a.txt": 2}, checked: 5,
		},
		{
			name:   "clean tree passes",
			files:  map[string]string{".editorconfig": vtSpaceConfig, ".editorconfig-checker.json": "{}\n", "ok.txt": "line\n  indented\n"},
			status: statusPassed, checked: 4,
		},
		{
			name:   "config_file_paths from atmos.yaml are honored",
			files:  map[string]string{"atmos.yaml": "validate:\n  editorconfig:\n    config_file_paths: [tools/ec.json]\n", ".editorconfig": vtSpaceConfig, "tools/ec.json": `{"Exclude":["ignored\\.txt$"]}`, "a.txt": vtTabbedText, "ignored.txt": vtTabbedText},
			status: "failed", failing: map[string]int{"a.txt": 2}, checked: 4,
		},
		{
			name:   "exclude option removes files from validation",
			files:  map[string]string{".editorconfig": vtSpaceConfig, ".editorconfig-checker.json": "{}\n", "a.txt": vtTabbedText, "b.txt": vtTabbedText},
			mutate: func(o *Options) { o.Exclude = []string{"a.txt"} },
			status: "failed", failing: map[string]int{"b.txt": 2}, checked: 4,
		},
		{
			name:   "affected selection limits validation to changed files",
			files:  map[string]string{".editorconfig": vtSpaceConfig, ".editorconfig-checker.json": "{}\n", "a.txt": vtTabbedText, "b.txt": vtTabbedText},
			mutate: func(o *Options) { o.Affected, o.Selected = true, []string{"b.txt"} },
			status: "failed", failing: map[string]int{"b.txt": 2}, checked: 1,
		},
		{
			name: "gitignored files are not validated inside a Git work tree",
			git:  true,
			files: map[string]string{
				".editorconfig": vtSpaceConfig, ".editorconfig-checker.json": "{}\n", ".gitignore": ".workdir/\n",
				"a.txt": vtTabbedText, ".workdir/vendored/bad.txt": vtTabbedText,
			},
			status: "failed", failing: map[string]int{"a.txt": 2}, checked: 5,
		},
		{
			name: "untracked files that Git does not ignore are validated",
			git:  true,
			files: map[string]string{
				".editorconfig": vtSpaceConfig, ".editorconfig-checker.json": "{}\n",
				"a.txt": vtTabbedText, "pending/bad.txt": vtTabbedText,
			},
			status: "failed", failing: map[string]int{"a.txt": 2, "pending/bad.txt": 2}, checked: 5,
		},
		{
			name: "validate.editorconfig.exclude globs are honored relative to the project root",
			git:  true,
			files: map[string]string{
				"atmos.yaml":    "validate:\n  editorconfig:\n    exclude: [\"vendor/**\"]\n",
				".editorconfig": vtSpaceConfig, ".editorconfig-checker.json": "{}\n",
				"a.txt": vtTabbedText, "vendor/lib/bad.txt": vtTabbedText,
			},
			status: "failed", failing: map[string]int{"a.txt": 2}, checked: 4,
		},
		{
			name: "validate.editorconfig.disable_indentation turns the check off",
			files: map[string]string{
				"atmos.yaml":    "validate:\n  editorconfig:\n    disable_indentation: true\n",
				".editorconfig": vtSpaceConfig, ".editorconfig-checker.json": "{}\n", "a.txt": vtTabbedText,
			},
			status: statusPassed, checked: 4,
		},
		{
			name: "default excludes skip log files",
			files: map[string]string{
				".editorconfig": vtSpaceConfig, ".editorconfig-checker.json": "{}\n", "a.txt": vtTabbedText, "run.log": vtTabbedText,
			},
			status: "failed", failing: map[string]int{"a.txt": 2}, checked: 4,
		},
		{
			name: "validate.editorconfig.ignore_defaults validates files the defaults skip",
			files: map[string]string{
				"atmos.yaml":    "validate:\n  editorconfig:\n    ignore_defaults: true\n",
				".editorconfig": vtSpaceConfig, ".editorconfig-checker.json": "{}\n", "a.txt": vtTabbedText, "run.log": vtTabbedText,
			},
			status: "failed", failing: map[string]int{"a.txt": 2, "run.log": 2}, checked: 5,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := newProject(t)
			if tc.git {
				output, err := gitCommand(root, "init", "--quiet").CombinedOutput()
				require.NoError(t, err, string(output))
			}
			if _, ok := tc.files["atmos.yaml"]; !ok {
				projectFile(t, root, "atmos.yaml", "{}\n")
			}
			for path, content := range tc.files {
				projectFile(t, root, filepath.FromSlash(path), content)
			}
			r := vtRun(t, root, ruleEditorConfig, tc.mutate)
			check := vtCheckByID(t, &r, ruleEditorConfig)
			require.Equal(t, tc.status, check.Status)
			require.Equal(t, tc.checked, check.Files)
			got := map[string]int{}
			for _, f := range vtFindingsByRule(&r, ruleEditorConfig) {
				require.Equal(t, validation.SeverityError, f.Severity)
				require.Equal(t, categoryValidation, f.Category)
				require.Contains(t, f.Message, "indent")
				got[filepath.ToSlash(f.File)] = f.Line
			}
			if len(tc.failing) == 0 {
				tc.failing = map[string]int{}
			}
			require.Equal(t, tc.failing, got)
		})
	}
}

func TestEditorConfigRejectsMalformedCheckerConfiguration(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", "{}\n")
	projectFile(t, root, ".editorconfig", vtSpaceConfig)
	projectFile(t, root, ".editorconfig-checker.json", "{not json")
	r, err := testEngine(t).Run(context.Background(), Options{Root: root, Offline: true, Only: []string{ruleEditorConfig}})
	require.Error(t, err)
	require.NotEmpty(t, r.OperationalError)
	require.Equal(t, 2, r.ExitCode("error"), "an unreadable checker configuration is operational, never a pass")
}

func TestSelectedPathsAppliesRootSelectionAndExclusions(t *testing.T) {
	root := newProject(t)
	outside := newProject(t)
	p := &project{root: root}
	inputs := []string{
		filepath.Join(root, "keep.txt"),
		filepath.Join(root, "sub", "keep.txt"),
		filepath.Join(root, "sub", "skip.txt"),
		filepath.Join(outside, "elsewhere.txt"),
	}
	for _, tc := range []struct {
		name string
		o    Options
		want []string
	}{
		{"everything inside the root", Options{}, inputs[:3]},
		{"exclude globs drop matches", Options{Exclude: []string{"sub/skip.txt"}}, inputs[:2]},
		{"affected keeps only selected", Options{Affected: true, Selected: []string{"sub/keep.txt"}}, inputs[1:2]},
		{"affected selection can be fully excluded", Options{Affected: true, Selected: []string{"sub/skip.txt"}, Exclude: []string{"sub/**"}}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := selectedPaths(p, &tc.o, inputs)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestSchemaMatchPaths(t *testing.T) {
	root := newProject(t)
	for _, name := range []string{"data/a.yaml", "data/b.yaml", "other/c.yaml"} {
		projectFile(t, root, filepath.FromSlash(name), "k: v\n")
	}
	p := &project{root: root}
	abs := func(name string) string { return filepath.Join(root, filepath.FromSlash(name)) }
	entry := func(patterns ...any) map[string]any { return map[string]any{"matches": patterns} }
	for _, tc := range []struct {
		name  string
		entry map[string]any
		o     Options
		want  []string
	}{
		{"relative glob resolves under root", entry("data/*.yaml"), Options{}, []string{abs("data/a.yaml"), abs("data/b.yaml")}},
		{"absolute glob is used as-is", entry(abs("other/*.yaml")), Options{}, []string{abs("other/c.yaml")}},
		{"overlapping patterns report each file once", entry("data/*.yaml", "**/a.yaml", "data/a.yaml"), Options{}, []string{abs("data/a.yaml"), abs("data/b.yaml")}},
		{"non-string patterns are ignored", entry(42, nil, "other/*.yaml"), Options{}, []string{abs("other/c.yaml")}},
		{"no matches key yields nothing", map[string]any{}, Options{}, nil},
		{"exclude applies", entry("data/*.yaml"), Options{Exclude: []string{"data/b.yaml"}}, []string{abs("data/a.yaml")}},
		{"affected selection applies", entry("data/*.yaml", "other/*.yaml"), Options{Affected: true, Selected: []string{"other/c.yaml"}}, []string{abs("other/c.yaml")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := schemaMatchPaths(p, &tc.o, tc.entry)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
	_, err := schemaMatchPaths(p, &Options{}, entry("["))
	require.ErrorIs(t, err, doublestar.ErrBadPattern, "a malformed glob is surfaced, not silently ignored")
}

func TestCaptureSchemaClassifiesSources(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "plain.json", `{"type":"object"}`)
	projectFile(t, root, "internal-ref.json", `{"definitions":{"x":{"type":"string"}},"properties":{"a":{"$ref":"#/definitions/x"}}}`)
	projectFile(t, root, "external-ref.json", `{"properties":{"a":{"$ref":"other.json"}}}`)
	projectFile(t, root, "nested-external-ref.json", `{"allOf":[{"properties":{"a":{"$dynamicRef":"https://example.com/s.json"}}}]}`)
	projectFile(t, root, "recursive-ref.json", `{"$recursiveRef":"common.json"}`)
	projectFile(t, root, "invalid.json", `{nope`)
	for _, tc := range []struct {
		name, source, wantReason string
		wantSource               string
		captured                 bool
		wantErr                  error
		anyErr                   bool
	}{
		{name: "template", source: "{{ .schema }}", wantSource: "{{ .schema }}", wantReason: "Dynamic schema source was not evaluated"},
		{name: "yaml function", source: "!env SCHEMA", wantSource: "!env SCHEMA", wantReason: "Dynamic schema source was not evaluated"},
		{name: "bundled atmos schema", source: "atmos://schema/atmos/manifest/1.0", wantSource: "atmos://schema/atmos/manifest/1.0"},
		{name: "remote schema", source: "https://example.com/schema.json", wantSource: "https://example.com/schema.json", wantReason: "Remote schema cannot be included in the static source snapshot"},
		{name: "local relative schema", source: "plain.json", wantSource: filepath.Join(root, "plain.json"), captured: true},
		{name: "local schema with internal reference", source: "internal-ref.json", wantSource: filepath.Join(root, "internal-ref.json"), captured: true},
		{name: "external reference", source: "external-ref.json", wantSource: filepath.Join(root, "external-ref.json"), wantReason: "External schema references need a transitive source snapshot; validation was not attempted", captured: true},
		{name: "nested dynamic reference", source: "nested-external-ref.json", wantSource: filepath.Join(root, "nested-external-ref.json"), wantReason: "External schema references need a transitive source snapshot; validation was not attempted", captured: true},
		{name: "recursive reference", source: "recursive-ref.json", wantSource: filepath.Join(root, "recursive-ref.json"), wantReason: "External schema references need a transitive source snapshot; validation was not attempted", captured: true},
		{name: "missing local schema", source: "missing.json", wantSource: filepath.Join(root, "missing.json"), wantErr: fs.ErrNotExist},
		{name: "malformed local schema", source: "invalid.json", wantSource: filepath.Join(root, "invalid.json"), captured: true, anyErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := &project{root: root, extra: map[string][]byte{}}
			e := &Engine{FS: filesystem.NewOSFileSystem()}
			source, reason, err := e.captureSchema(p, tc.source)
			switch {
			case tc.wantErr != nil:
				require.ErrorIs(t, err, tc.wantErr)
			case tc.anyErr:
				require.Error(t, err)
			default:
				require.NoError(t, err)
			}
			require.Equal(t, tc.wantSource, source)
			require.Equal(t, tc.wantReason, reason)
			if tc.captured {
				require.Contains(t, p.extra, tc.wantSource, "local schemas join the source snapshot")
			} else {
				require.Empty(t, p.extra)
			}
		})
	}
}

func TestExternalSchemaReference(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value any
		want  bool
	}{
		{"scalar", "$ref", false},
		{"empty object", map[string]any{}, false},
		{"internal pointer", map[string]any{"$ref": "#/definitions/x"}, false},
		{"external file", map[string]any{"$ref": "x.json"}, true},
		{"non-string reference value", map[string]any{"$ref": 5}, false},
		{"reference-like property name is data", map[string]any{"properties": map[string]any{"x": map[string]any{"type": "string"}}, "description": "see other.json"}, false},
		{"external inside array", []any{map[string]any{"a": []any{map[string]any{"$recursiveRef": "y.json"}}}}, true},
		{"only internal inside array", []any{map[string]any{"$dynamicRef": "#node"}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, externalSchemaReference(tc.value))
		})
	}
}

func vtSchemaProject(t *testing.T, registries string, files map[string]string) string {
	t.Helper()
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", "schemas:\n"+registries)
	for path, content := range files {
		projectFile(t, root, filepath.FromSlash(path), content)
	}
	return root
}

func TestCustomSchemaChecksCoverageAndFindings(t *testing.T) {
	const requireName = `{"type":"object","required":["name"]}`
	for _, tc := range []struct {
		name      string
		registry  string
		files     map[string]string
		wantCheck string
		status    string
		reason    string
		count     int
		findings  []string
	}{
		{
			name:      "unparseable matched file is a schema error",
			registry:  "  docs:\n    schema: s.json\n    matches: [data/*.yaml]\n",
			files:     map[string]string{"s.json": requireName, "data/ok.yaml": "name: x\n", "data/broken.yaml": "key: [\n"},
			wantCheck: "schema/docs", status: "failed", count: 2, findings: []string{"data/broken.yaml"},
		},
		{
			name:      "valid files pass",
			registry:  "  docs:\n    schema: s.json\n    matches: [data/*.yaml]\n",
			files:     map[string]string{"s.json": requireName, "data/ok.yaml": "name: x\n"},
			wantCheck: "schema/docs", status: statusPassed, count: 1,
		},
		{
			name:      "remote schema is incomplete and not evaluated",
			registry:  "  docs:\n    schema: https://example.com/schema.json\n    matches: [data/*.yaml]\n",
			files:     map[string]string{"data/bad.yaml": "other: 1\n"},
			wantCheck: "schema/docs", status: "incomplete", reason: "Remote schema cannot be included in the static source snapshot",
		},
		{
			name:      "dynamic schema source is incomplete",
			registry:  "  docs:\n    schema: '{{ .schema }}'\n    matches: [data/*.yaml]\n",
			files:     map[string]string{"data/bad.yaml": "other: 1\n"},
			wantCheck: "schema/docs", status: "incomplete", reason: "Dynamic schema source was not evaluated",
		},
		{
			name:      "external reference is incomplete",
			registry:  "  docs:\n    schema: s.json\n    matches: [data/*.yaml]\n",
			files:     map[string]string{"s.json": `{"properties":{"a":{"$ref":"common.json"}},"required":["name"]}`, "data/bad.yaml": "other: 1\n"},
			wantCheck: "schema/docs", status: "incomplete", reason: "External schema references need a transitive source snapshot; validation was not attempted",
		},
		{
			name:      "internal reference is still validated",
			registry:  "  docs:\n    schema: s.json\n    matches: [data/*.yaml]\n",
			files:     map[string]string{"s.json": `{"definitions":{"n":{"type":"string"}},"properties":{"name":{"$ref":"#/definitions/n"}},"required":["name"]}`, "data/bad.yaml": "other: 1\n"},
			wantCheck: "schema/docs", status: "failed", count: 1, findings: []string{"data/bad.yaml"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := vtSchemaProject(t, tc.registry, tc.files)
			r := vtRun(t, root, ruleSchema, nil)
			check := vtCheckByID(t, &r, tc.wantCheck)
			require.Equal(t, tc.status, check.Status)
			require.Equal(t, tc.reason, check.Reason)
			require.Equal(t, tc.count, check.Files)
			var schemaFiles []string
			for _, f := range vtFindingsByRule(&r, ruleSchema) {
				if f.File != "atmos.yaml" {
					schemaFiles = append(schemaFiles, filepath.ToSlash(f.File))
				}
			}
			require.Equal(t, tc.findings, schemaFiles)
		})
	}
}

func TestCustomSchemaRegistriesIgnoreMalformedEntries(t *testing.T) {
	root := vtSchemaProject(t, "  scalar: just-a-string\n  noschema:\n    matches: [data/*.yaml]\n  emptyschema:\n    schema: ''\n    matches: [data/*.yaml]\n", map[string]string{"data/a.yaml": "other: 1\n"})
	r := vtRun(t, root, ruleSchema, nil)
	for _, c := range r.Checks {
		require.NotContains(t, c.ID, "schema/", "entries without a usable schema produce no check")
	}
	require.Empty(t, vtFiles(&r, ruleSchema))
}

func TestCustomSchemasDisabledWhenSchemaAndDeprecationsAreExcluded(t *testing.T) {
	root := vtSchemaProject(t, "  docs:\n    schema: s.json\n    matches: [data/*.yaml]\n", map[string]string{"s.json": `{"required":["name"]}`, "data/bad.yaml": "other: 1\n"})
	r := vtRun(t, root, ruleEditorConfig, nil)
	for _, c := range r.Checks {
		require.NotEqual(t, "schema/docs", c.ID)
	}
}

func TestCheckSchemaFilesHonorsCancellationAndReadErrors(t *testing.T) {
	root := newProject(t)
	existing := projectFile(t, root, "data/a.yaml", "k: v\n")
	missing := filepath.Join(root, "data", "missing.yaml")
	v := validator.NewYAMLSchemaValidator(nil)
	check := func(paths ...string) schemaCheck {
		return schemaCheck{validator: v, source: "atmos://schema/atmos/manifest/1.0", paths: paths}
	}

	t.Run("cancelled context stops before reading", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		p := &project{root: root, extra: map[string][]byte{}, byPath: map[string]*document{}}
		err := (&Engine{FS: filesystem.NewOSFileSystem()}).checkSchemaFiles(ctx, p, &Options{}, &Report{}, check(existing))
		require.ErrorIs(t, err, context.Canceled)
		require.Empty(t, p.extra)
	})
	t.Run("unreadable file is an operational error", func(t *testing.T) {
		p := &project{root: root, extra: map[string][]byte{}, byPath: map[string]*document{}}
		err := (&Engine{FS: filesystem.NewOSFileSystem()}).checkSchemaFiles(context.Background(), p, &Options{}, &Report{}, check(missing))
		require.ErrorIs(t, err, fs.ErrNotExist)
	})
	t.Run("files outside discovered documents join the source snapshot", func(t *testing.T) {
		p := &project{root: root, extra: map[string][]byte{}, byPath: map[string]*document{}}
		require.NoError(t, (&Engine{FS: filesystem.NewOSFileSystem()}).checkSchemaFiles(context.Background(), p, &Options{}, &Report{}, check(existing)))
		require.Equal(t, []byte("k: v\n"), p.extra[existing])
	})
	t.Run("already discovered documents are not duplicated", func(t *testing.T) {
		p := &project{root: root, extra: map[string][]byte{}, byPath: map[string]*document{existing: {path: existing}}}
		require.NoError(t, (&Engine{FS: filesystem.NewOSFileSystem()}).checkSchemaFiles(context.Background(), p, &Options{}, &Report{}, check(existing)))
		require.Empty(t, p.extra)
	})
}
