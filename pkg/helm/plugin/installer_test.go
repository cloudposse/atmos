package plugin

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// recordedCall captures a single subprocess invocation.
type recordedCall struct {
	name string
	args []string
	env  []string
}

// fakeRunner is a scriptable Runner for tests.
type fakeRunner struct {
	calls []recordedCall
	// listOutput is returned for `helm plugin list`.
	listOutput string
	// installErr, if set, is returned for `helm plugin install`.
	installErr error
}

func (f *fakeRunner) Run(_ context.Context, name string, args, extraEnv []string) (string, string, error) {
	f.calls = append(f.calls, recordedCall{name: name, args: append([]string(nil), args...), env: append([]string(nil), extraEnv...)})

	if len(args) >= 2 && args[0] == "plugin" {
		switch args[1] {
		case "list":
			return f.listOutput, "", nil
		case "install":
			if f.installErr != nil {
				return "", "boom", f.installErr
			}
			return "", "", nil
		case "uninstall":
			return "", "", nil
		}
	}
	return "", "", nil
}

func (f *fakeRunner) installCalls() []recordedCall {
	var out []recordedCall
	for _, c := range f.calls {
		if len(c.args) >= 2 && c.args[0] == "plugin" && c.args[1] == "install" {
			out = append(out, c)
		}
	}
	return out
}

func (f *fakeRunner) uninstallCalls() []recordedCall {
	var out []recordedCall
	for _, c := range f.calls {
		if len(c.args) >= 2 && c.args[0] == "plugin" && c.args[1] == "uninstall" {
			out = append(out, c)
		}
	}
	return out
}

func newTestInstaller(r Runner, dir string) *Installer {
	return NewInstaller("/fake/helm", WithRunner(r), WithDir(dir))
}

func TestEnsurePlugins_InstallsMissing(t *testing.T) {
	dir := t.TempDir()
	runner := &fakeRunner{listOutput: "NAME\tVERSION\tDESCRIPTION\n"}
	inst := newTestInstaller(runner, dir)

	got, err := inst.EnsurePlugins(context.Background(), []Spec{
		{Name: "diff", URL: "https://github.com/databus23/helm-diff", Version: "v3.9.4"},
	})
	require.NoError(t, err)
	assert.Equal(t, dir, got)

	installs := runner.installCalls()
	require.Len(t, installs, 1)
	assert.Equal(t, []string{"plugin", "install", "https://github.com/databus23/helm-diff", "--version", "v3.9.4"}, installs[0].args)
	assert.Contains(t, installs[0].env, "HELM_PLUGINS="+dir)
	assert.Empty(t, runner.uninstallCalls())
}

func TestEnsurePlugins_InstallsLatestWithoutVersionFlag(t *testing.T) {
	runner := &fakeRunner{listOutput: "NAME\tVERSION\n"}
	inst := newTestInstaller(runner, t.TempDir())

	_, err := inst.EnsurePlugins(context.Background(), []Spec{
		{Name: "secrets", URL: "https://github.com/jkroepke/helm-secrets"},
	})
	require.NoError(t, err)

	installs := runner.installCalls()
	require.Len(t, installs, 1)
	assert.Equal(t, []string{"plugin", "install", "https://github.com/jkroepke/helm-secrets"}, installs[0].args)
}

func TestEnsurePlugins_SkipsWhenAlreadyInstalled(t *testing.T) {
	runner := &fakeRunner{listOutput: "NAME\tVERSION\tDESCRIPTION\ndiff\t3.9.4\tdiff plugin\n"}
	inst := newTestInstaller(runner, t.TempDir())

	_, err := inst.EnsurePlugins(context.Background(), []Spec{
		{Name: "diff", URL: "https://github.com/databus23/helm-diff", Version: "v3.9.4"},
	})
	require.NoError(t, err)
	assert.Empty(t, runner.installCalls(), "should not reinstall a satisfied plugin")
	assert.Empty(t, runner.uninstallCalls())
}

func TestEnsurePlugins_ReinstallsOnVersionMismatch(t *testing.T) {
	runner := &fakeRunner{listOutput: "NAME\tVERSION\ndiff\t3.8.0\n"}
	inst := newTestInstaller(runner, t.TempDir())

	_, err := inst.EnsurePlugins(context.Background(), []Spec{
		{Name: "diff", URL: "https://github.com/databus23/helm-diff", Version: "v3.9.4"},
	})
	require.NoError(t, err)

	uninstalls := runner.uninstallCalls()
	require.Len(t, uninstalls, 1)
	assert.Equal(t, []string{"plugin", "uninstall", "diff"}, uninstalls[0].args)
	require.Len(t, runner.installCalls(), 1)
}

func TestEnsurePlugins_SkipsLatestWhenPresent(t *testing.T) {
	runner := &fakeRunner{listOutput: "NAME\tVERSION\ndiff\t3.8.0\n"}
	inst := newTestInstaller(runner, t.TempDir())

	_, err := inst.EnsurePlugins(context.Background(), []Spec{
		{Name: "diff", URL: "https://github.com/databus23/helm-diff"}, // latest
	})
	require.NoError(t, err)
	assert.Empty(t, runner.installCalls(), "latest should not reinstall when already present")
}

func TestEnsurePlugins_EmptyReturnsDirNoCalls(t *testing.T) {
	dir := t.TempDir()
	runner := &fakeRunner{}
	inst := newTestInstaller(runner, dir)

	got, err := inst.EnsurePlugins(context.Background(), nil)
	require.NoError(t, err)
	assert.Equal(t, dir, got)
	assert.Empty(t, runner.calls)
}

func TestEnsurePlugins_MissingHelmBinary(t *testing.T) {
	runner := &fakeRunner{}
	inst := NewInstaller("", WithRunner(runner), WithDir(t.TempDir()))

	_, err := inst.EnsurePlugins(context.Background(), []Spec{{Name: "diff", URL: "u"}})
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrHelmBinaryNotFound))
}

func TestEnsurePlugins_InstallFailureWrapped(t *testing.T) {
	runner := &fakeRunner{listOutput: "NAME\tVERSION\n", installErr: errors.New("network down")}
	inst := newTestInstaller(runner, t.TempDir())

	_, err := inst.EnsurePlugins(context.Background(), []Spec{{Name: "diff", URL: "u", Version: "v1"}})
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrHelmPluginInstall))
}

func TestParsePluginList(t *testing.T) {
	out := "NAME\tVERSION\tDESCRIPTION\ndiff\t3.9.4\tShows diffs\nsecrets\t4.6.0\tManage secrets\n"
	got := parsePluginList(out)
	assert.Equal(t, map[string]string{"diff": "3.9.4", "secrets": "4.6.0"}, got)

	assert.Empty(t, parsePluginList(""))
	assert.Empty(t, parsePluginList("NAME\tVERSION\n"))
}

func TestVersionsEqual(t *testing.T) {
	assert.True(t, versionsEqual("v3.9.4", "3.9.4"))
	assert.True(t, versionsEqual("3.9.4", "3.9.4"))
	assert.False(t, versionsEqual("3.9.4", "3.9.5"))
}

func TestMatchInstalled(t *testing.T) {
	installed := map[string]string{"helm-git": "0.16.0"}
	name, ver, ok := matchInstalled(Spec{Name: "git"}, installed)
	require.True(t, ok)
	assert.Equal(t, "helm-git", name)
	assert.Equal(t, "0.16.0", ver)

	_, _, ok = matchInstalled(Spec{Name: "diff"}, installed)
	assert.False(t, ok)
}

func TestExtractSpecs(t *testing.T) {
	assert.Nil(t, ExtractSpecs(map[string]any{}))
	assert.Equal(t, []string{"diff@v3.9.4"}, ExtractSpecs(map[string]any{"plugins": []string{"diff@v3.9.4"}}))
	assert.Equal(t, []string{"diff", "secrets"}, ExtractSpecs(map[string]any{"plugins": []any{"diff", "secrets"}}))
	// Non-list value is ignored.
	assert.Nil(t, ExtractSpecs(map[string]any{"plugins": "diff"}))
}

func TestManagedDirSuffix(t *testing.T) {
	assert.True(t, strings.HasSuffix(ManagedDir(), managedDirName))
}
