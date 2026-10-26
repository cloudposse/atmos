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

// resetRegistry clears the package-global registry and once guard for test isolation.
// Safe because tests in this package run in the same process and are not parallel.
func resetRegistry() {
	registryMu.Lock()
	registry = nil
	registryMu.Unlock()
	ensureOnce = sync.Once{}
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

	assert.Equal(t, 1, p.provisioned, "EnsureCredentials must provision at most once per process")
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

// TestEnsureCredentials_ConcurrentProvisionOnce proves the sync.Once happens-before barrier:
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
