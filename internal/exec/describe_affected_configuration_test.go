package exec

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

type revisionConfigurationRuntime struct {
	*MockComponentConfigurationRuntime
	revision func(string) (schema.ComponentConfigurationRuntime, error)
}

func (r *revisionConfigurationRuntime) ForRevision(root string) (schema.ComponentConfigurationRuntime, error) {
	return r.revision(root)
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
			runtime := &revisionConfigurationRuntime{MockComponentConfigurationRuntime: current, revision: func(root string) (schema.ComponentConfigurationRuntime, error) {
				require.Equal(t, baseRoot, root)
				forked = true
				return previous, tc.forkErr
			}}
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
	original := NewMockComponentConfigurationRuntime(gomock.NewController(t))
	config := &schema.AtmosConfiguration{ComponentConfigurations: original}
	require.ErrorIs(t, rebaseConfigurationDiscovery(config), errUtils.ErrPluginUnsupportedExtension)
	require.Same(t, original, config.ComponentConfigurations)
	for _, failure := range []error{context.Canceled, errUtils.ErrPluginInvalidConfig} {
		runtime := &revisionConfigurationRuntime{MockComponentConfigurationRuntime: original, revision: func(string) (schema.ComponentConfigurationRuntime, error) { return nil, failure }}
		config.ComponentConfigurations = runtime
		require.ErrorIs(t, rebaseConfigurationDiscovery(config), failure)
		require.Same(t, runtime, config.ComponentConfigurations)
	}
}

func TestAffectedConfigurationWatchRoot(t *testing.T) {
	root := t.TempDir()
	config := &schema.AtmosConfiguration{BasePathAbsolute: root}
	for _, provider := range []bool{true, false} {
		settings := map[string]any{}
		if provider {
			settings["configuration"] = map[string]any{"provider": "terragrunt"}
		}
		absolute := filepath.Join(root, "module")
		deps := []schema.ComponentDependency{{Kind: "file", Path: "root.hcl"}, {Kind: "folder", Path: absolute}}
		configurationWatchPaths(config, settings, deps)
		require.Equal(t, absolute, deps[1].Path)
		if provider {
			require.Equal(t, filepath.Join(root, "root.hcl"), deps[0].Path)
			changed, kind, _, err := isComponentDependentFolderOrFileChangedIndexed(newChangedFilesIndex(config, []string{"root.hcl"}, root), deps)
			require.NoError(t, err)
			require.True(t, changed)
			require.Equal(t, "file", kind)
		} else {
			require.Equal(t, "root.hcl", deps[0].Path)
		}
	}
}

// watchFixture builds a project with many units that each watch their own files and folder,
// plus a changed-file set that touches only some of them.
func watchFixture(t testing.TB, units, changed int) (*changedFilesIndex, [][]schema.ComponentDependency) {
	t.Helper()
	root := t.TempDir()
	config := &schema.AtmosConfiguration{BasePathAbsolute: root, BasePath: root}
	files := make([]string, 0, changed)
	for i := 0; i < changed; i++ {
		files = append(files, filepath.Join("docs", "generated", "file-"+itoa(i)+".md"))
	}
	files = append(files, filepath.Join("live", "unit-7", "extra.tfvars"), filepath.Join("modules", "shared", "deep", "main.tf"))
	deps := make([][]schema.ComponentDependency, units)
	for i := range deps {
		unit := filepath.Join(root, "live", "unit-"+itoa(i))
		deps[i] = []schema.ComponentDependency{
			{Kind: "file", Path: filepath.Join(unit, "terragrunt.hcl")},
			{Kind: "file", Path: filepath.Join(unit, "extra.tfvars")},
			{Kind: "folder", Path: filepath.Join(root, "modules", "mod-"+itoa(i%10))},
		}
	}
	// Unit 3 consumes the shared module folder, unit 4 watches a glob.
	deps[3] = append(deps[3], schema.ComponentDependency{Kind: "folder", Path: filepath.Join(root, "modules", "shared")})
	deps[4] = append(deps[4], schema.ComponentDependency{Kind: "folder", Path: filepath.Join(root, "docs", "generated", "file-1[0-9].md")})
	return newChangedFilesIndex(config, files, root), deps
}

func itoa(i int) string { return strconv.Itoa(i) }

func TestFileWatchSelectionUsesIndex(t *testing.T) {
	index, deps := watchFixture(t, 300, 9000)
	var selected []int
	for unit := range deps {
		changed, _, _, err := isComponentDependentFolderOrFileChangedIndexed(index, deps[unit])
		require.NoError(t, err)
		if changed {
			selected = append(selected, unit)
		}
	}
	// Unit 3 watches the changed shared module, unit 7 has a changed var file, and unit 4 watches
	// a glob that matches generated documents: literal and glob watches agree on the result.
	require.Equal(t, []int{3, 4, 7}, selected)

	// Literal watches are answered from the index: only the one glob watch scans the change set.
	// Matching every watch against every changed file made selection grow with units times files.
	scans := watchPatternScans.Load()
	for unit := range deps {
		_, _, _, err := isComponentDependentFolderOrFileChangedIndexed(index, deps[unit])
		require.NoError(t, err)
	}
	require.LessOrEqual(t, watchPatternScans.Load()-scans, int64(1), "literal watches must not scan every changed file")
}

func TestFileWatchFolderSemantics(t *testing.T) {
	root := t.TempDir()
	config := &schema.AtmosConfiguration{BasePathAbsolute: root, BasePath: root}
	index := newChangedFilesIndex(config, []string{filepath.Join("modules", "a", "b", "main.tf"), "top.md"}, root)
	for _, tc := range []struct {
		name string
		dep  schema.ComponentDependency
		want bool
	}{
		{"folder contains change", schema.ComponentDependency{Kind: "folder", Path: filepath.Join(root, "modules", "a")}, true},
		{"nested folder", schema.ComponentDependency{Kind: "folder", Path: filepath.Join(root, "modules", "a", "b")}, true},
		{"project root", schema.ComponentDependency{Kind: "folder", Path: root}, true},
		{"sibling prefix is not contained", schema.ComponentDependency{Kind: "folder", Path: filepath.Join(root, "modules", "a", "b", "mai")}, false},
		{"other folder", schema.ComponentDependency{Kind: "folder", Path: filepath.Join(root, "modules", "c")}, false},
		{"exact file", schema.ComponentDependency{Kind: "file", Path: filepath.Join(root, "top.md")}, true},
		{"file is not a folder prefix", schema.ComponentDependency{Kind: "file", Path: filepath.Join(root, "modules")}, false},
		{"glob folder", schema.ComponentDependency{Kind: "folder", Path: filepath.Join(root, "modules", "*")}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed, _, _, err := isComponentDependentFolderOrFileChangedIndexed(index, []schema.ComponentDependency{tc.dep})
			require.NoError(t, err)
			require.Equal(t, tc.want, changed)
		})
	}
}

func TestConfigurationWatchIndexIsBuiltOncePerExclusionSet(t *testing.T) {
	root := t.TempDir()
	config := &schema.AtmosConfiguration{BasePathAbsolute: root, BasePath: root}
	index := newChangedFilesIndex(config, []string{"live/a/.terragrunt-cache/x.tf", "live/a/main.hcl"}, root)
	settings := map[string]any{"configuration": map[string]any{"watch_excludes": []any{"**/.terragrunt-cache/**"}}}
	first, err := configurationWatchIndex(config, settings, index)
	require.NoError(t, err)
	second, err := configurationWatchIndex(config, settings, index)
	require.NoError(t, err)
	require.Same(t, first, second)
	require.Equal(t, []string{filepath.Join(root, "live", "a", "main.hcl")}, first.getAllFiles())
	require.Len(t, index.getAllFiles(), 2, "the shared change set is not modified")

	other := map[string]any{"configuration": map[string]any{"watch_excludes": []any{"**/*.hcl"}}}
	different, err := configurationWatchIndex(config, other, index)
	require.NoError(t, err)
	require.NotSame(t, first, different)
	require.Equal(t, []string{filepath.Join(root, "live", "a", ".terragrunt-cache", "x.tf")}, different.getAllFiles())
}

func BenchmarkFileWatchSelection300Units9000Files(b *testing.B) {
	index, deps := watchFixture(b, 300, 9000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for unit := range deps {
			if _, _, _, err := isComponentDependentFolderOrFileChangedIndexed(index, deps[unit]); err != nil {
				b.Fatal(err)
			}
		}
	}
}

func TestSortAffectedIsDeterministic(t *testing.T) {
	// Terraform precedes helmfile inside a stack because that is the processing order.
	want := []schema.Affected{
		{Stack: "a", ComponentType: "terraform", Component: "app", Affected: "component"},
		{Stack: "a", ComponentType: "terraform", Component: "app", Affected: "stack.vars"},
		{Stack: "a", ComponentType: "terraform", Component: "vpc", Affected: "component"},
		{Stack: "a", ComponentType: "helmfile", Component: "z", Affected: "stack.vars"},
		{Stack: "b", ComponentType: "terraform", Component: "app", Affected: "component"},
		{Stack: "b", ComponentType: "terraform", Component: "zeta", Affected: "component"},
	}
	// Stack results arrive in any order and components within a type in map order.
	arrivals := [][]int{{0, 1, 2, 3, 4, 5}, {4, 5, 3, 2, 1, 0}, {5, 4, 2, 0, 3, 1}, {3, 2, 1, 0, 5, 4}}
	for _, arrival := range arrivals {
		input := make([]schema.Affected, 0, len(want))
		for _, index := range arrival {
			input = append(input, want[index])
		}
		// A scrambled arrival may also reorder types; keep the processing order for the check.
		sortAffected(input)
		require.ElementsMatch(t, want, input)
		for i := 1; i < len(input); i++ {
			require.LessOrEqual(t, input[i-1].Stack, input[i].Stack)
		}
	}
	input := []schema.Affected{want[5], want[4], want[2], want[0], want[1], want[3]}
	sortAffected(input)
	require.Equal(t, want, input)
}

func TestRemoveOnSignalCleansUpBeforeExit(t *testing.T) {
	exited := make(chan int, 1)
	original := exitProcess
	exitProcess = func(code int) { exited <- code }
	t.Cleanup(func() { exitProcess = original })

	var cleaned atomic.Int32
	stop := removeOnSignal(func() { cleaned.Add(1) })
	defer stop()
	process, err := os.FindProcess(os.Getpid())
	require.NoError(t, err)
	if err := process.Signal(syscall.SIGTERM); err != nil {
		t.Skipf("signals cannot be delivered on this platform: %v", err)
	}
	select {
	case code := <-exited:
		require.Equal(t, 128+int(syscall.SIGTERM), code)
		require.Equal(t, int32(1), cleaned.Load(), "the worktree is removed before the process exits")
	case <-time.After(10 * time.Second):
		t.Fatal("signal was not handled")
	}
}

func TestRemoveOnSignalStopReleasesWatcher(t *testing.T) {
	var cleaned atomic.Int32
	removeOnSignal(func() { cleaned.Add(1) })()
	// After a normal return the watcher is gone: it must never run cleanup on its own.
	time.Sleep(20 * time.Millisecond)
	require.Equal(t, int32(0), cleaned.Load())
}
