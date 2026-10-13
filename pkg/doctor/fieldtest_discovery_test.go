package doctor

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/config/homedir"
	"github.com/cloudposse/atmos/pkg/validation"
)

const (
	ftCustomLabelWorkflow = "name: ci\non: push\njobs:\n  a:\n    runs-on: [my-custom-runner]\n    steps:\n      - run: echo ok\n"
	ftComponentVendor     = "apiVersion: atmos/v1\nkind: ComponentVendorConfig\nspec:\n  source:\n    uri: github.com/cloudposse/terraform-aws-components.git//modules/vpc?ref=1.495.0\n    version: 1.495.0\n"
)

func TestConfigurationIsFoundFromAProjectSubdirectory(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", "base_path: ./infra\n")
	projectFile(t, root, "atmos.d/layer.yaml", ecBroken)
	projectFile(t, root, "infra/stacks/dev.yaml", ecBroken)
	projectFile(t, root, "infra/stacks/deep/nested/keep.txt", "x\n")
	projectFile(t, root, "deep/keep.txt", "x\n")

	for _, dir := range []string{"deep", filepath.Join("infra", "stacks", "deep", "nested")} {
		t.Run(dir, func(t *testing.T) {
			r := ecRun(t, &Options{Root: filepath.Join(root, dir), Offline: true, Only: []string{ruleSchema, ruleImports}})
			require.Equal(t, []string{"atmos.d/layer.yaml", "infra/stacks/dev.yaml"}, ecLoaded(&r),
				"paths stay project-root relative; atmos.d follows the configuration directory; base_path follows atmos.yaml")
		})
	}
}

func TestAffectedSelectionFollowsAProjectRootDiscoveredAboveTheWorkingDirectory(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", "{}\n")
	projectFile(t, root, "deep/vendor.yaml", ecBroken)
	projectFile(t, root, "other/vendor.yaml", ecBroken)

	r := ecRun(t, &Options{Root: filepath.Join(root, "deep"), Offline: true, Only: []string{ruleSchema}, Affected: true, Selected: []string{"vendor.yaml"}})
	require.Equal(t, []string{"deep/vendor.yaml"}, ecLoaded(&r), "selected paths are relative to the invocation directory and rebased onto the project root")

	r = ecRun(t, &Options{Root: filepath.Join(root, "deep"), Offline: true, Only: []string{ruleSchema}, Affected: true, Selected: []string{"deep/vendor.yaml"}})
	require.Empty(t, ecLoaded(&r), "deep/vendor.yaml relative to the invocation directory selects nothing")
}

func TestCliConfigPathEnvironmentSelectsTheProject(t *testing.T) {
	project := newProject(t)
	projectFile(t, project, "atmos.yaml", "base_path: .\n")
	projectFile(t, project, "stacks/dev.yaml", ecBroken)
	elsewhere := newProject(t)

	t.Setenv("ATMOS_CLI_CONFIG_PATH", project)
	r := ecRun(t, &Options{Root: elsewhere, Offline: true, Only: []string{ruleSchema, ruleImports}})
	require.Equal(t, []string{"stacks/dev.yaml"}, ecLoaded(&r))
}

func TestMissingConfigurationPointsAtHowToSelectOne(t *testing.T) {
	r, err := testEngine(t).Run(context.Background(), Options{Root: newProject(t), Offline: true})
	require.ErrorIs(t, err, ErrNoConfiguration)
	for _, hint := range []string{"project directory", "--config", "ATMOS_CLI_CONFIG_PATH"} {
		require.Contains(t, r.OperationalError, hint)
	}
	require.Equal(t, 2, r.ExitCode("none"))
}

func TestAmbientConfigurationMakesVerificationIncompleteButIsNeverRead(t *testing.T) {
	home := newProject(t)
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	// The loader's home lookup memoizes the directory; disable that so this test controls it.
	homedir.DisableCache = true
	t.Cleanup(func() { homedir.DisableCache = false; homedir.Reset() })
	projectFile(t, home, filepath.Join(".atmos", "atmos.yaml"), ecBroken)
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", "components:\n  terraform:\n    init_run_reconfigure: false\n")

	r := ecRun(t, &Options{Root: root, Offline: true, Fix: true, Only: []string{categoryDeprecations}})
	require.Empty(t, ecLoaded(&r))
	var reasons []string
	for _, f := range ecFindings(&r, ruleImports) {
		reasons = append(reasons, f.Reason)
	}
	require.Len(t, reasons, 1)
	require.Contains(t, reasons[0], "Ambient configuration")
	require.Contains(t, reasons[0], filepath.Join(home, ".atmos"))
	require.Empty(t, r.ChangedFiles, "an unverifiable merge input blocks repairs")
}

func TestOutsideConfigurationFilesCollapseIntoOneNotice(t *testing.T) {
	outside := newProject(t)
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", "{}\n")
	var files []string
	for _, name := range []string{"a.yaml", "b.yaml", "c.yaml"} {
		files = append(files, projectFile(t, outside, name, ecBroken))
	}
	r := ecRun(t, &Options{Root: root, Offline: true, Only: []string{ruleSchema, ruleImports}, ConfigFiles: append([]string{"atmos.yaml"}, files...)})
	require.Empty(t, ecLoaded(&r))
	notices := ecFindings(&r, ruleImports)
	require.Len(t, notices, 1)
	require.Contains(t, notices[0].Reason, "3 configuration files")
	require.Empty(t, notices[0].File)
}

func TestComponentVendorManifestSourceIsInspected(t *testing.T) {
	for _, tt := range []struct {
		name    string
		path    string
		content string
		want    bool
	}{
		{"component.yaml spec.source.uri", "components/terraform/vpc/component.yaml", ftComponentVendor, true},
		{"component.yml spec.source.uri", "components/terraform/vpc/component.yml", ftComponentVendor, true},
		{"spec.source without a uri", "components/terraform/vpc/component.yaml", "kind: ComponentVendorConfig\nspec:\n  source:\n    version: 1.0.0\n", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := newProject(t)
			projectFile(t, root, "atmos.yaml", "{}\n")
			projectFile(t, root, tt.path, tt.content)
			stub := &repositoryStub{health: RepositoryHealth{"archived", "Upstream repository is archived"}}
			e := testEngine(t)
			e.Repositories = stub
			e.Monorepo = identicalMonorepo(t)
			r, err := e.Run(context.Background(), Options{Root: root, Only: []string{ruleSourceRepository, ruleModernization}})
			require.NoError(t, err)
			if !tt.want {
				require.Empty(t, stub.calls)
				return
			}
			require.Equal(t, []string{"https://github.com/cloudposse/terraform-aws-components"}, stub.calls)
			found := vtFindingsByRule(&r, ruleSourceRepository)
			require.Len(t, found, 1)
			require.Equal(t, "spec.source.uri", found[0].Path)
			require.Equal(t, 5, found[0].Line)
			legacy := vtFindingsByRule(&r, ruleComponentMonorepo)
			require.Len(t, legacy, 1)
			require.Equal(t, monorepoMessage, legacy[0].Message)
			require.Equal(t, "available", legacy[0].Status)
			require.Equal(t, "spec.source.uri", legacy[0].Path)
			require.Equal(t, 5, legacy[0].Line)
			require.Equal(t, "findings", vtCheckByID(t, &r, ruleSourceRepository).Status)
		})
	}
}

func TestSourcePathAndModernizationKeys(t *testing.T) {
	for _, tt := range []struct {
		kind   string
		path   []string
		source bool
	}{
		{"vendor", []string{"spec", "source", "uri"}, true},
		{"vendor", []string{"spec", "source"}, true},
		{"vendor", []string{"spec", "sources", "0", "uri"}, true},
		{"vendor", []string{"spec", "other", "uri"}, false},
		{"vendor", []string{"source", "uri"}, false},
		{"stack", []string{"spec", "source", "uri"}, false},
	} {
		require.Equal(t, tt.source, sourcePath(tt.kind, tt.path), "%s %v", tt.kind, tt.path)
	}
	require.True(t, componentSourceKey([]string{"components", "terraform", "a", "source"}))
	require.True(t, componentSourceKey([]string{"spec", "source", "uri"}))
	require.False(t, componentSourceKey([]string{"spec", "targets", "uri"}))
	require.False(t, componentSourceKey([]string{"uri"}))
}

func TestChecksWithoutInputsAreSkippedNotPassed(t *testing.T) {
	t.Run("no source declarations", func(t *testing.T) {
		root := newProject(t)
		projectFile(t, root, "atmos.yaml", "{}\n")
		projectFile(t, root, "stacks/dev.yaml", "vars:\n  source: not-a-component-source\n")
		e := testEngine(t)
		e.Repositories = &repositoryStub{}
		r, err := e.Run(context.Background(), Options{Root: root, Only: []string{ruleSourceRepository}})
		require.NoError(t, err)
		check := vtCheckByID(t, &r, ruleSourceRepository)
		require.Equal(t, statusSkipped, check.Status)
		require.Zero(t, check.Files)
		require.NotEmpty(t, check.Reason)
	})

	t.Run("only local sources", func(t *testing.T) {
		root := newProject(t)
		projectFile(t, root, "atmos.yaml", "{}\n")
		projectFile(t, root, "stacks/dev.yaml", "components:\n  terraform:\n    app:\n      source: ../components/local\n")
		e := testEngine(t)
		e.Repositories = &repositoryStub{}
		r, err := e.Run(context.Background(), Options{Root: root, Only: []string{ruleSourceRepository}})
		require.NoError(t, err)
		require.Equal(t, statusSkipped, vtCheckByID(t, &r, ruleSourceRepository).Status)
	})

	t.Run("no selected documents", func(t *testing.T) {
		root := newProject(t)
		projectFile(t, root, "atmos.yaml", "{}\n")
		r := ecRun(t, &Options{Root: root, Offline: true, Affected: true, Selected: []string{"unrelated.txt"}, Only: []string{categoryDeprecations}})
		for _, id := range []string{ruleDeprecatedField, ruleCommandProvides, ruleHookEvent, ruleTerraformReconfigure} {
			check := vtCheckByID(t, &r, id)
			require.Equal(t, statusSkipped, check.Status, id)
			require.Zero(t, check.Files, id)
		}
	})

	t.Run("a custom schema that matches nothing", func(t *testing.T) {
		root := vtSchemaProject(t, "  custom:\n    schema: schema.json\n    matches:\n      - nothing/*.yaml\n", map[string]string{"schema.json": "{}"})
		r := vtRun(t, root, ruleSchema, nil)
		require.Equal(t, statusSkipped, vtCheckByID(t, &r, "schema/custom").Status)
	})
}

func TestActionlintHonorsRepositoryConfiguration(t *testing.T) {
	for _, tt := range []struct {
		name   string
		config string
		file   string
		want   bool
	}{
		{"no configuration", "", "", true},
		{"actionlint.yaml", "self-hosted-runner:\n  labels:\n    - my-custom-runner\n", "actionlint.yaml", false},
		{"actionlint.yml", "self-hosted-runner:\n  labels:\n    - my-custom-runner\n", "actionlint.yml", false},
		{"configuration for another label", "self-hosted-runner:\n  labels:\n    - something-else\n", "actionlint.yaml", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := newProject(t)
			projectFile(t, root, "atmos.yaml", "{}\n")
			projectFile(t, root, ".github/workflows/ci.yaml", ftCustomLabelWorkflow)
			if tt.file != "" {
				projectFile(t, root, filepath.Join(".github", tt.file), tt.config)
			}
			_, err := os.Stat(filepath.Join(root, ".git"))
			require.True(t, os.IsNotExist(err), "the fixture is deliberately not a Git repository")
			r := vtRun(t, root, ruleGitHubActions, nil)
			found := vtFindingsByRule(&r, ruleGitHubActions)
			if !tt.want {
				require.Empty(t, found)
				return
			}
			require.Len(t, found, 1)
			require.Equal(t, `label "my-custom-runner" is unknown`, found[0].Message)
			require.Contains(t, found[0].Remediation, ".github/actionlint.yaml")
			require.Contains(t, found[0].Remediation, "self-hosted-runner.labels")
			require.NotContains(t, found[0].Message, "available labels")
			require.Equal(t, 5, found[0].Line)
		})
	}
}

func TestActionlintConfigPrefersYamlAndIgnoresDirectories(t *testing.T) {
	root := newProject(t)
	require.Empty(t, actionlintConfig(root))
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".github", "actionlint.yaml"), 0o700))
	require.Empty(t, actionlintConfig(root), "a directory is not a configuration file")
	yml := projectFile(t, root, ".github/actionlint.yml", "{}\n")
	require.Equal(t, yml, actionlintConfig(root))
}

func TestAppendActionDiagnosticsOnlyRewritesUnknownLabels(t *testing.T) {
	r := &Report{}
	appendActionDiagnostics(r, validation.Report{Diagnostics: []validation.Diagnostic{
		{Source: ruleGitHubActions, Severity: validation.SeverityError, Message: `label "x" is unknown. available labels are "a", "b". if it is a custom label for self-hosted runner, set list of labels in actionlint.yaml config file`},
		{Source: ruleGitHubActions, Severity: validation.SeverityError, Message: `undefined variable "nope"`},
	}})
	require.Len(t, r.Findings, 2)
	require.Equal(t, `label "x" is unknown`, r.Findings[0].Message)
	require.Equal(t, unknownLabelRemedy, r.Findings[0].Remediation)
	require.Equal(t, `undefined variable "nope"`, r.Findings[1].Message)
	require.Empty(t, r.Findings[1].Remediation)
}

func TestOfflineNoticesDoNotCountTowardFailOnWarning(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", "{}\n")
	projectFile(t, root, "stacks/dev.yaml", "components:\n  terraform:\n    app:\n      source: github.com/example/repo.git//modules/app?ref=v1\n")

	t.Run("offline is informational but stays in the report", func(t *testing.T) {
		stub := &repositoryStub{}
		e := testEngine(t)
		e.Repositories = stub
		r, err := e.Run(context.Background(), Options{Root: root, Offline: true, Only: []string{"sources"}})
		require.NoError(t, err)
		require.Len(t, r.Findings, 1)
		f := r.Findings[0]
		require.Equal(t, repositoryOfflineMessage, f.Message)
		require.Equal(t, validation.SeverityNotice, f.Severity)
		require.Equal(t, statusVerificationIncomplete, f.Status)
		for _, threshold := range []string{"none", "error", "warning"} {
			require.Zero(t, r.ExitCode(threshold), threshold)
		}
		require.Equal(t, validation.SeverityNotice, r.ValidationReport().Diagnostics[0].Severity)
	})

	t.Run("an unreachable host still warns", func(t *testing.T) {
		e := testEngine(t)
		e.Repositories = &repositoryStub{health: RepositoryHealth{repositoryInconclusive, "rate limited"}}
		r, err := e.Run(context.Background(), Options{Root: root, Only: []string{"sources"}})
		require.NoError(t, err)
		require.Equal(t, validation.SeverityWarning, r.Findings[0].Severity)
		require.Equal(t, 1, r.ExitCode("warning"))
		require.Zero(t, r.ExitCode("error"))
	})

	t.Run("dynamic sources still warn", func(t *testing.T) {
		dynamic := newProject(t)
		projectFile(t, dynamic, "atmos.yaml", "{}\n")
		projectFile(t, dynamic, "stacks/dev.yaml", "components:\n  terraform:\n    app:\n      source: '{{ .vars.source }}'\n")
		r := ecRun(t, &Options{Root: dynamic, Offline: true, Only: []string{"sources"}})
		require.Equal(t, validation.SeverityWarning, r.Findings[0].Severity)
		require.Equal(t, 1, r.ExitCode("warning"))
	})
}
