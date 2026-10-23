package policy_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pluginpolicy "github.com/cloudposse/atmos/pkg/plugin/policy"
	"github.com/cloudposse/atmos/pkg/policy/guard"
)

// recordInputs replaces the evaluator with one that records every plugin input.
func recordInputs(t *testing.T) *[]guard.PluginInput {
	t.Helper()
	t.Cleanup(guard.SaveHooks())
	t.Cleanup(func() { guard.SetActiveProfiles(nil) })
	var inputs []guard.PluginInput
	guard.SetEnabled(func(string) (bool, error) { return true, nil })
	guard.SetEvaluator(func(_ context.Context, kind string, raw json.RawMessage) error {
		require.Equal(t, "plugin", kind)
		var input guard.PluginInput
		require.NoError(t, json.Unmarshal(raw, &input))
		inputs = append(inputs, input)
		return nil
	})
	return &inputs
}

func TestDevelopmentPathIsRelativeToTheProjectWithoutDotDot(t *testing.T) {
	project := t.TempDir()
	inside := filepath.Join(project, "build", "oidc")
	require.NoError(t, os.MkdirAll(inside, 0o700))
	outside := t.TempDir()

	cases := []struct {
		name, project, directory, want string
	}{
		{"inside the project", project, inside, "build/oidc"},
		{"project root", project, project, "."},
		{"unclean spelling", project, filepath.Join(project, "build", "..", "build", "oidc"), "build/oidc"},
		{"relative directory", project, filepath.Join(project, "build", "oidc", "..", "oidc"), "build/oidc"},
		{"outside is absolute", project, outside, filepath.ToSlash(realPath(t, outside))},
		{"missing directory stays below the project", project, filepath.Join(project, "not", "built", "yet"), "not/built/yet"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got := pluginpolicy.DevelopmentPath(testCase.project, testCase.directory)
			assert.Equal(t, testCase.want, got)
			assert.NotContains(t, got, "..", "a relative allow-root must never match an escaping path")
			assert.NotContains(t, got, "\\")
		})
	}
}

func TestDevelopmentPathResolvesSymlinksOutOfTheProject(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating symlinks needs elevated privileges on Windows")
	}
	project, outside := t.TempDir(), t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(project, "build"), 0o700))
	link := filepath.Join(project, "build", "escape")
	require.NoError(t, os.Symlink(outside, link))

	got := pluginpolicy.DevelopmentPath(project, link)

	assert.Equal(t, filepath.ToSlash(realPath(t, outside)), got, "a link below build/ that points outside is not below build/")
	assert.NotContains(t, got, "build/escape")

	// Negative: a link that stays inside the project resolves to its target inside the project.
	target := filepath.Join(project, "real")
	require.NoError(t, os.MkdirAll(target, 0o700))
	inner := filepath.Join(project, "build", "inner")
	require.NoError(t, os.Symlink(target, inner))
	assert.Equal(t, "real", pluginpolicy.DevelopmentPath(project, inner))
}

func realPath(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	require.NoError(t, err)
	return resolved
}

func TestPathReachesPolicyInputOnlyForDevelopmentOrigin(t *testing.T) {
	inputs := recordInputs(t)
	path := pluginpolicy.WithPath("build/oidc")
	definition, descriptors := packageFixture(t)

	require.NoError(t, pluginpolicy.Source(t.Context(), "activate", "", path))
	require.NoError(t, pluginpolicy.Package(t.Context(), "activate", "", definition, descriptors, path))
	require.NoError(t, pluginpolicy.Source(t.Context(), "install", "oci://registry.example.com/acme/auth", path))
	require.NoError(t, pluginpolicy.Package(t.Context(), "install", "oci://registry.example.com/acme/auth", definition, descriptors, path, pluginpolicy.WithCache()))
	require.NoError(t, pluginpolicy.Source(t.Context(), "install", "oci://registry.example.com/acme/auth", path, pluginpolicy.WithLayout()))

	require.Len(t, *inputs, 5)
	assert.Equal(t, "development", (*inputs)[0].Origin)
	assert.Equal(t, "build/oidc", (*inputs)[0].Path)
	assert.Equal(t, "build/oidc", (*inputs)[1].Path)
	for _, input := range (*inputs)[2:] {
		assert.Empty(t, input.Path, "registry, cache and layout origins have no path: %s", input.Origin)
	}
	// Negative: a development gate that supplies no directory leaves the path empty.
	require.NoError(t, pluginpolicy.Source(t.Context(), "test", ""))
	assert.Empty(t, (*inputs)[5].Path)
}

func TestActiveProfilesReachEveryPluginGate(t *testing.T) {
	inputs := recordInputs(t)
	definition, descriptors := packageFixture(t)
	guard.SetActiveProfiles([]string{"sandbox", "alice", "sandbox", ""})

	require.NoError(t, pluginpolicy.Source(t.Context(), "install", "oci://registry.example.com/acme/auth"))
	require.NoError(t, pluginpolicy.Package(t.Context(), "install", "oci://registry.example.com/acme/auth", definition, descriptors))
	require.NoError(t, pluginpolicy.Source(t.Context(), "activate", "", pluginpolicy.WithPath("build/x")))
	require.NoError(t, pluginpolicy.Source(t.Context(), "test", ""))
	require.NoError(t, pluginpolicy.Login(t.Context(), "registry.example.com"))

	require.Len(t, *inputs, 5)
	for _, input := range *inputs {
		assert.Equal(t, []string{"sandbox", "alice"}, input.Profiles, "names as typed, de-duplicated, in activation order (%s/%s)", input.Operation, input.Origin)
	}

	// Negative: with no profile the list is empty, never nil, so it serializes as [].
	guard.SetActiveProfiles(nil)
	*inputs = nil
	require.NoError(t, pluginpolicy.Source(t.Context(), "install", "oci://registry.example.com/acme/auth"))
	require.Len(t, *inputs, 1)
	assert.NotNil(t, (*inputs)[0].Profiles)
	assert.Empty(t, (*inputs)[0].Profiles)
}

func TestProfilesSerializeAsAnEmptyListAndCannotBeMutatedThroughTheAccessor(t *testing.T) {
	t.Cleanup(func() { guard.SetActiveProfiles(nil) })
	raw, err := json.Marshal(&guard.PluginInput{Capabilities: []string{}, Profiles: guard.ActiveProfiles()})
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"profiles":[]`)
	assert.Contains(t, string(raw), `"path":""`)

	guard.SetActiveProfiles([]string{"one"})
	got := guard.ActiveProfiles()
	got[0] = "changed"
	assert.Equal(t, []string{"one"}, guard.ActiveProfiles(), "the accessor returns a copy")
}

func TestCELRuleOverProfilesAndPath(t *testing.T) {
	governingPolicy(t, `input.origin == 'development' && !('sandbox' in input.profiles) && !input.path.startsWith('build/')`)
	t.Cleanup(func() { guard.SetActiveProfiles(nil) })
	allowedRoot := pluginpolicy.WithPath("build/oidc")
	elsewhere := pluginpolicy.WithPath("scratch/oidc")

	guard.SetActiveProfiles(nil)
	require.NoError(t, pluginpolicy.Source(t.Context(), "activate", "", allowedRoot))
	require.Error(t, pluginpolicy.Source(t.Context(), "activate", "", elsewhere), "outside the approved root without the profile")

	guard.SetActiveProfiles([]string{"sandbox"})
	require.NoError(t, pluginpolicy.Source(t.Context(), "activate", "", elsewhere), "the named profile lifts the path restriction in this rule")

	guard.SetActiveProfiles([]string{"sandboxed"})
	require.Error(t, pluginpolicy.Source(t.Context(), "activate", "", elsewhere), "only an exact profile name matches")
}
