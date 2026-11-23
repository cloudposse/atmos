package broker

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// fakeProvider is a configurable test Provider.
type fakeProvider struct {
	name        string
	enabled     bool
	env         map[string]string
	err         error
	provisioned int
}

func (f *fakeProvider) Name() string { return f.name }

func (f *fakeProvider) Enabled(*schema.AtmosConfiguration) bool { return f.enabled }

func (f *fakeProvider) Provision(context.Context, *schema.AtmosConfiguration) (map[string]string, error) {
	f.provisioned++
	return f.env, f.err
}

// resetRegistry clears the package-global registry and run state for test isolation.
// Safe because tests in this package run in the same process and are not parallel.
func resetRegistry() {
	registryMu.Lock()
	registry = nil
	registryMu.Unlock()
	resetEnsureStateForTest()
	resetBrokeredForTest()
}

func TestRegister_IgnoresNil(t *testing.T) {
	resetRegistry()
	t.Cleanup(resetRegistry)

	Register(nil)
	assert.Empty(t, snapshot(), "nil provider must not be registered")
}

func TestRunEnabledBrokers_ExportsEnvForEnabledOnly(t *testing.T) {
	resetRegistry()
	t.Cleanup(resetRegistry)

	enabledKey := "ATMOS_BROKER_TEST_ENABLED"
	disabledKey := "ATMOS_BROKER_TEST_DISABLED"
	t.Setenv(enabledKey, "")
	t.Setenv(disabledKey, "")
	require.NoError(t, os.Unsetenv(enabledKey))
	require.NoError(t, os.Unsetenv(disabledKey))

	on := &fakeProvider{name: "on", enabled: true, env: map[string]string{enabledKey: "yes"}}
	off := &fakeProvider{name: "off", enabled: false, env: map[string]string{disabledKey: "no"}}
	Register(on)
	Register(off)

	runEnabledBrokers(context.Background(), &schema.AtmosConfiguration{})

	assert.Equal(t, 1, on.provisioned, "enabled broker should be provisioned")
	assert.Equal(t, 0, off.provisioned, "disabled broker should be skipped")
	assert.Equal(t, "yes", os.Getenv(enabledKey), "enabled broker env must be exported")
	_, present := os.LookupEnv(disabledKey)
	assert.False(t, present, "disabled broker env must not be exported")
}

func TestRunEnabledBrokers_ProvisionErrorIsNonFatal(t *testing.T) {
	resetRegistry()
	t.Cleanup(resetRegistry)

	okKey := "ATMOS_BROKER_TEST_OK"
	require.NoError(t, os.Unsetenv(okKey))

	bad := &fakeProvider{name: "bad", enabled: true, err: errors.New("mint failed")}
	good := &fakeProvider{name: "good", enabled: true, env: map[string]string{okKey: "1"}}
	Register(bad)
	Register(good)

	// Must not panic; the good broker still runs and exports its env.
	runEnabledBrokers(context.Background(), &schema.AtmosConfiguration{})

	assert.Equal(t, 1, bad.provisioned)
	assert.Equal(t, "1", os.Getenv(okKey))
	require.NoError(t, os.Unsetenv(okKey))
}

func TestEnsureCredentials_ProcessOnce(t *testing.T) {
	resetRegistry()
	t.Cleanup(resetRegistry)

	p := &fakeProvider{name: "once", enabled: true}
	Register(p)

	cfg := &schema.AtmosConfiguration{}
	EnsureCredentials(context.Background(), cfg)
	EnsureCredentials(context.Background(), cfg)

	assert.Equal(t, 1, p.provisioned, "EnsureCredentials must not re-provision within the refresh interval")
}

func TestEnsureCredentials_NoProviders(t *testing.T) {
	resetRegistry()
	t.Cleanup(resetRegistry)

	// No registered providers: must be a safe no-op.
	assert.NotPanics(t, func() {
		EnsureCredentials(context.Background(), &schema.AtmosConfiguration{})
	})
}

// TestHasBrokeredCredentials_TrueAfterNonEmptyProvision asserts that a broker which exports
// a non-empty environment flips HasBrokeredCredentials to true.
func TestHasBrokeredCredentials_TrueAfterNonEmptyProvision(t *testing.T) {
	resetRegistry()
	t.Cleanup(resetRegistry)

	key := "ATMOS_BROKER_TEST_BROKERED"
	require.NoError(t, os.Unsetenv(key))

	assert.False(t, HasBrokeredCredentials(), "no brokering should have happened yet")

	Register(&fakeProvider{name: "mints", enabled: true, env: map[string]string{key: "tok"}})
	runEnabledBrokers(context.Background(), &schema.AtmosConfiguration{})

	assert.True(t, HasBrokeredCredentials(), "a non-empty provision must mark brokered credentials")
	require.NoError(t, os.Unsetenv(key))
}

// TestHasBrokeredCredentials_FalseWhenEmptyOrErrored asserts that an empty-env or erroring
// broker does not mark brokered credentials, so downstream auth-retry stays disabled.
func TestHasBrokeredCredentials_FalseWhenEmptyOrErrored(t *testing.T) {
	resetRegistry()
	t.Cleanup(resetRegistry)

	// Enabled but returns an empty env (no token), and an enabled broker that errors.
	Register(&fakeProvider{name: "empty", enabled: true, env: map[string]string{}})
	Register(&fakeProvider{name: "boom", enabled: true, err: errors.New("mint failed")})
	runEnabledBrokers(context.Background(), &schema.AtmosConfiguration{})

	assert.False(t, HasBrokeredCredentials(), "no token was provisioned, so nothing is brokered")
}

// TestHasBrokeredCredentials_FalseWhenExportFails asserts that when every os.Setenv fails
// (here via an invalid empty key), no credentials reach the process env and brokered state
// is NOT marked — so downstream auth-retry is not enabled without usable credentials.
func TestHasBrokeredCredentials_FalseWhenExportFails(t *testing.T) {
	resetRegistry()
	t.Cleanup(resetRegistry)

	// An empty key is rejected by os.Setenv on all platforms, so the export loop fails for
	// every entry.
	Register(&fakeProvider{name: "unexportable", enabled: true, env: map[string]string{"": "value"}})
	runEnabledBrokers(context.Background(), &schema.AtmosConfiguration{})

	assert.False(t, HasBrokeredCredentials(), "no variable was exported, so nothing is brokered")
}

// TestEnsureCredentials_ConcurrentProvisionOnce proves the ensureMu happens-before barrier:
// concurrent callers provision exactly once AND every caller observes the exported env after
// EnsureCredentials returns (the credential is in place before any caller proceeds to clone).
// Run under -race to catch ordering regressions if vendoring is ever parallelized.
func TestEnsureCredentials_ConcurrentProvisionOnce(t *testing.T) {
	resetRegistry()
	t.Cleanup(resetRegistry)

	key := "ATMOS_BROKER_TEST_CONCURRENT"
	require.NoError(t, os.Unsetenv(key))

	p := &fakeProvider{name: "once", enabled: true, env: map[string]string{key: "ready"}}
	Register(p)

	const goroutines = 16
	var wg sync.WaitGroup
	observed := make([]string, goroutines)
	wg.Add(goroutines)
	for i := range goroutines {
		go func(idx int) {
			defer wg.Done()
			EnsureCredentials(context.Background(), &schema.AtmosConfiguration{})
			observed[idx] = os.Getenv(key)
		}(i)
	}
	wg.Wait()

	assert.Equal(t, 1, p.provisioned, "concurrent EnsureCredentials must provision exactly once")
	assert.True(t, HasBrokeredCredentials())
	for i, v := range observed {
		assert.Equal(t, "ready", v, "goroutine %d must observe the exported credential after EnsureCredentials returns", i)
	}
	require.NoError(t, os.Unsetenv(key))
}

// GIT_CONFIG_COUNT is published after the indexed pairs it counts, so a concurrent subprocess
// never sees a count larger than the number of complete pairs.
func TestExportOrder_GitConfigCountLast(t *testing.T) {
	env := map[string]string{
		"GIT_CONFIG_COUNT":   "2",
		"GIT_CONFIG_KEY_0":   "k0",
		"GIT_CONFIG_VALUE_0": "v0",
		"GIT_CONFIG_KEY_1":   "k1",
		"GIT_CONFIG_VALUE_1": "v1",
		"GH_TOKEN":           "t",
	}

	order := exportOrder(env)

	require.Len(t, order, len(env))
	assert.Equal(t, "GIT_CONFIG_COUNT", order[len(order)-1])
	assert.NotContains(t, order[:len(order)-1], "GIT_CONFIG_COUNT")
	assert.Equal(t, []string{"GH_TOKEN", "GIT_CONFIG_KEY_0", "GIT_CONFIG_KEY_1", "GIT_CONFIG_VALUE_0", "GIT_CONFIG_VALUE_1", "GIT_CONFIG_COUNT"}, order)
	// Without a count nothing is appended.
	assert.Equal(t, []string{"A"}, exportOrder(map[string]string{"A": "1"}))
}

func TestRefreshInterval_MatchesSchedule(t *testing.T) {
	assert.Equal(t, refreshInterval, RefreshInterval())
}

// TestRunEnabledBrokers_PartialEnvWithErrorIsFailedRun asserts that a broker returning both an
// environment and an error exports the environment but reports the run as failed, so it is
// retried on retryBackoff rather than waiting the full refreshInterval.
func TestRunEnabledBrokers_PartialEnvWithErrorIsFailedRun(t *testing.T) {
	resetRegistry()
	t.Cleanup(resetRegistry)

	key := "ATMOS_BROKER_TEST_PARTIAL"
	require.NoError(t, os.Unsetenv(key))
	t.Cleanup(func() { _ = os.Unsetenv(key) })

	Register(&fakeProvider{name: "partial", enabled: true, env: map[string]string{key: "1"}, err: errors.New("mint failed")})

	ok := runEnabledBrokers(context.Background(), &schema.AtmosConfiguration{})

	assert.False(t, ok, "a broker error must mark the run failed")
	assert.Equal(t, "1", os.Getenv(key), "partial env must still be exported")
}
