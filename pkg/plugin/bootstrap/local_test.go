package bootstrap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	adapter "github.com/cloudposse/atmos/pkg/plugin/adapter/auth"
	"github.com/cloudposse/atmos/pkg/plugin/config"
	"github.com/cloudposse/atmos/pkg/plugin/install"
	pluginlock "github.com/cloudposse/atmos/pkg/plugin/lock"
	"github.com/cloudposse/atmos/pkg/schema"
)

const localAuthKind = "test/bootstrap-provider"

// localAuthPackage writes a build directory whose manifest exports localAuthKind, the way a developer's
// `atmos plugin build` output looks.
func localAuthPackage(t *testing.T, executable string) string {
	t.Helper()
	directory := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(directory, filepath.Dir(executable)), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(directory, executable), []byte("must never execute while registering"), 0o700))
	require.NoError(t, os.Mkdir(filepath.Join(directory, "schemas"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "schemas", "provider.json"), []byte(`{"type":"object"}`), 0o600))
	writeLocalAuthManifest(t, directory, executable)
	return directory
}

func writeLocalAuthManifest(t *testing.T, directory, executable string) {
	t.Helper()
	source := strings.ReplaceAll(automaticManifest, "example/automatic", "example/local-auth")
	source = strings.ReplaceAll(source, "executable: bin/plugin", "executable: "+executable)
	source = strings.ReplaceAll(source, "PLATFORM", install.CurrentPlatform())
	source = strings.Replace(source, "    commands:\n      about: {description: Static automatic-install fixture}", "    auth:\n      providers:\n        "+localAuthKind+": {schema: schemas/provider.json, credentials: [oidc/v1]}", 1)
	require.NoError(t, os.WriteFile(filepath.Join(directory, "plugin.yaml"), []byte(source), 0o600))
}

func TestPathWinsOverTheLockedReleaseAndNeverTouchesTheLock(t *testing.T) {
	atmos, requests := automaticFixture(t)
	cachedBootstrapAuth(t, atmos)
	requests.Store(0)
	before, err := os.ReadFile(pluginlock.Path(ProjectDirectory(atmos)))
	require.NoError(t, err)

	// Without the profile the package is the locked release, exactly as before.
	state, err := selectedForUse(t.Context(), atmos)
	require.NoError(t, err)
	released := state.Packages["bootstrap-auth"]
	require.False(t, released.Local)
	require.Equal(t, "bin/plugin", released.Executable)

	// A profile adds path beside the release's source and version: path wins.
	local := localAuthPackage(t, filepath.Join("out", "plugin"))
	declareLocal(atmos, "bootstrap-auth", local)
	state, err = selectedForUse(t.Context(), atmos)
	require.NoError(t, err)
	selected := state.Packages["bootstrap-auth"]
	require.True(t, selected.Local)
	require.Equal(t, local, selected.Path)
	require.NotEqual(t, released.Path, selected.Path)
	require.Zero(t, requests.Load(), "a path package never contacts a registry")
	after, err := os.ReadFile(pluginlock.Path(ProjectDirectory(atmos)))
	require.NoError(t, err)
	require.Equal(t, before, after, "the release lock is not rewritten while the path is active")

	// Removing the path again returns to the release.
	declareLocal(atmos, "bootstrap-auth", "")
	state, err = selectedForUse(t.Context(), atmos)
	require.NoError(t, err)
	require.False(t, state.Packages["bootstrap-auth"].Local)
}

func TestPathPackageIsActivatedOnlyWhenACommandNeedsIt(t *testing.T) {
	atmos, requests := automaticFixture(t)
	local := localAuthPackage(t, "bin/plugin")
	declareLocal(atmos, "fixture", local)
	output := captureNotices(t)

	// No auth kind references the plugin: nothing is opened, registered or announced.
	state, err := selectedForUse(t.Context(), atmos)
	require.NoError(t, err)
	require.Empty(t, state.Packages)
	require.NoError(t, os.Remove(filepath.Join(local, "plugin.yaml")))
	cleanup, err := Prepare(t.Context(), atmos)
	require.NoError(t, err, "an unbuilt path package must not break commands that do not use it")
	cleanup()
	assert.NotContains(t, output.String(), "local path")

	// A referenced kind exported by the path package activates it, even though the manifest is
	// restored, and the release in the registry is never fetched.
	writeLocalAuthManifest(t, local, "bin/plugin")
	atmos.Auth.Providers = map[string]schema.Provider{"corporate": {Kind: localAuthKind}}
	state, err = selectedForUse(t.Context(), atmos)
	require.NoError(t, err)
	require.True(t, state.Packages["fixture"].Local)
	require.Zero(t, requests.Load())
}

func TestPathPackageWarnsOncePerAliasPerInvocation(t *testing.T) {
	atmos, _ := automaticFixture(t)
	t.Setenv("ATMOS_KEYRING_TYPE", "memory")
	t.Setenv("ATMOS_XDG_CACHE_HOME", t.TempDir())
	local := localAuthPackage(t, "bin/plugin")
	declareLocal(atmos, "fixture", local)
	atmos.Auth.Providers = map[string]schema.Provider{"corporate": {Kind: localAuthKind}}
	atmos.Auth.Identities = map[string]schema.Identity{"reader": {Kind: "aws/assume-role", Via: &schema.IdentityVia{Provider: "corporate"}}}
	atmos.Plugins.Registries = map[string]schema.PluginRegistry{"corporate": {Catalog: "oci://registry.example.com/catalog:stable", Identity: "reader"}}
	output := captureNotices(t)

	// Prepare registers the package twice: once for registry authentication and once for the command.
	cleanup, err := Prepare(t.Context(), atmos)
	require.NoError(t, err)
	t.Cleanup(cleanup)

	plain := ansi.Strip(output.String())
	assert.Equal(t, 1, strings.Count(plain, "Plugin fixture runs from local path"), plain)
	assert.Contains(t, plain, "lock, signature and version checks are bypassed")
	assert.Contains(t, plain, local)

	// A copied configuration within the same process does not warn again.
	next := *atmos
	selection := &config.Selections{Packages: map[string]config.Selection{"fixture": {Path: local, Local: true}}}
	announceLocal(&next, selection)
	assert.Equal(t, 1, strings.Count(ansi.Strip(output.String()), "runs from local path"))

	// A later invocation is a new process, which starts with nothing announced.
	resetAnnouncedLocal()
	announceLocal(&next, &config.Selections{Packages: map[string]config.Selection{"fixture": {Path: local, Local: true}, "released": {Path: "elsewhere"}}})
	plain = ansi.Strip(output.String())
	assert.Equal(t, 2, strings.Count(plain, "runs from local path"))
	assert.NotContains(t, plain, "released", "a released package is never announced")
}

func TestPathPackageExecutableIsReadFromTheManifestOnEveryActivation(t *testing.T) {
	local := localAuthPackage(t, filepath.Join("v1", "plugin"))
	entry := config.Selection{Path: local, Local: true}
	activate := func() {
		constructors := newConstructors(&adapter.Services{})
		registry, err := descriptors(constructors)
		require.NoError(t, err)
		require.NoError(t, activatePackage(t.Context(), &entry, registry))
		require.Contains(t, constructors.Batch.Providers, localAuthKind)
	}
	activate()

	// A rebuild that moves the executable is picked up by the next command without reinstalling.
	require.NoError(t, os.MkdirAll(filepath.Join(local, "v2"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(local, "v2", "plugin"), []byte("rebuilt"), 0o700))
	writeLocalAuthManifest(t, local, filepath.ToSlash(filepath.Join("v2", "plugin")))
	require.NoError(t, os.RemoveAll(filepath.Join(local, "v1")))
	activate()

	// Negative: a missing executable names the alias and path, and keeps the sentinel.
	require.NoError(t, os.RemoveAll(filepath.Join(local, "v2")))
	registry, err := Descriptors()
	require.NoError(t, err)
	err = activatePackage(t.Context(), &entry, registry)
	require.ErrorIs(t, err, errUtils.ErrPluginNotInstalled)
	assert.Contains(t, strings.Join(errUtils.AllHints(err), "\n"), "atmos plugin build")
}

func TestPathPackageDisabledIsIgnored(t *testing.T) {
	atmos, _ := automaticFixture(t)
	disabled := false
	declaration := atmos.Plugins.Packages["fixture"]
	declaration.Path, declaration.Enabled = filepath.Join(t.TempDir(), "missing"), &disabled
	atmos.Plugins.Packages["fixture"] = declaration
	state, err := selectedPackages(t.Context(), atmos)
	require.NoError(t, err)
	require.Empty(t, state.Packages)
}

func TestSelectedPackagesLimitsLocalSelectionToTheRequestedAlias(t *testing.T) {
	atmos, _ := automaticFixture(t)
	atmos.Plugins.Packages["other"] = schema.PluginPackage{Path: localPackageFixture(t)}
	declareLocal(atmos, "fixture", localPackageFixture(t))
	state, err := selectedPackages(t.Context(), atmos, "other")
	require.NoError(t, err)
	require.Len(t, state.Packages, 1)
	require.True(t, state.Packages["other"].Local)
}
