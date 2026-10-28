package bootstrap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/plugin/install"
	pluginlock "github.com/cloudposse/atmos/pkg/plugin/lock"
	"github.com/cloudposse/atmos/pkg/policy/admission"
	"github.com/cloudposse/atmos/pkg/policy/guard"
)

func bootstrapAdmissionPolicy(t *testing.T, engine, predicate string) {
	t.Helper()
	bootstrapAdmissionPolicyWith(t, engine, predicate, "")
}

// bootstrapAdmissionPolicyWith adds the rule's parameters, given as the YAML body of its with: map.
func bootstrapAdmissionPolicyWith(t *testing.T, engine, predicate, parameters string) {
	t.Helper()
	t.Cleanup(ci.SwapRegistryForTest())
	directory := t.TempDir()
	t.Setenv("ATMOS_POLICY_DIR", directory)
	with := ""
	if parameters != "" {
		with = "      with:\n        " + parameters + "\n"
	}
	document := "apiVersion: atmos/v1\nkind: Policy\nmetadata: {name: corporate}\nspec:\n  policies:\n    plugins:\n      on: plugin\n      action: deny\n      engine: " + engine + "\n" + with + "      rule: |\n        " + predicate + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(directory, "plugins.yaml"), []byte(document), 0o600))
	restore, err := admission.BeginInvocation()
	require.NoError(t, err)
	t.Cleanup(restore)
}

func TestAdmissionAutomaticInstallDeniesBeforeNetworkAndLockMutation(t *testing.T) {
	for _, tt := range []struct{ engine, rule string }{
		{engine: "cel", rule: `input.registry != 'corporate.example.com'`},
		{engine: "starlark", rule: `return ctx.input["registry"] != "corporate.example.com"`},
	} {
		t.Run(tt.engine, func(t *testing.T) {
			atmos, requests := automaticFixture(t)
			path := filepath.Join(atmos.BasePathConfigDir, "atmos.yaml")
			before, err := os.ReadFile(path)
			require.NoError(t, err)
			bootstrapAdmissionPolicy(t, tt.engine, tt.rule)

			_, err = selectedPackages(t.Context(), atmos)

			require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
			require.Zero(t, requests.Load(), "a forbidden registry must not be contacted")
			_, err = os.Stat(pluginlock.Path(atmos.BasePathConfigDir))
			require.ErrorIs(t, err, os.ErrNotExist, "a denied install must not create a lockfile")
			after, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, before, after)
			entries, err := os.ReadDir(atmos.Plugins.CacheDir)
			require.NoError(t, err)
			require.Empty(t, entries)
		})
	}
}

func TestAdmissionCachedActivationRechecksCurrentPolicy(t *testing.T) {
	for _, tt := range []struct{ name, rule string }{
		{name: "source", rule: `input.registry != 'corporate.example.com'`},
		{name: "inspected package", rule: `input.phase == 'package' && input.name == 'example/automatic'`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			atmos, requests := automaticFixture(t)
			_, err := selectedPackages(t.Context(), atmos)
			require.NoError(t, err)
			before, err := os.ReadFile(pluginlock.Path(atmos.BasePathConfigDir))
			require.NoError(t, err)
			requests.Store(0)
			atmos.Plugins.Policy.Offline = true
			bootstrapAdmissionPolicy(t, "cel", tt.rule)

			_, err = selectedPackages(t.Context(), atmos)

			require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
			require.Zero(t, requests.Load())
			after, err := os.ReadFile(pluginlock.Path(atmos.BasePathConfigDir))
			require.NoError(t, err)
			require.Equal(t, before, after, "new policy must deny activation without rewriting the lock")
		})
	}
}

func TestAdmissionPathPackageActivationUsesCurrentPolicy(t *testing.T) {
	for _, tt := range []struct {
		name, rule string
		allowed    bool
	}{
		{name: "local denied", rule: `input.registry != 'corporate.example.com'`},
		{name: "local exempted", rule: `!input.development && input.registry != 'corporate.example.com'`, allowed: true},
		{name: "inspected name denied", rule: `input.phase == 'package' && input.name == 'example/automatic'`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			atmos, requests := automaticFixture(t)
			directory := localPackageFixture(t)
			declareLocal(atmos, "fixture", directory)
			bootstrapAdmissionPolicy(t, "cel", tt.rule)

			state, err := selectedPackages(t.Context(), atmos)

			if tt.allowed {
				require.NoError(t, err)
				require.Equal(t, directory, state.Packages["fixture"].Path)
			} else {
				require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
			}
			require.Zero(t, requests.Load())
			_, err = os.Stat(pluginlock.Path(atmos.BasePathConfigDir))
			require.ErrorIs(t, err, os.ErrNotExist)
		})
	}
}

// The documented allow-root rule: a path package is admitted only below approved project roots. This
// proves the CEL helpers exists and startsWith, and the params map, work in the policy engine.
func TestAdmissionPathPackageAllowRoots(t *testing.T) {
	const rule = `input.origin == 'development' && !params.dev_roots.exists(r, input.path.startsWith(r))`
	for _, tt := range []struct {
		name    string
		relDir  string // Relative to the project; empty places the plugin outside the project.
		allowed bool
	}{
		{name: "below an approved root", relDir: filepath.Join("build", "fixture"), allowed: true},
		{name: "nested below an approved root", relDir: filepath.Join("build", "plugins", "fixture"), allowed: true},
		{name: "other project directory", relDir: filepath.Join("scratch", "fixture")},
		{name: "outside the project", relDir: ""},
		{name: "look-alike of the root", relDir: "build-old"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			atmos, _ := automaticFixture(t)
			directory := localPackageFixture(t)
			if tt.relDir != "" {
				directory = filepath.Join(atmos.BasePathConfigDir, tt.relDir)
				require.NoError(t, os.MkdirAll(filepath.Join(directory, "bin"), 0o700))
				require.NoError(t, os.WriteFile(filepath.Join(directory, "bin", "plugin"), []byte("static"), 0o700))
				raw := strings.ReplaceAll(automaticManifest, "PLATFORM", install.CurrentPlatform())
				require.NoError(t, os.WriteFile(filepath.Join(directory, "plugin.yaml"), []byte(raw), 0o600))
			}
			declareLocal(atmos, "fixture", directory)
			bootstrapAdmissionPolicyWith(t, "cel", rule, "dev_roots: ['build/']")

			_, err := selectedPackages(t.Context(), atmos)

			if tt.allowed {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
		})
	}
}

// Rules may use input.profiles, but the origin and path stay the dependable gate.
func TestAdmissionPathPackageSeesActiveProfiles(t *testing.T) {
	const rule = `input.origin == 'development' && !('sandbox' in input.profiles)`
	for _, tt := range []struct {
		name     string
		profiles []string
		allowed  bool
	}{
		{name: "matching profile", profiles: []string{"base", "sandbox"}, allowed: true},
		{name: "other profile", profiles: []string{"alice"}},
		{name: "no profile"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			atmos, _ := automaticFixture(t)
			declareLocal(atmos, "fixture", localPackageFixture(t))
			bootstrapAdmissionPolicy(t, "cel", rule)
			guard.SetActiveProfiles(tt.profiles)
			t.Cleanup(func() { guard.SetActiveProfiles(nil) })

			_, err := selectedPackages(t.Context(), atmos)

			if tt.allowed {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
		})
	}
}
