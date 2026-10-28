package releaseplan

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/process"
)

//go:generate go run go.uber.org/mock/mockgen -destination mock_repository_test.go -package releaseplan . Repository
//go:generate go run go.uber.org/mock/mockgen -destination mock_process_test.go -package releaseplan github.com/cloudposse/atmos/pkg/process Runner

const inventoryYAML = `runners:
  linux/amd64: linux-runner
  linux/arm64/v8.2: arm-runner
packages:
  example:
    path: packages/example
    repository: oci://registry.example.com/team/example
    inputs: [sdk/**, go.mod]
`

func manifestYAML(version string) string {
	return `apiVersion: atmos/v1
kind: AtmosPluginConfig
metadata:
  name: team/example
  version: ` + version + `
spec:
  capabilities: {}
  runtime:
    type: native
    protocol: 1
    executable: bin/plugin
  compatibility:
    protocol: '>=1 <2'
  platforms:
    linux/amd64:
      executable: bin/linux-amd64/plugin
    linux/arm64/v8.2:
      executable: bin/linux-arm64-v8.2/plugin
`
}

func repositoryFor(t *testing.T, overrides map[string]string, failure string) *MockRepository {
	t.Helper()
	values := map[string]string{
		"rev-parse --verify --end-of-options HEAD^{commit}":                   "head-sha\n",
		"rev-parse --verify --end-of-options base^{commit}":                   "base-sha\n",
		"show base-sha:plugins/releases.yaml":                                 inventoryYAML,
		"show head-sha:plugins/releases.yaml":                                 inventoryYAML,
		"show head-sha:packages/example/plugin.yaml":                          manifestYAML("1.1.0"),
		"show base-sha:packages/example/plugin.yaml":                          manifestYAML("1.0.0"),
		"diff --name-only --no-relative --no-renames -z base-sha head-sha --": "packages/example/main.go\x00",
		"ls-tree --full-tree -r --name-only -z base-sha":                      "packages/example/plugin.yaml\x00plugins/releases.yaml\x00",
		"log -1 --first-parent --format=%H head-sha -- :(top,literal)packages/example :(top,literal)plugins/releases.yaml :(top,glob)sdk/** :(top,glob)go.mod": "input-sha\n",
	}
	for key, value := range overrides {
		values[key] = value
	}
	repository := NewMockRepository(gomock.NewController(t))
	repository.EXPECT().Run(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, args ...string) ([]byte, error) {
		key := strings.Join(args, " ")
		if key == failure {
			return nil, errUtils.ErrProcessWaitFailed
		}
		value, ok := values[key]
		require.True(t, ok, "unexpected repository request: %s", key)
		return []byte(value), nil
	}).AnyTimes()
	return repository
}

func TestCreate(t *testing.T) {
	repository := repositoryFor(t, nil, "")
	plan, err := Create(t.Context(), WithRepository(repository), WithHead("HEAD"), WithBase("base"), WithInventory("plugins/releases.yaml"))
	require.NoError(t, err)
	expected := Entry{Name: "example", Path: "packages/example", Version: "1.1.0", Source: "input-sha", Repository: "oci://registry.example.com/team/example"}
	require.Equal(t, []Entry{expected}, plan.Packages.Include)
	require.Equal(t, []BuildEntry{
		{Entry: expected, Platform: "linux/amd64", Runner: "linux-runner", Artifact: "linux-amd64", Executable: "bin/linux-amd64/plugin"},
		{Entry: expected, Platform: "linux/arm64/v8.2", Runner: "arm-runner", Artifact: "linux-arm64-v8.2", Executable: "bin/linux-arm64-v8.2/plugin"},
	}, plan.Build.Include)
	raw, err := json.Marshal(plan)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"packages":{"include":[{"name":"example"`)
	require.Contains(t, string(raw), `"platform":"linux/arm64/v8.2"`)
}

func TestCreateCrossBuildPlanWithoutRunnerMappings(t *testing.T) {
	_, packages, found := strings.Cut(inventoryYAML, "packages:")
	require.True(t, found, "inventory fixture must include packages")
	withoutRunners := "packages:" + packages
	repository := repositoryFor(t, map[string]string{"show head-sha:plugins/releases.yaml": withoutRunners}, "")
	plan, err := Create(t.Context(), WithRepository(repository))
	require.NoError(t, err)
	require.Len(t, plan.Packages.Include, 1)
	require.Len(t, plan.Build.Include, 2)
	for _, entry := range plan.Build.Include {
		require.Empty(t, entry.Runner)
	}
	raw, err := json.Marshal(plan)
	require.NoError(t, err)
	require.NotContains(t, string(raw), `"runner"`)
}

func TestCreateFirstReleaseAndUnchangedPackage(t *testing.T) {
	t.Run("first release needs no base", func(t *testing.T) {
		plan, err := Create(t.Context(), WithRepository(repositoryFor(t, nil, "")))
		require.NoError(t, err)
		require.Equal(t, "input-sha", plan.Packages.Include[0].Source)
	})
	t.Run("new package absent from base", func(t *testing.T) {
		_, err := Create(t.Context(), WithBase("base"), WithRepository(repositoryFor(t, map[string]string{"ls-tree --full-tree -r --name-only -z base-sha": "unrelated.yaml\x00"}, "")))
		require.NoError(t, err)
	})
	t.Run("unrelated changes preserve version and source", func(t *testing.T) {
		plan, err := Create(t.Context(), WithBase("base"), WithRepository(repositoryFor(t, map[string]string{
			"show head-sha:packages/example/plugin.yaml":                          manifestYAML("1.0.0"),
			"diff --name-only --no-relative --no-renames -z base-sha head-sha --": "docs/readme.md\x00",
		}, "")))
		require.NoError(t, err)
		require.Equal(t, "1.0.0", plan.Packages.Include[0].Version)
		require.Equal(t, "input-sha", plan.Packages.Include[0].Source)
	})
}

func TestCreateRequiresIndependentVersionIncrease(t *testing.T) {
	for _, changed := range []string{"packages/example/main.go", "sdk/nested/api.go", "go.mod"} {
		t.Run(changed, func(t *testing.T) {
			_, err := Create(t.Context(), WithBase("base"), WithRepository(repositoryFor(t, map[string]string{
				"show head-sha:packages/example/plugin.yaml":                          manifestYAML("1.0.0"),
				"diff --name-only --no-relative --no-renames -z base-sha head-sha --": changed + "\x00",
			}, "")))
			require.ErrorIs(t, err, errUtils.ErrPluginReleasePlan)
			require.ErrorContains(t, err, "version to increase")
		})
	}
}

func TestCreateInvalidData(t *testing.T) {
	tests := []struct{ name, key, value, message string }{
		{"yaml", "plugins/releases.yaml", "[", "inventory"},
		{"extra document", "plugins/releases.yaml", inventoryYAML + "---\npackages: {}", "one YAML document"},
		{"unknown field", "plugins/releases.yaml", inventoryYAML + "unknown: true\n", "inventory"},
		{"empty inventory", "plugins/releases.yaml", "packages: {}", "no packages"},
		{"invalid name", "plugins/releases.yaml", strings.ReplaceAll(inventoryYAML, "  example:", "  EXAMPLE:"), "planning failed"},
		{"escaping path", "plugins/releases.yaml", strings.ReplaceAll(inventoryYAML, "packages/example", "../example"), "planning failed"},
		{"glob path", "plugins/releases.yaml", strings.ReplaceAll(inventoryYAML, "packages/example", "packages/*"), "planning failed"},
		{"missing scheme", "plugins/releases.yaml", strings.ReplaceAll(inventoryYAML, "oci://", ""), "oci://"},
		{"tagged repository", "plugins/releases.yaml", strings.ReplaceAll(inventoryYAML, "team/example", "team/example:latest"), "tag or digest"},
		{"unsafe input", "plugins/releases.yaml", strings.ReplaceAll(inventoryYAML, "sdk/**", "../sdk/**"), "invalid build input"},
		{"malformed pattern", "plugins/releases.yaml", strings.ReplaceAll(inventoryYAML, "sdk/**", "'sdk/['"), "invalid build input"},
		{"missing runner", "plugins/releases.yaml", strings.ReplaceAll(inventoryYAML, "linux/amd64: linux-runner", "darwin/amd64: mac-runner"), "requires a runner"},
		{"invalid manifest", "packages/example/plugin.yaml", "spec: {}", "planning failed"},
		{"prerelease", "packages/example/plugin.yaml", manifestYAML("1.1.0-rc.1"), "stable major.minor.patch"},
		{"metadata version", "packages/example/plugin.yaml", manifestYAML("1.1.0+build"), "stable major.minor.patch"},
		{"identity mismatch", "packages/example/plugin.yaml", strings.ReplaceAll(manifestYAML("1.1.0"), "team/example", "team/other"), "manifest name"},
		{"platform invalid", "packages/example/plugin.yaml", strings.ReplaceAll(manifestYAML("1.1.0"), "linux/amd64", "linux/amd64/v99"), "planning failed"},
		{"executable escape", "packages/example/plugin.yaml", strings.ReplaceAll(manifestYAML("1.1.0"), "bin/linux-amd64/plugin", "../plugin"), "safe executable"},
		{"no platforms", "packages/example/plugin.yaml", strings.Split(manifestYAML("1.1.0"), "  platforms:")[0], "no release platforms"},
		{"version rollback", "packages/example/plugin.yaml", manifestYAML("0.9.0"), "cannot decrease"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Create(t.Context(), WithBase("base"), WithRepository(repositoryFor(t, map[string]string{"show head-sha:" + tc.key: tc.value}, "")))
			require.ErrorIs(t, err, errUtils.ErrPluginReleasePlan)
			require.ErrorContains(t, err, tc.message)
		})
	}
}

func TestCreateRepositoryFailures(t *testing.T) {
	for _, failure := range []string{
		"rev-parse --verify --end-of-options HEAD^{commit}", "show head-sha:plugins/releases.yaml",
		"diff --name-only --no-relative --no-renames -z base-sha head-sha --", "ls-tree --full-tree -r --name-only -z base-sha",
		"show head-sha:packages/example/plugin.yaml", "show base-sha:packages/example/plugin.yaml", "show base-sha:plugins/releases.yaml",
		"log -1 --first-parent --format=%H head-sha -- :(top,literal)packages/example :(top,literal)plugins/releases.yaml :(top,glob)sdk/** :(top,glob)go.mod",
	} {
		t.Run(failure, func(t *testing.T) {
			_, err := Create(t.Context(), WithBase("base"), WithRepository(repositoryFor(t, nil, failure)))
			require.ErrorIs(t, err, errUtils.ErrPluginReleasePlan)
			require.ErrorIs(t, err, errUtils.ErrProcessWaitFailed)
		})
	}
	t.Run("invalid base manifest", func(t *testing.T) {
		_, err := Create(t.Context(), WithBase("base"), WithRepository(repositoryFor(t, map[string]string{"show base-sha:packages/example/plugin.yaml": "invalid"}, "")))
		require.ErrorIs(t, err, errUtils.ErrPluginReleasePlan)
	})
	t.Run("empty history", func(t *testing.T) {
		_, err := Create(t.Context(), WithRepository(repositoryFor(t, map[string]string{"log -1 --first-parent --format=%H head-sha -- :(top,literal)packages/example :(top,literal)plugins/releases.yaml :(top,glob)sdk/** :(top,glob)go.mod": ""}, "")))
		require.ErrorContains(t, err, "source commit missing")
	})
}

func TestCreateInvalidOptions(t *testing.T) {
	for _, option := range []Option{WithRepository(nil), WithInventory("../outside.yaml"), WithHead("")} {
		_, err := Create(t.Context(), option)
		require.ErrorIs(t, err, errUtils.ErrPluginReleasePlan)
	}
}

func TestSafePath(t *testing.T) {
	for _, value := range []string{"", ".", "..", "../a", "a/../b", "/abs", "a\\b", "C:/a", "a\n", "a\x00"} {
		require.False(t, safePath(value), value)
	}
	for _, value := range []string{"plugins/example", "sdk/**", "go.mod", "a space/file"} {
		require.True(t, safePath(value), value)
	}
}

func TestGit(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprint(fail), func(t *testing.T) {
			runner := NewMockRunner(gomock.NewController(t))
			runner.EXPECT().Run(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, spec process.TaskSpec) process.Result {
				require.Equal(t, "git", spec.Command)
				require.Equal(t, []string{"show", "sha:package/plugin.yaml"}, spec.Args)
				require.True(t, spec.SkipMetrics)
				if fail {
					_, _ = spec.Streams.Stderr.Write([]byte("missing object"))
					return process.Result{ExitCode: 1, Err: errUtils.ErrProcessWaitFailed}
				}
				_, _ = spec.Streams.Stdout.Write([]byte("committed bytes\n"))
				return process.Result{}
			})
			result, err := (Git{Runner: runner}).Run(t.Context(), "show", "sha:package/plugin.yaml")
			if fail {
				require.ErrorIs(t, err, errUtils.ErrPluginReleasePlan)
				require.ErrorContains(t, err, "missing object")
				return
			}
			require.NoError(t, err)
			require.Equal(t, "committed bytes\n", string(result))
		})
	}
}

func TestValidPattern(t *testing.T) {
	for _, input := range []string{"sdk/{one,two}", "sdk/part**", "sdk/**/part**"} {
		require.False(t, validPattern(input))
	}
	require.True(t, validPattern("sdk/**/part*"))
}

func TestInventoryChangesArePackageScoped(t *testing.T) {
	for _, tc := range []struct {
		name, old string
		changed   bool
	}{
		{"unchanged", inventoryYAML, false},
		{"repository changed", strings.ReplaceAll(inventoryYAML, "team/example", "old/example"), true},
		{"input removed", strings.ReplaceAll(inventoryYAML, "sdk/**, go.mod", "sdk/**, go.mod, old/**"), true},
		{"used runner changed", strings.ReplaceAll(inventoryYAML, "linux-runner", "old-runner"), true},
		{"unrelated runner changed", strings.ReplaceAll(inventoryYAML, "runners:", "runners:\n  darwin/amd64: mac-runner"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Create(t.Context(), WithBase("base"), WithRepository(repositoryFor(t, map[string]string{
				"show head-sha:packages/example/plugin.yaml":                          manifestYAML("1.0.0"),
				"show base-sha:plugins/releases.yaml":                                 tc.old,
				"diff --name-only --no-relative --no-renames -z base-sha head-sha --": "plugins/releases.yaml\x00",
			}, "")))
			if tc.changed {
				require.ErrorContains(t, err, "version to increase")
			} else {
				require.NoError(t, err)
			}
		})
	}
	t.Run("adding another package preserves existing version", func(t *testing.T) {
		plan, err := Create(t.Context(), WithBase("base"), WithRepository(repositoryFor(t, map[string]string{
			"show head-sha:plugins/releases.yaml":        inventoryYAML + "  another:\n    path: packages/another\n    repository: oci://registry.example.com/team/another\n",
			"show head-sha:packages/example/plugin.yaml": manifestYAML("1.0.0"),
			"show head-sha:packages/another/plugin.yaml": strings.ReplaceAll(manifestYAML("1.0.0"), "team/example", "team/another"),
			"log -1 --first-parent --format=%H head-sha -- :(top,literal)packages/another :(top,literal)plugins/releases.yaml": "new-source",
			"diff --name-only --no-relative --no-renames -z base-sha head-sha --":                                              "plugins/releases.yaml\x00packages/another/plugin.yaml\x00",
		}, "")))
		require.NoError(t, err)
		require.Len(t, plan.Packages.Include, 2)
		require.Equal(t, "another", plan.Packages.Include[0].Name)
		require.Equal(t, "example", plan.Packages.Include[1].Name)
		require.Equal(t, "1.0.0", plan.Packages.Include[1].Version)
	})
	t.Run("moving a package still compares its old version", func(t *testing.T) {
		_, err := Create(t.Context(), WithBase("base"), WithRepository(repositoryFor(t, map[string]string{
			"show head-sha:packages/example/plugin.yaml":     manifestYAML("1.0.0"),
			"show base-sha:plugins/releases.yaml":            strings.ReplaceAll(inventoryYAML, "packages/example", "old/example"),
			"ls-tree --full-tree -r --name-only -z base-sha": "plugins/releases.yaml\x00old/example/plugin.yaml\x00",
			"show base-sha:old/example/plugin.yaml":          manifestYAML("1.0.0"),
		}, "")))
		require.ErrorContains(t, err, "version to increase")
	})
}
