package git

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci"
	ghprovider "github.com/cloudposse/atmos/pkg/ci/providers/github"
	"github.com/cloudposse/atmos/pkg/schema"
)

// registerGitHubProviderOnly isolates the CI registry to just the GitHub
// provider so ci.Detect() resolves deterministically in tests.
func registerGitHubProviderOnly(t *testing.T) {
	t.Helper()
	restore := ci.SwapRegistryForTest()
	t.Cleanup(restore)
	ci.Register(ghprovider.NewProvider())
}

// baseRepoForTest is the base repository used by the fork-checkout gate tests.
const baseRepoForTest = "acme/infra"

// setForkPRTargetEnv puts the process in a GitHub Actions pull_request_target
// run for baseRepoForTest.
func setForkPRTargetEnv(t *testing.T) {
	t.Helper()
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("GITHUB_EVENT_NAME", "pull_request_target")
	t.Setenv("GITHUB_REPOSITORY", baseRepoForTest)
	t.Setenv("GITHUB_SERVER_URL", "https://github.com")
	// Ensure no stray event payload leaks fork metadata from the host env.
	t.Setenv("GITHUB_EVENT_PATH", "")
}

func TestGuardForkCheckout_RefusesPRRefOverrideUnderElevatedEvent(t *testing.T) {
	registerGitHubProviderOnly(t)
	setForkPRTargetEnv(t)

	err := guardForkCheckout("refs/pull/5/merge", "https://github.com/acme/infra.git", &cloneOptions{})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrUnsafeForkCheckout)
}

func TestGuardForkCheckout_RefusesCrossRepoURIUnderElevatedEvent(t *testing.T) {
	registerGitHubProviderOnly(t)
	setForkPRTargetEnv(t)

	err := guardForkCheckout("", "https://github.com/attacker/infra.git", &cloneOptions{})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrUnsafeForkCheckout)
}

func TestGuardForkCheckout_AllowsBaseCheckoutUnderElevatedEvent(t *testing.T) {
	registerGitHubProviderOnly(t)
	setForkPRTargetEnv(t)

	// No-arg CI checkout: base ref + base repo URI.
	err := guardForkCheckout("refs/heads/main", "https://github.com/acme/infra.git", &cloneOptions{})
	assert.NoError(t, err)
}

func TestGuardForkCheckout_OptInFlagDisablesGate(t *testing.T) {
	registerGitHubProviderOnly(t)
	setForkPRTargetEnv(t)

	// Same dangerous request as the refusal test, but opted in via flag/env.
	err := guardForkCheckout("refs/pull/5/merge", "https://github.com/acme/infra.git", &cloneOptions{AllowUnsafeFork: true})
	assert.NoError(t, err)
}

func TestGuardForkCheckout_OptInConfigDisablesGate(t *testing.T) {
	registerGitHubProviderOnly(t)
	setForkPRTargetEnv(t)

	original := atmosConfigPtr
	t.Cleanup(func() { atmosConfigPtr = original })
	atmosConfigPtr = &schema.AtmosConfiguration{
		CI: schema.CIConfig{AllowUnsafeForkExecution: true},
	}

	err := guardForkCheckout("refs/pull/5/merge", "https://github.com/acme/infra.git", &cloneOptions{})
	assert.NoError(t, err)
}

func TestGuardForkCheckout_NoCIProviderIsTrusted(t *testing.T) {
	// Isolate to an empty registry: no provider detected.
	restore := ci.SwapRegistryForTest()
	t.Cleanup(restore)

	err := guardForkCheckout("refs/pull/5/merge", "https://github.com/attacker/infra.git", &cloneOptions{})
	assert.NoError(t, err)
}

func TestGuardForkCheckout_NonElevatedEventIsTrusted(t *testing.T) {
	registerGitHubProviderOnly(t)
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("GITHUB_EVENT_NAME", "pull_request") // low-privilege event.
	t.Setenv("GITHUB_REPOSITORY", "acme/infra")
	t.Setenv("GITHUB_SERVER_URL", "https://github.com")

	err := guardForkCheckout("refs/pull/5/merge", "https://github.com/acme/infra.git", &cloneOptions{})
	assert.NoError(t, err)
}

func TestAllowUnsafeFork(t *testing.T) {
	original := atmosConfigPtr
	t.Cleanup(func() { atmosConfigPtr = original })

	atmosConfigPtr = nil
	assert.False(t, allowUnsafeFork(&cloneOptions{}))
	assert.True(t, allowUnsafeFork(&cloneOptions{AllowUnsafeFork: true}))

	atmosConfigPtr = &schema.AtmosConfiguration{CI: schema.CIConfig{AllowUnsafeForkExecution: true}}
	assert.True(t, allowUnsafeFork(&cloneOptions{}))

	atmosConfigPtr = &schema.AtmosConfiguration{CI: schema.CIConfig{AllowUnsafeForkExecution: false}}
	assert.False(t, allowUnsafeFork(&cloneOptions{}))
}
