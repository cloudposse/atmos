package policy

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestPolicyDirectoryDiscovery(t *testing.T) {
	isolateDiscovery(t)
	t.Chdir(t.TempDir())
	directory, err := DirectoryFromEnvironment()
	require.NoError(t, err)
	require.Empty(t, directory)
	require.NoError(t, os.Mkdir("policy.d", 0o700))
	directory, err = DirectoryFromEnvironment()
	require.NoError(t, err)
	want, err := filepath.Abs("policy.d")
	require.NoError(t, err)
	require.Equal(t, want, directory)
	t.Setenv("ATMOS_POLICY_DIR", "explicit")
	directory, err = DirectoryFromEnvironment()
	require.NoError(t, err)
	require.Equal(t, "explicit", directory)
}

func TestPolicyDirectoryDiscoveryRejectsFile(t *testing.T) {
	isolateDiscovery(t)
	t.Chdir(t.TempDir())
	require.NoError(t, os.WriteFile("policy.d", []byte("not a directory"), 0o600))
	_, err := DirectoryFromEnvironment()
	require.Error(t, err)
}

// setArgs replaces os.Args for one test, since default discovery honors --config/--config-path.
func setArgs(t *testing.T, args ...string) {
	t.Helper()
	previous := os.Args
	os.Args = args
	t.Cleanup(func() { os.Args = previous })
}

func isolateDiscovery(t *testing.T) {
	t.Helper()
	for _, name := range []string{"ATMOS_POLICY_DIR", "ATMOS_CLI_CONFIG_PATH", "ATMOS_GIT_ROOT_BASEPATH", "ATMOS_CONFIG", "ATMOS_CONFIG_PATH", "TEST_GIT_ROOT"} {
		t.Setenv(name, "")
	}
	setArgs(t, "atmos")
}

func writeFixtureFile(t *testing.T, parts ...string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(parts...), []byte("x"), 0o600))
}

// TestPolicyDirectoryFollowsProjectRoot verifies default policy.d discovery selects
// the same project root as atmos.yaml discovery.
func TestPolicyDirectoryFollowsProjectRoot(t *testing.T) {
	type fixture struct {
		root, sub, elsewhere string
	}
	tests := []struct {
		name  string
		setup func(t *testing.T, f fixture)
		want  func(f fixture) string // Empty result means no policies.
	}{
		{
			name:  "git root policy.d from subdirectory",
			setup: func(t *testing.T, f fixture) { t.Setenv("TEST_GIT_ROOT", f.root) },
			want:  func(f fixture) string { return filepath.Join(f.root, "policy.d") },
		},
		{
			name: "cwd policy.d wins over git root policy.d",
			setup: func(t *testing.T, f fixture) {
				t.Setenv("TEST_GIT_ROOT", f.root)
				require.NoError(t, os.Mkdir(filepath.Join(f.sub, "policy.d"), 0o755))
			},
			want: func(f fixture) string { return filepath.Join(f.sub, "policy.d") },
		},
		{
			name: "cwd with local config is the project root and has no policy.d",
			setup: func(t *testing.T, f fixture) {
				t.Setenv("TEST_GIT_ROOT", f.root)
				writeFixtureFile(t, f.sub, "atmos.yaml")
			},
			want: func(fixture) string { return "" },
		},
		{
			name: "git root discovery disabled",
			setup: func(t *testing.T, f fixture) {
				t.Setenv("TEST_GIT_ROOT", f.root)
				t.Setenv("ATMOS_GIT_ROOT_BASEPATH", "false")
			},
			want: func(fixture) string { return "" },
		},
		{
			name: "git root without policy.d means no policies",
			setup: func(t *testing.T, f fixture) {
				require.NoError(t, os.Remove(filepath.Join(f.root, "policy.d")))
				t.Setenv("TEST_GIT_ROOT", f.root)
			},
			want: func(fixture) string { return "" },
		},
		{
			name: "ATMOS_CLI_CONFIG_PATH directory replaces git root",
			setup: func(t *testing.T, f fixture) {
				t.Setenv("TEST_GIT_ROOT", f.root)
				t.Setenv("ATMOS_CLI_CONFIG_PATH", f.elsewhere)
			},
			want: func(f fixture) string { return filepath.Join(f.elsewhere, "policy.d") },
		},
		{
			name: "ATMOS_CLI_CONFIG_PATH file resolves to its directory",
			setup: func(t *testing.T, f fixture) {
				t.Setenv("TEST_GIT_ROOT", f.root)
				writeFixtureFile(t, f.elsewhere, "custom.yaml")
				t.Setenv("ATMOS_CLI_CONFIG_PATH", filepath.Join(f.elsewhere, "custom.yaml"))
			},
			want: func(f fixture) string { return filepath.Join(f.elsewhere, "policy.d") },
		},
		{
			name: "ATMOS_CLI_CONFIG_PATH set skips git root",
			setup: func(t *testing.T, f fixture) {
				t.Setenv("TEST_GIT_ROOT", f.root)
				t.Setenv("ATMOS_CLI_CONFIG_PATH", t.TempDir())
			},
			want: func(fixture) string { return "" },
		},
		{
			name: "--config-path flag",
			setup: func(t *testing.T, f fixture) {
				t.Setenv("TEST_GIT_ROOT", f.root)
				setArgs(t, "atmos", "terraform", "plan", "--config-path="+f.elsewhere)
			},
			want: func(f fixture) string { return filepath.Join(f.elsewhere, "policy.d") },
		},
		{
			name: "--config flag resolves to the file's directory",
			setup: func(t *testing.T, f fixture) {
				writeFixtureFile(t, f.elsewhere, "custom.yaml")
				setArgs(t, "atmos", "--config", filepath.Join(f.elsewhere, "custom.yaml"), "terraform", "plan")
			},
			want: func(f fixture) string { return filepath.Join(f.elsewhere, "policy.d") },
		},
		{
			name: "flag beats ATMOS_CLI_CONFIG_PATH and ATMOS_CONFIG_PATH",
			setup: func(t *testing.T, f fixture) {
				t.Setenv("ATMOS_CLI_CONFIG_PATH", f.root)
				t.Setenv("ATMOS_CONFIG_PATH", f.root)
				setArgs(t, "atmos", "--config-path="+f.elsewhere)
			},
			want: func(f fixture) string { return filepath.Join(f.elsewhere, "policy.d") },
		},
		{
			name:  "ATMOS_CONFIG_PATH env",
			setup: func(t *testing.T, f fixture) { t.Setenv("ATMOS_CONFIG_PATH", f.elsewhere) },
			want:  func(f fixture) string { return filepath.Join(f.elsewhere, "policy.d") },
		},
		{
			name: "config selection without policy.d still honors cwd policy.d",
			setup: func(t *testing.T, f fixture) {
				require.NoError(t, os.Remove(filepath.Join(f.elsewhere, "policy.d")))
				require.NoError(t, os.Mkdir(filepath.Join(f.sub, "policy.d"), 0o755))
				setArgs(t, "atmos", "--config-path="+f.elsewhere)
			},
			want: func(f fixture) string { return filepath.Join(f.sub, "policy.d") },
		},
		{
			name: "ancestor atmos.yaml outside a repository",
			setup: func(t *testing.T, f fixture) {
				t.Setenv("ATMOS_GIT_ROOT_BASEPATH", "false")
				writeFixtureFile(t, f.root, "atmos.yaml")
			},
			want: func(f fixture) string { return filepath.Join(f.root, "policy.d") },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isolateDiscovery(t)
			f := fixture{root: t.TempDir(), elsewhere: t.TempDir()}
			f.sub = filepath.Join(f.root, "components", "terraform", "mock")
			require.NoError(t, os.MkdirAll(f.sub, 0o755))
			require.NoError(t, os.Mkdir(filepath.Join(f.root, "policy.d"), 0o755))
			require.NoError(t, os.Mkdir(filepath.Join(f.elsewhere, "policy.d"), 0o755))
			tt.setup(t, f)
			t.Chdir(f.sub)

			directory, err := DirectoryFromEnvironment()
			require.NoError(t, err)
			want := tt.want(f)
			if want == "" {
				assert.Empty(t, directory)
				return
			}
			// t.Chdir may resolve symlinks (macOS /var), so compare resolved paths.
			wantResolved, err := filepath.EvalSymlinks(want)
			require.NoError(t, err)
			gotResolved, err := filepath.EvalSymlinks(directory)
			require.NoError(t, err)
			assert.Equal(t, wantResolved, gotResolved)
		})
	}
}

func TestPolicyDirectoryExplicitOverrideBeatsDiscovery(t *testing.T) {
	isolateDiscovery(t)
	root := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(root, "policy.d"), 0o755))
	t.Setenv("TEST_GIT_ROOT", root)
	sub := filepath.Join(root, "sub")
	require.NoError(t, os.Mkdir(sub, 0o755))
	t.Chdir(sub)
	t.Setenv("ATMOS_POLICY_DIR", "explicit")
	directory, err := DirectoryFromEnvironment()
	require.NoError(t, err)
	assert.Equal(t, "explicit", directory)
}

func TestPolicyDirectoryGitRootPolicyFileIsRejected(t *testing.T) {
	isolateDiscovery(t)
	root := t.TempDir()
	writeFixtureFile(t, root, "policy.d")
	t.Setenv("TEST_GIT_ROOT", root)
	sub := filepath.Join(root, "sub")
	require.NoError(t, os.Mkdir(sub, 0o755))
	t.Chdir(sub)
	_, err := DirectoryFromEnvironment()
	require.ErrorIs(t, err, errUtils.ErrPolicyInvalid)
}
