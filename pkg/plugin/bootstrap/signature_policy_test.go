package bootstrap

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	pluginlock "github.com/cloudposse/atmos/pkg/plugin/lock"
	"github.com/cloudposse/atmos/pkg/schema"
)

// A path package has no signature, so required signatures govern released packages only: policy rules
// over origin development and input.path are the gate for local builds.
func TestRequiredSignaturesDoNotGovernPathPackages(t *testing.T) {
	atmos, requests := automaticFixture(t)
	directory := localPackageFixture(t)
	declareLocal(atmos, "fixture", directory)
	// An allowlist alone enforces signature verification of released packages.
	atmos.Plugins.Policy.TrustedPublishers = []schema.PluginPublisher{{Publisher: "example", Identity: "builder@example.com", Issuer: "https://issuer.example.com"}}
	state, err := selectedPackages(t.Context(), atmos)
	require.NoError(t, err)
	require.Equal(t, directory, state.Packages["fixture"].Path)
	require.Zero(t, requests.Load(), "a path package never contacts the registry")
	_, err = os.Stat(pluginlock.Path(atmos.BasePathConfigDir))
	require.ErrorIs(t, err, os.ErrNotExist, "a path package is never locked")

	// Negative: without the path the same package is a release and the signature requirement applies.
	declareLocal(atmos, "fixture", "")
	atmos.Plugins.Policy.Offline = true
	_, err = selectedPackages(t.Context(), atmos)
	require.Error(t, err)
	require.NotErrorIs(t, err, errUtils.ErrPolicyDenied)
}

func TestCachedBootstrapCannotTrustForgedLockSignature(t *testing.T) {
	atmos, requests := automaticFixture(t)
	_, err := selectedPackages(t.Context(), atmos)
	require.NoError(t, err)
	file, err := pluginlock.Load(atmos.BasePathConfigDir)
	require.NoError(t, err)
	entry := file.Spec.Packages["fixture"]
	entry.Verification = pluginlock.Verification{Signature: "verified", Identity: "builder@example.com", Issuer: "https://issuer.example.com"}
	file.Spec.Packages["fixture"] = entry
	require.NoError(t, pluginlock.Save(atmos.BasePathConfigDir, file))
	before, err := os.ReadFile(pluginlock.Path(atmos.BasePathConfigDir))
	require.NoError(t, err)
	configBefore, err := os.ReadFile(filepath.Join(atmos.BasePathConfigDir, "atmos.yaml"))
	require.NoError(t, err)
	atmos.Plugins.Policy.Offline = true
	atmos.Plugins.Policy.TrustedPublishers = []schema.PluginPublisher{{Publisher: "example", Identity: entry.Verification.Identity, Issuer: entry.Verification.Issuer}}
	requests.Store(0)
	state, err := selectedPackages(t.Context(), atmos)
	require.ErrorIs(t, err, errUtils.ErrPluginSignatureInvalid)
	require.Nil(t, state)
	require.Zero(t, requests.Load())
	after, err := os.ReadFile(pluginlock.Path(atmos.BasePathConfigDir))
	require.NoError(t, err)
	require.Equal(t, before, after)
	configAfter, err := os.ReadFile(filepath.Join(atmos.BasePathConfigDir, "atmos.yaml"))
	require.NoError(t, err)
	require.Equal(t, configBefore, configAfter)
}
