package exec

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/cloudposse/atmos/pkg/component/configuration"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// revisionConfigurationRuntime composes the generated runtime and revision mocks,
// because a revision-capable provider implements both schema interfaces.
type revisionConfigurationRuntime struct {
	*MockComponentConfigurationRuntime
	*MockComponentConfigurationRevisionRuntime
}

func affectedConfigurationDefinitions(names ...string) map[string]map[string]any {
	components := map[string]any{}
	for _, name := range names {
		components[name] = map[string]any{"source": "./modules/unit", "vars": map[string]any{}}
	}
	return map[string]map[string]any{"live": {"name": "live", "components": map[string]any{"terraform": components}}}
}

func TestAffectedConfigurationRevisions(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		before, after        []string
		want                 string
		deleted              bool
		forkErr, discoverErr error
	}{
		{name: "addition", before: []string{"network", "app"}, after: []string{"network", "network-v2", "app"}, want: "network-v2"},
		{name: "removal", before: []string{"network", "old"}, after: []string{"network"}, want: "old", deleted: true},
		{name: "fork canceled", after: []string{"network"}, forkErr: context.Canceled},
		{name: "BASE discovery failed", after: []string{"network"}, discoverErr: errUtils.ErrPluginProtocol},
		{name: "unchanged", before: []string{"network", "app"}, after: []string{"network", "app"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			headRoot, head := affectedPolicyRepo(t, map[string]string{"README": "unchanged"})
			baseRoot, base := affectedPolicyRepo(t, map[string]string{"README": "unchanged"})
			config := affectedPolicyTestConfig(headRoot)
			config.TerraformDirAbsolutePath = filepath.Join(headRoot, "components", "terraform")
			config.HelmfileDirAbsolutePath = filepath.Join(headRoot, "components", "helmfile")
			config.PackerDirAbsolutePath = filepath.Join(headRoot, "components", "packer")
			config.Components.Terraform.ConfigurationProviders = map[string]schema.ComponentConfiguration{"terragrunt": {Enabled: true, Discovery: schema.ComponentDiscoveryConfig{Enabled: true}}}
			ctrl := gomock.NewController(t)
			current := NewMockComponentConfigurationRuntime(ctrl)
			current.EXPECT().Discover(gomock.Any()).Return(affectedConfigurationDefinitions(tc.after...), nil).AnyTimes()
			previous := NewMockComponentConfigurationRuntime(ctrl)
			previous.EXPECT().Discover(gomock.Any()).Return(affectedConfigurationDefinitions(tc.before...), tc.discoverErr).AnyTimes()
			forked := false
			revisions := NewMockComponentConfigurationRevisionRuntime(ctrl)
			revisions.EXPECT().ForRevision(baseRoot).DoAndReturn(func(string) (schema.ComponentConfigurationRuntime, error) {
				forked = true
				return previous, tc.forkErr
			}).Times(1)
			runtime := &revisionConfigurationRuntime{MockComponentConfigurationRuntime: current, MockComponentConfigurationRevisionRuntime: revisions}
			config.ComponentConfigurations = runtime
			affected, _, _, err := executeDescribeAffected(config, headRoot, baseRoot, head, base, false, false, "", false, false, nil, AffectedFilter{}, nil, true, DescribeStacksErrorOptions{})
			require.True(t, forked, "BASE must discover independently")
			require.Same(t, runtime, config.ComponentConfigurations, "restore HEAD for dependent expansion and execution")
			require.Equal(t, headRoot, config.BasePathAbsolute)
			if tc.forkErr != nil {
				require.ErrorIs(t, err, tc.forkErr)
				return
			}
			if tc.discoverErr != nil {
				require.ErrorIs(t, err, tc.discoverErr)
				return
			}
			require.NoError(t, err)
			if tc.want == "" {
				require.Empty(t, affected)
			} else {
				require.Len(t, affected, 1)
				require.Equal(t, tc.want, affected[0].Component)
				require.Equal(t, tc.deleted, affected[0].Deleted)
			}
		})
	}
}

func TestAffectedConfigurationForkErrors(t *testing.T) {
	require.NoError(t, rebaseConfigurationDiscovery(&schema.AtmosConfiguration{}))
	ctrl := gomock.NewController(t)
	original := NewMockComponentConfigurationRuntime(ctrl)
	config := &schema.AtmosConfiguration{ComponentConfigurations: original}
	require.ErrorIs(t, rebaseConfigurationDiscovery(config), errUtils.ErrPluginUnsupportedExtension)
	require.Same(t, original, config.ComponentConfigurations)
	for _, failure := range []error{context.Canceled, errUtils.ErrPluginInvalidConfig} {
		revisions := NewMockComponentConfigurationRevisionRuntime(ctrl)
		revisions.EXPECT().ForRevision(gomock.Any()).Return(nil, failure).Times(1)
		runtime := &revisionConfigurationRuntime{MockComponentConfigurationRuntime: original, MockComponentConfigurationRevisionRuntime: revisions}
		config.ComponentConfigurations = runtime
		require.ErrorIs(t, rebaseConfigurationDiscovery(config), failure)
		require.Same(t, runtime, config.ComponentConfigurations)
	}
}

// TestAffectedConfigurationWatchRootSpelling reproduces the Windows runner, where
// base_path resolves to the long form of TEMP (runneradmin) while changed files
// are joined to the 8.3 form the CLI's working directory uses (RUNNER~1). A
// symlinked repository root gives the same two spellings of one directory.
func TestAffectedConfigurationWatchRootSpelling(t *testing.T) {
	temp, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	repo := filepath.Join(temp, "repo")
	require.NoError(t, os.MkdirAll(filepath.Join(repo, "project", "modules", "unit"), 0o755))
	link := filepath.Join(temp, "link")
	if err := os.Symlink(repo, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	config := &schema.AtmosConfiguration{BasePathAbsolute: filepath.Join(repo, "project")}
	settings := map[string]any{"configuration": map[string]any{"provider": "terragrunt"}}
	watches := func() []schema.ComponentDependency {
		deps := []schema.ComponentDependency{{Kind: "folder", Path: "modules/unit"}}
		require.True(t, configuration.WatchPaths(config, settings, deps))
		return deps
	}

	match := func(index *changedFilesIndex, deps []schema.ComponentDependency, provider bool) (bool, string, string) {
		resolvedIndex, matched := configurationMatchInputs(index, deps, provider)
		changed, kind, path, err := isComponentDependentFolderOrFileChangedIndexed(resolvedIndex, matched)
		require.NoError(t, err)
		return changed, kind, configuration.ConfiguredDependencyPath(deps, matched, path)
	}

	deps := watches()
	index := newChangedFilesIndex(config, []string{"project/modules/unit/main.tf"}, link)
	changed, _, _, err := isComponentDependentFolderOrFileChangedIndexed(index, deps)
	require.NoError(t, err)
	require.False(t, changed, "precondition: the two spellings differ")

	changed, kind, path := match(index, deps, true)
	require.True(t, changed)
	require.Equal(t, "folder", kind)
	require.Equal(t, filepath.Join(repo, "project", "modules", "unit"), path, "report the configured spelling")

	other := newChangedFilesIndex(config, []string{"project/modules/other/main.tf"}, link)
	changed, _, _ = match(other, watches(), true)
	require.False(t, changed, "a change outside the watch must not select the unit")

	// YAML dependencies keep exact matching.
	changed, _, _ = match(index, watches(), false)
	require.False(t, changed)
}

// Native discovery starts during configuration initialization, so the command's cancellation must reach it.
func TestParseDescribeAffectedCliArgsPassesCommandContext(t *testing.T) {
	root := t.TempDir()
	config := `base_path: "."
components:
  terraform:
    base_path: components/terraform
    terragrunt:
      enabled: true
      discovery:
        enabled: true
        included_paths: ["live/**"]
stacks:
  base_path: stacks
  included_paths: ["**/*"]
  name_template: "{{ .vars.stage }}"
`
	require.NoError(t, os.WriteFile(filepath.Join(root, "atmos.yaml"), []byte(config), 0o600))
	t.Setenv("ATMOS_CLI_CONFIG_PATH", root)
	t.Setenv("ATMOS_BASE_PATH", root)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	cmd := &cobra.Command{Use: "affected"}
	cmd.Flags().AddFlagSet(newDescribeAffectedFlagSet())
	cmd.Flags().StringSlice("profile", nil, "")
	cmd.Flags().StringSlice("config-path", nil, "")
	cmd.Flags().StringSlice("config", nil, "")
	cmd.Flags().String("base-path", "", "")
	cmd.SetContext(ctx)
	_, err := ParseDescribeAffectedCliArgs(cmd, nil)
	require.ErrorIs(t, err, context.Canceled)
}

func TestResolvedChangedFilesPerIndex(t *testing.T) {
	first := &changedFilesIndex{allFiles: []string{filepath.Join(t.TempDir(), "first")}}
	second := &changedFilesIndex{allFiles: []string{filepath.Join(t.TempDir(), "second")}}
	const n = 20
	results := make(chan *changedFilesIndex, n)
	for range n {
		go func() { results <- resolvedChangedFiles(first) }()
	}
	initial := <-results
	for i := 1; i < n; i++ {
		require.Same(t, initial, <-results)
	}
	require.NotSame(t, initial, resolvedChangedFiles(second))
	require.Same(t, initial, resolvedChangedFiles(first))
}
