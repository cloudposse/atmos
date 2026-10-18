package doctor

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/filesystem"
	"github.com/cloudposse/atmos/pkg/validation"
)

// ecBroken is YAML that cannot be parsed, so a "loaded" source is observable as a finding.
const ecBroken = "ec-broken: [\n"

// ecRun runs the engine with a fresh backup directory and requires success.
func ecRun(t *testing.T, o *Options) Report {
	t.Helper()
	r, err := testEngine(t).Run(context.Background(), *o)
	require.NoError(t, err)
	return r
}

// ecLoaded returns the sorted project-relative files whose YAML failed to parse, which is
// how these tests observe which sources discovery actually read.
func ecLoaded(r *Report) []string {
	files := []string{}
	for i := range r.Findings {
		f := &r.Findings[i]
		if f.RuleID == ruleSchema && f.Status == statusManual && f.Severity == validation.SeverityError && f.Path == "" {
			files = append(files, f.File)
		}
	}
	sort.Strings(files)
	return files
}

func ecFindings(r *Report, rule string) []Finding {
	var found []Finding
	for i := range r.Findings {
		if r.Findings[i].RuleID == rule {
			found = append(found, r.Findings[i])
		}
	}
	return found
}

func ecCheck(r *Report, id string) *Check {
	for i := range r.Checks {
		if r.Checks[i].ID == id {
			return &r.Checks[i]
		}
	}
	return nil
}

func ecIncomplete(r *Report, reason string) []Finding {
	var found []Finding
	findings := ecFindings(r, ruleImports)
	for i := range findings {
		if findings[i].Status == statusVerificationIncomplete && findings[i].Reason == reason {
			found = append(found, findings[i])
		}
	}
	return found
}

func TestEcParseRejectsUnsupportedDocuments(t *testing.T) {
	node, err := parse([]byte("a: 1\n"))
	require.NoError(t, err)
	require.NotNil(t, node)

	_, err = parse([]byte("a: 1\n---\nb: 2\n"))
	require.ErrorIs(t, err, ErrVerification)

	// Duplicate mapping keys decode as a YAML node but fail typed decoding.
	_, err = parse([]byte("a: 1\na: 2\n"))
	require.Error(t, err)
	require.NotErrorIs(t, err, ErrVerification)

	_, err = parse([]byte("a: [\n"))
	require.Error(t, err)

	node, err = parse(nil)
	require.NoError(t, err)
	require.NotNil(t, node)
}

func TestEcValidateOptionsRejectsInvalidInvocations(t *testing.T) {
	for _, tt := range []struct {
		name      string
		options   Options
		isOptions bool
		message   string
	}{
		{"dry run requires fix", Options{DryRun: true}, true, "--dry-run requires --fix"},
		{"unknown check selector", Options{Only: []string{"bogus"}}, true, `unknown check "bogus"`},
		{"known selector among unknown", Options{Only: []string{ruleSchema, "bogus"}}, true, `unknown check "bogus"`},
		{"malformed exclude glob", Options{Exclude: []string{"["}}, false, "exclude"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tt.options.Root = newProject(t)
			r, err := testEngine(t).Run(context.Background(), tt.options)
			require.Error(t, err)
			require.Equal(t, tt.isOptions, errors.Is(err, ErrOptions))
			require.Contains(t, err.Error(), tt.message)
			require.Equal(t, err.Error(), r.OperationalError)
			require.Equal(t, 2, r.ExitCode("none"))
			require.Empty(t, r.ChangedFiles)
		})
	}

	t.Run("rule ids and categories are accepted", func(t *testing.T) {
		root := newProject(t)
		projectFile(t, root, "atmos.yaml", "{}\n")
		for _, only := range []string{ruleHookEvent, categoryDeprecations, ruleSourceRepository, "sources"} {
			ecRun(t, &Options{Root: root, Offline: true, Only: []string{only}})
		}
	})
}

func TestEcConfigureResolvesTheRoot(t *testing.T) {
	t.Run("defaults to the working directory", func(t *testing.T) {
		root := newProject(t)
		projectFile(t, root, "atmos.yaml", ecBroken)
		t.Chdir(root)
		r := ecRun(t, &Options{Offline: true, Only: []string{ruleSchema, ruleImports}})
		require.Equal(t, []string{"atmos.yaml"}, ecLoaded(&r))
	})

	t.Run("missing root is an operational error", func(t *testing.T) {
		r, err := testEngine(t).Run(context.Background(), Options{Root: filepath.Join(newProject(t), "missing")})
		require.ErrorIs(t, err, fs.ErrNotExist)
		require.Equal(t, 2, r.ExitCode("none"))
	})

	t.Run("missing selection root is an operational error", func(t *testing.T) {
		root := newProject(t)
		r, err := testEngine(t).Run(context.Background(), Options{Root: root, Affected: true, Selected: []string{"atmos.yaml"}, SelectedRoot: filepath.Join(root, "missing")})
		require.ErrorIs(t, err, fs.ErrNotExist)
		require.Equal(t, 2, r.ExitCode("none"))
	})
}

func TestEcAffectedSelectionBroadensForConfigAndStackChanges(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", "{}\n")
	projectFile(t, root, "stacks/dev.yaml", "vars: {}\n")
	projectFile(t, root, ".github/workflows/changed.yaml", legacyActionWorkflow)
	projectFile(t, root, ".github/workflows/unchanged.yaml", legacyActionWorkflow)
	workflows := []string{".github/workflows/changed.yaml", ".github/workflows/unchanged.yaml"}

	for _, tt := range []struct {
		name     string
		selected []string
		want     []string
	}{
		{"workflow-only change keeps narrowing", []string{".github/workflows/changed.yaml"}, workflows[:1]},
		{"config change checks every document", []string{"atmos.yaml", ".github/workflows/changed.yaml"}, workflows},
		{"config layer change checks every document", []string{"atmos.d/extra.yaml"}, workflows},
		{"stack change checks every document", []string{filepath.Join("stacks", "dev.yaml")}, workflows},
		{"unrelated file keeps narrowing", []string{"README.md"}, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := ecRun(t, &Options{Root: root, Offline: true, Only: []string{ruleModernization}, Affected: true, Selected: tt.selected})
			require.Equal(t, tt.want, modernizationFiles(t, &r))
		})
	}
}

func TestEcDiscoveryReportsUnverifiedAndMissingConfiguration(t *testing.T) {
	t.Run("unverified inputs make verification incomplete", func(t *testing.T) {
		root := newProject(t)
		projectFile(t, root, "atmos.yaml", "components:\n  terraform:\n    init_run_reconfigure: false\n")
		r := ecRun(t, &Options{Root: root, Offline: true, Fix: true, Only: []string{categoryDeprecations}, UnverifiedInputs: []string{"CLI override was not captured"}})
		found := ecIncomplete(&r, "CLI override was not captured")
		require.Len(t, found, 1)
		require.Equal(t, "Configuration verification incomplete", found[0].Message)
		require.Empty(t, found[0].File)
		require.Empty(t, r.ChangedFiles, "an unverified input must prevent repairs")
	})

	t.Run("profiles and multiple layers require effective-input verification", func(t *testing.T) {
		const reason = "Multiple configuration layers or profiles require effective-input verification"
		root := newProject(t)
		projectFile(t, root, "atmos.yaml", "{}\n")
		r := ecRun(t, &Options{Root: root, Offline: true, Only: []string{ruleSchema, ruleImports}, Profiles: []string{"ci"}})
		require.Len(t, ecIncomplete(&r, reason), 1)

		projectFile(t, root, "atmos.d/extra.yaml", "{}\n")
		r = ecRun(t, &Options{Root: root, Offline: true, Only: []string{ruleSchema, ruleImports}})
		require.Len(t, ecIncomplete(&r, reason), 1)
	})

	t.Run("single configuration without profiles is complete", func(t *testing.T) {
		root := newProject(t)
		projectFile(t, root, "atmos.yaml", "{}\n")
		r := ecRun(t, &Options{Root: root, Offline: true, Only: []string{ruleSchema, ruleImports}})
		require.Empty(t, r.Findings)
	})

	t.Run("missing configuration is an operational error with guidance", func(t *testing.T) {
		root := newProject(t)
		for _, only := range [][]string{nil, {ruleSchema}, {ruleModernization}} {
			r, err := testEngine(t).Run(context.Background(), Options{Root: root, Offline: true, Only: only})
			require.ErrorIs(t, err, ErrNoConfiguration)
			require.Contains(t, err.Error(), "--config")
			require.Contains(t, err.Error(), "ATMOS_CLI_CONFIG_PATH")
			require.Equal(t, 2, r.ExitCode("none"))
			require.Contains(t, r.OperationalError, "no project Atmos configuration found")
		}
	})
}

func TestEcConfigSourcesFollowTheLoaderOrder(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", "{}\n")
	for _, path := range []string{"atmos.d/a.yaml", ".atmos.d/b.yml", "profiles/dev/c.yaml", "profiles/ci/d.yaml", "extra/atmos.yaml", "node_modules/skipped.yaml", "atmos.d/notes.txt"} {
		projectFile(t, root, path, ecBroken)
	}

	t.Run("atmos.d layers load and inactive profiles do not", func(t *testing.T) {
		r := ecRun(t, &Options{Root: root, Offline: true, Only: []string{ruleSchema, ruleImports}})
		require.Equal(t, []string{".atmos.d/b.yml", "atmos.d/a.yaml"}, ecLoaded(&r))
	})

	t.Run("only active profiles load", func(t *testing.T) {
		r := ecRun(t, &Options{Root: root, Offline: true, Only: []string{ruleSchema, ruleImports}, Profiles: []string{"ci"}})
		require.Equal(t, []string{".atmos.d/b.yml", "atmos.d/a.yaml", "profiles/ci/d.yaml"}, ecLoaded(&r))
	})

	t.Run("a missing active profile is reported", func(t *testing.T) {
		r := ecRun(t, &Options{Root: root, Offline: true, Only: []string{ruleSchema, ruleImports}, Profiles: []string{"nope"}})
		var messages []string
		for _, f := range ecFindings(&r, ruleImports) {
			messages = append(messages, f.Message)
		}
		require.Contains(t, messages, "Active profile was not found: nope")
	})

	t.Run("a config directory selection replaces discovery", func(t *testing.T) {
		r := ecRun(t, &Options{Root: root, Offline: true, Only: []string{ruleSchema, ruleImports}, ConfigDirs: []string{"extra"}})
		require.Equal(t, []string{"extra/atmos.yaml"}, ecLoaded(&r))
	})
}

func TestEcExplicitConfigFilesReplaceDefaultDiscovery(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", ecBroken)
	projectFile(t, root, "custom/main.yaml", ecBroken)
	absolute := projectFile(t, root, "custom/other.yaml", ecBroken)
	r := ecRun(t, &Options{Root: root, Offline: true, Only: []string{ruleSchema, ruleImports}, ConfigFiles: []string{filepath.Join("custom", "main.yaml"), absolute}})
	require.Equal(t, []string{"custom/main.yaml", "custom/other.yaml"}, ecLoaded(&r))
}

func TestEcMissingExplicitConfigFileIsReported(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", "{}\n")
	missing := filepath.Join(root, "missing.yaml")

	t.Run("alone it is the no-project error naming the path", func(t *testing.T) {
		r, err := testEngine(t).Run(context.Background(), Options{Root: root, Offline: true, Only: []string{ruleSchema, ruleImports}, ConfigFiles: []string{"missing.yaml"}})
		require.ErrorIs(t, err, ErrNoConfiguration)
		require.Contains(t, r.OperationalError, "Configured path does not exist: "+missing+" (from --config)")
		require.Equal(t, 2, r.ExitCode("error"))
	})

	t.Run("beside a real file it is an error finding", func(t *testing.T) {
		r := ecRun(t, &Options{Root: root, Offline: true, Only: []string{ruleSchema, ruleImports}, ConfigFiles: []string{"atmos.yaml", "missing.yaml"}})
		found := ecFindings(&r, ruleImports)
		require.Len(t, found, 1)
		require.Equal(t, "Configured path does not exist: "+missing+" (from --config)", found[0].Message)
		require.Equal(t, validation.SeverityError, found[0].Severity)
		require.Equal(t, 1, r.ExitCode("error"))
	})
}

// ecCancelAfterRead cancels the run once the first source has been read, which places the
// cancellation after configuration loading and before the vendor manifest walk.
type ecCancelAfterRead struct {
	filesystem.FileSystem
	cancel context.CancelFunc
}

func (f *ecCancelAfterRead) ReadFile(path string) ([]byte, error) {
	content, err := f.FileSystem.ReadFile(path)
	f.cancel()
	return content, err
}

func TestEcCancellationStopsDiscovery(t *testing.T) {
	for _, tt := range []struct {
		name  string
		start func(context.CancelFunc) *Engine
	}{
		{"while loading sources", func(cancel context.CancelFunc) *Engine { cancel(); return testEngine(t) }},
		{"while scanning for vendor manifests", func(cancel context.CancelFunc) *Engine {
			e := testEngine(t)
			e.FS = &ecCancelAfterRead{FileSystem: filesystem.NewOSFileSystem(), cancel: cancel}
			return e
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := newProject(t)
			projectFile(t, root, "atmos.yaml", "{}\n")
			projectFile(t, root, "vendor.yaml", "spec: {}\n")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			r, err := tt.start(cancel).Run(ctx, Options{Root: root, Offline: true})
			require.ErrorIs(t, err, context.Canceled)
			require.Equal(t, 2, r.ExitCode("none"))
		})
	}
}

func TestEcProjectSourcesFollowConfiguredBasePaths(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", "base_path: infra\nstacks:\n  base_path: envs\n")
	projectFile(t, root, "infra/envs/dev.yaml", ecBroken)
	projectFile(t, root, "other/envs/prod.yaml", ecBroken)
	projectFile(t, root, "stacks/ignored.yaml", ecBroken)
	projectFile(t, root, "infra/stacks/ignored.yaml", ecBroken)

	t.Run("base_path and stacks.base_path from atmos.yaml", func(t *testing.T) {
		r := ecRun(t, &Options{Root: root, Offline: true, Only: []string{ruleSchema, ruleImports}})
		require.Equal(t, []string{"infra/envs/dev.yaml"}, ecLoaded(&r))
	})

	t.Run("the base path option overrides atmos.yaml", func(t *testing.T) {
		r := ecRun(t, &Options{Root: root, Offline: true, Only: []string{ruleSchema, ruleImports}, BasePath: "other"})
		require.Equal(t, []string{"other/envs/prod.yaml"}, ecLoaded(&r))
	})

	t.Run("the default stacks directory is stacks", func(t *testing.T) {
		clean := newProject(t)
		projectFile(t, clean, "atmos.yaml", "{}\n")
		projectFile(t, clean, "stacks/nested/dev.yml", ecBroken)
		r := ecRun(t, &Options{Root: clean, Offline: true, Only: []string{ruleSchema, ruleImports}})
		require.Equal(t, []string{"stacks/nested/dev.yml"}, ecLoaded(&r))
	})
}

func TestEcVendorSourcesFollowConfiguredBasePath(t *testing.T) {
	for _, tt := range []struct {
		name  string
		base  string
		files []string
		want  []string
	}{
		{"file", "manifests/custom-vendor.yaml", []string{"manifests/custom-vendor.yaml", "manifests/other.yaml"}, []string{"manifests/custom-vendor.yaml"}},
		{"directory", "vendor-dir", []string{"vendor-dir/a.yaml", "vendor-dir/sub/b.yml", "vendor-dir/node_modules/c.yaml"}, []string{"vendor-dir/a.yaml", "vendor-dir/sub/b.yml"}},
		{"missing path", "does-not-exist", []string{"unrelated.yaml"}, []string{}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := newProject(t)
			projectFile(t, root, "atmos.yaml", "vendor:\n  base_path: "+tt.base+"\n")
			for _, file := range tt.files {
				projectFile(t, root, file, ecBroken)
			}
			r := ecRun(t, &Options{Root: root, Offline: true, Only: []string{ruleSchema, ruleImports}})
			require.Equal(t, tt.want, ecLoaded(&r))
		})
	}
}

func TestEcDirectoryWalksSkipGeneratedAndDependencyDirectories(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", "{}\n")
	for _, path := range []string{"deep/vendor.yaml", "deep/component.yml", "stacks/ok.yaml", "stacks/.cache/dep.yaml"} {
		projectFile(t, root, path, ecBroken)
	}
	for _, path := range []string{"node_modules/pkg/vendor.yaml", ".git/vendor.yml", "build/component.yaml", ".terraform/vendor.yaml", "dist/vendor.yaml", "stacks/node_modules/dep.yaml", "other/readme.yaml"} {
		projectFile(t, root, path, ecBroken)
	}
	r := ecRun(t, &Options{Root: root, Offline: true, Only: []string{ruleSchema, ruleImports}})
	require.Equal(t, []string{"deep/component.yml", "deep/vendor.yaml", "stacks/.cache/dep.yaml", "stacks/ok.yaml"}, ecLoaded(&r), "directories inside the stacks base path are never skipped, except tool directories")
}

func TestEcLocalImportsResolveRelativeToTheDeclaringDocument(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", "import:\n  - path: optional-missing.yaml\n    skip_if_missing: true\n  - path: required-missing.yaml\n    skip_if_missing: false\n  - broken/*.yaml\n  - '{{ .dynamic }}'\n  - https://example.invalid/remote.yaml\n  - skip_if_missing: true\n  - path: nested/with-extension\n")
	projectFile(t, root, "broken/a.yaml", ecBroken)
	projectFile(t, root, "broken/b.yaml", ecBroken)
	projectFile(t, root, "nested/with-extension.yaml", ecBroken)
	projectFile(t, root, "atmos.d/layer.yaml", "imports: ../shared/common\n")
	projectFile(t, root, "shared/common.yaml", ecBroken)

	r := ecRun(t, &Options{Root: root, Offline: true, Only: []string{ruleSchema, ruleImports}})
	require.Equal(t, []string{"broken/a.yaml", "broken/b.yaml", "nested/with-extension.yaml", "shared/common.yaml"}, ecLoaded(&r))

	unresolved := []Finding{}
	for _, f := range ecFindings(&r, ruleImports) {
		if f.Status == statusManual {
			unresolved = append(unresolved, f)
		}
	}
	require.Len(t, unresolved, 1, "optional and path-less declarations must not be reported")
	require.Equal(t, "Local import does not resolve: required-missing.yaml", unresolved[0].Message)
	require.Equal(t, "atmos.yaml", unresolved[0].File)
	require.Equal(t, 4, unresolved[0].Line)
	require.Equal(t, validation.SeverityError, unresolved[0].Severity)

	dynamic := ecIncomplete(&r, "Dynamic or remote import was not evaluated")
	require.Len(t, dynamic, 2, "the templated and remote imports are not evaluated")
	for _, f := range dynamic {
		require.Equal(t, "atmos.yaml", f.File)
	}
}

func TestEcStackImportsResolveRelativeToTheStacksDirectory(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", "{}\n")
	projectFile(t, root, "stacks/dev.yaml", "import: ../shared/base\n")
	projectFile(t, root, "shared/base.yaml", ecBroken)
	r := ecRun(t, &Options{Root: root, Offline: true, Only: []string{ruleSchema, ruleImports}})
	require.Equal(t, []string{"shared/base.yaml"}, ecLoaded(&r))
}

func TestEcSourcesOutsideTheProjectAreNotRead(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", "{}\n")
	outside := projectFile(t, newProject(t), "outside.yaml", ecBroken)
	r := ecRun(t, &Options{Root: root, Offline: true, Only: []string{ruleSchema, ruleImports}, ConfigFiles: []string{"atmos.yaml", outside}})
	require.Empty(t, ecLoaded(&r))
	found := ecIncomplete(&r, "Source is outside the project; it will not be edited")
	require.Len(t, found, 1)
	require.True(t, strings.HasSuffix(found[0].File, "outside.yaml"), found[0].File)
}

func TestEcSymlinkedSourcesRequireManualReview(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", "{}\n")
	projectFile(t, root, "real/inside.yaml", ecBroken)
	outside := projectFile(t, newProject(t), "outside.yaml", ecBroken)
	if err := os.Symlink(filepath.Join(root, "real", "inside.yaml"), filepath.Join(root, "inside-link.yaml")); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}
	require.NoError(t, os.Symlink(outside, filepath.Join(root, "outside-link.yaml")))

	r := ecRun(t, &Options{Root: root, Offline: true, Only: []string{ruleSchema, ruleImports}, ConfigFiles: []string{"inside-link.yaml", "outside-link.yaml"}})
	found := ecIncomplete(&r, "Symlinked sources require manual review")
	files := []string{}
	for _, f := range found {
		files = append(files, f.File)
	}
	sort.Strings(files)
	require.Equal(t, []string{"inside-link.yaml", "outside-link.yaml"}, files)
	// Only the link whose target stays inside the project is read.
	require.Equal(t, []string{"inside-link.yaml"}, ecLoaded(&r))
}

func TestEcExcludedSourcesAreNotRead(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", "{}\n")
	projectFile(t, root, "stacks/skip.yaml", ecBroken)
	projectFile(t, root, "stacks/keep.yaml", ecBroken)
	projectFile(t, root, "stacks/generated/gen.yaml", ecBroken)

	r := ecRun(t, &Options{Root: root, Offline: true, Only: []string{ruleSchema, ruleImports}})
	require.Equal(t, []string{"stacks/generated/gen.yaml", "stacks/keep.yaml", "stacks/skip.yaml"}, ecLoaded(&r))

	r = ecRun(t, &Options{Root: root, Offline: true, Only: []string{ruleSchema, ruleImports}, Exclude: []string{"stacks/skip.yaml", "stacks/generated/**"}})
	require.Equal(t, []string{"stacks/keep.yaml"}, ecLoaded(&r))
}

func TestEcConfiguredStackManifestSchema(t *testing.T) {
	t.Run("a local manifest schema validates stack manifests", func(t *testing.T) {
		root := newProject(t)
		projectFile(t, root, "atmos.yaml", "schemas:\n  atmos:\n    manifest: stack-schema.json\n")
		projectFile(t, root, "stack-schema.json", `{"type":"object","required":["vars"]}`)
		projectFile(t, root, "stacks/missing.yaml", "components: {}\n")
		projectFile(t, root, "stacks/valid.yaml", "vars: {}\n")
		r := ecRun(t, &Options{Root: root, Offline: true, Only: []string{ruleSchema}})
		var files []string
		for _, f := range ecFindings(&r, ruleSchema) {
			if f.Severity == validation.SeverityError {
				files = append(files, f.File)
			}
		}
		require.Equal(t, []string{"stacks/missing.yaml"}, files)
	})

	t.Run("a remote manifest schema is reported as incomplete coverage", func(t *testing.T) {
		root := newProject(t)
		projectFile(t, root, "atmos.yaml", "schemas:\n  atmos:\n    manifest: https://example.invalid/stack.json\n")
		projectFile(t, root, "stacks/dev.yaml", "vars: {}\n")
		r := ecRun(t, &Options{Root: root, Offline: true, Only: []string{ruleSchema}})
		check := ecCheck(&r, "schema/atmos")
		require.NotNil(t, check)
		require.Equal(t, "incomplete", check.Status)
		require.Contains(t, check.Reason, "Remote schema")
		for _, f := range ecFindings(&r, ruleSchema) {
			require.NotEqual(t, "stacks/dev.yaml", f.File, "an uncaptured schema must not validate the stack")
		}
	})

	t.Run("a missing local manifest schema is an operational error", func(t *testing.T) {
		root := newProject(t)
		projectFile(t, root, "atmos.yaml", "schemas:\n  atmos:\n    manifest: missing-schema.json\n")
		projectFile(t, root, "stacks/dev.yaml", "vars: {}\n")
		r, err := testEngine(t).Run(context.Background(), Options{Root: root, Offline: true, Only: []string{ruleSchema}})
		require.ErrorIs(t, err, fs.ErrNotExist)
		require.Equal(t, 2, r.ExitCode("none"))
	})
}

func TestEcFinishReportNormalizesFindings(t *testing.T) {
	root := newProject(t)
	r := Report{Findings: []Finding{
		{RuleID: "z", File: filepath.Join(root, "stacks", "b.yaml")},
		{RuleID: "a", File: "stacks/a.yaml", Documentation: "https://example.invalid/doc", Remediation: "Do the thing"},
	}}
	finishReport(&r, root, nil)
	require.Empty(t, r.OperationalError)
	require.Equal(t, "stacks/a.yaml", r.Findings[0].File)
	require.Equal(t, "https://example.invalid/doc", r.Findings[0].Documentation)
	require.Equal(t, "Do the thing", r.Findings[0].Remediation)
	require.Equal(t, "stacks/b.yaml", r.Findings[1].File, "absolute files are reported relative to the root")
	require.Equal(t, "https://atmos.tools/cli/commands/doctor", r.Findings[1].Documentation)
	require.Equal(t, "Review the declaration and the rule documentation", r.Findings[1].Remediation)

	finishReport(&r, root, errors.New("boom"))
	require.Equal(t, "boom", r.OperationalError)
}

func TestEcScanCoverageReportsPerRuleStatus(t *testing.T) {
	checks := func(r *Report, o *Options) map[string]Check {
		appendScanCoverage(o, r, 3)
		result := map[string]Check{}
		for _, c := range r.Checks {
			result[c.ID] = c
		}
		return result
	}

	t.Run("all scan rules are reported with the checked file count", func(t *testing.T) {
		got := checks(&Report{}, &Options{})
		for _, id := range []string{ruleComponentDependencies, ruleSchema, ruleYAMLFunctions, ruleDeprecatedField, ruleCommandProvides, ruleHookEvent, ruleTerraformReconfigure, ruleModernization, ruleComponentMonorepo, ruleImports} {
			require.Equal(t, Check{ID: id, Status: statusPassed, Files: 3}, got[id])
		}
		require.Len(t, got, 10)
	})

	t.Run("warnings produce a findings status and errors produce failed", func(t *testing.T) {
		r := &Report{Findings: []Finding{
			{RuleID: ruleDeprecatedField, Severity: validation.SeverityWarning},
			{RuleID: ruleSchema, Severity: validation.SeverityError},
		}}
		got := checks(r, &Options{})
		require.Equal(t, statusFindings, got[ruleDeprecatedField].Status)
		require.Equal(t, "failed", got[ruleSchema].Status)
		require.Equal(t, statusPassed, got[ruleImports].Status)
	})

	t.Run("selection limits the reported rules", func(t *testing.T) {
		got := checks(&Report{}, &Options{Only: []string{categoryDeprecations}})
		require.Len(t, got, 5)
		for _, id := range []string{ruleComponentDependencies, ruleDeprecatedField, ruleCommandProvides, ruleHookEvent, ruleTerraformReconfigure} {
			require.Contains(t, got, id)
		}
	})
}

func TestEcCancellationStopsDocumentChecks(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", "{}\n")
	projectFile(t, root, "stacks/dev.yaml", "vars: {}\n")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var checked []string
	e := testEngine(t)
	e.Observe = func(event Event) {
		if event.Phase == "Check" && event.Label != "" && !event.Done && strings.HasSuffix(event.Label, ".yaml") {
			checked = append(checked, event.Label)
			cancel()
		}
	}
	r, err := e.Run(ctx, Options{Root: root, Offline: true, Only: []string{ruleSchema}})
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, []string{"atmos.yaml"}, checked, "no document is checked after cancellation")
	require.Equal(t, 2, r.ExitCode("none"))
}

// TestEcEmittedFindingsAreProjectRelative keeps progress events aligned with their row labels. Rows
// are labeled relative to the project root, which can differ from the working directory when the
// configuration is found in a parent directory or the project is anchored at the Git root.
func TestEcEmittedFindingsAreProjectRelative(t *testing.T) {
	root := t.TempDir()
	var events []Event
	e := Engine{Observe: func(event Event) { events = append(events, event) }}

	e.emitFindings(root, []Finding{
		{File: filepath.Join(root, "deep", "vendor.yaml")},
		{File: "stacks/dev.yaml"},
		{},
	})

	require.Len(t, events, 3)
	require.Equal(t, "deep/vendor.yaml", events[0].Finding.File, "absolute paths become project-relative")
	require.Equal(t, "stacks/dev.yaml", events[1].Finding.File, "project-relative paths are unchanged")
	require.Empty(t, events[2].Finding.File)
}

// excludeNotices returns the notices that report --exclude patterns that matched nothing.
func excludeNotices(r *Report) []Finding {
	var found []Finding
	for i := range r.Findings {
		if r.Findings[i].RuleID == ruleExclude {
			found = append(found, r.Findings[i])
		}
	}
	return found
}

func TestExcludePatternsThatMatchNothingAreReported(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", "{}\n")
	projectFile(t, root, "stacks/skip.yaml", ecBroken)
	projectFile(t, root, "stacks/keep.yaml", ecBroken)

	tests := []struct {
		name    string
		exclude []string
		only    []string
		want    []string
	}{
		{"a matching pattern is silent", []string{"stacks/skip.yaml"}, []string{ruleSchema, ruleImports}, nil},
		{"a pattern that matches nothing is reported", []string{"nothing/**"}, []string{ruleSchema, ruleImports}, []string{"nothing/**"}},
		{"only the unmatched pattern is reported", []string{"stacks/skip.yaml", "nope.yaml"}, []string{ruleSchema, ruleImports}, []string{"nope.yaml"}},
		{"every unmatched pattern is reported in order", []string{"a/**", "stacks/*.yaml", "b/**"}, []string{ruleSchema}, []string{"a/**", "b/**"}},
		{"--only does not filter the notice", []string{"nothing/**"}, []string{ruleHookEvent}, []string{"nothing/**"}},
		{"no patterns, no notice", nil, []string{ruleSchema}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := ecRun(t, &Options{Root: root, Offline: true, Only: tt.only, Exclude: tt.exclude})
			var got []string
			for _, f := range excludeNotices(&r) {
				require.Equal(t, validation.SeverityNotice, f.Severity)
				require.Empty(t, f.File)
				require.Contains(t, f.Remediation, "relative to the project root")
				require.Contains(t, f.Message, "matched no project inputs; patterns are relative to the project root "+root)
				got = append(got, strings.Split(f.Message, `"`)[1])
			}
			require.Equal(t, tt.want, got)
		})
	}
}

func TestUnmatchedExcludeNoticeNeverTripsFailOnWarning(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", "{}\n")
	r := ecRun(t, &Options{Root: root, Offline: true, Only: []string{ruleSchema}, Exclude: []string{"nothing/**"}})
	require.Len(t, excludeNotices(&r), 1)
	require.Zero(t, r.ExitCode("warning"))
}

func TestExcludePatternMatchingOnlyAnEditorConfigFileIsNotReported(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", "{}\n")
	projectFile(t, root, ".editorconfig", "root = true\n[*]\nindent_style = space\n")
	projectFile(t, root, "docs/readme.txt", "hello\n")

	r := ecRun(t, &Options{Root: root, Offline: true, Only: []string{ruleEditorConfig}, Exclude: []string{"docs/readme.txt"}})
	require.Empty(t, excludeNotices(&r), "%+v", r.Findings)

	r = ecRun(t, &Options{Root: root, Offline: true, Only: []string{ruleEditorConfig}, Exclude: []string{"docs/missing.txt"}})
	require.Len(t, excludeNotices(&r), 1)
}
