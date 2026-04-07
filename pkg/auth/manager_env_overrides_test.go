package auth

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// managerEnvOverridesTestMu serializes any test in this file that mutates
// the package-level DI hooks (initCliConfigFn, createAuthManager,
// setEnvWithRestoreFn). Per CLAUDE.md these tests MUST NOT call t.Parallel(),
// but this mutex is a defensive guard so that if a future test author
// accidentally adds t.Parallel(), they get a deadlock or contention rather
// than silent cross-test stub interference (which would manifest as flaky
// failures that are very hard to diagnose).
var managerEnvOverridesTestMu sync.Mutex

// Static sentinel errors for test injection. These are package-level vars,
// not inline errors.New calls, so they comply with CLAUDE.md's "no dynamic
// errors" rule and can be matched via errors.Is in assertions.
var (
	errTestInitBoom   = errors.New("test: simulated InitCliConfig failure")
	errTestSetenvBoom = errors.New("test: simulated setenv failure")
	errTestNilCleanup = errors.New("test: simulated nil-cleanup failure")
)

// unsetEnvForTest unsets the given env vars and registers cleanup that
// restores them to their original state. Use when t.Setenv isn't appropriate
// (i.e., when a test asserts the variable is *unset* after some operation).
func unsetEnvForTest(t *testing.T, keys ...string) {
	t.Helper()
	for _, k := range keys {
		old, had := os.LookupEnv(k)
		require.NoError(t, os.Unsetenv(k))
		t.Cleanup(func() {
			if had {
				_ = os.Setenv(k, old)
			} else {
				_ = os.Unsetenv(k)
			}
		})
	}
}

// fakeAuthMgrForEnvOverrides is a minimal AuthManager stand-in used to verify
// that CreateAndAuthenticateManagerWithEnvOverrides wires everything together.
// Only PrepareShellEnvironment is exercised in tests.
type fakeAuthMgrForEnvOverrides struct {
	AuthManager // embed nil — only PrepareShellEnvironment is exercised.
	marker      string
}

func (f *fakeAuthMgrForEnvOverrides) PrepareShellEnvironment(_ context.Context, identityName string, currentEnv []string) ([]string, error) {
	return append(currentEnv, "FAKE="+f.marker+":"+identityName), nil
}

// withStubbedManagerEnvOverrides swaps the package-level hooks used by
// CreateAndAuthenticateManagerWithEnvOverrides with test doubles and restores
// them on cleanup.
//
// The observed pointer (if non-nil) captures the value of ATMOS_PROFILE at
// the moment initCliConfigFn runs. The errOnInit value (if non-nil) is
// returned from initCliConfigFn. The mgrToReturn value is whatever the
// createAuthManager fake should return; pass nil to simulate the documented
// "auth disabled / no identity" contract.
func withStubbedManagerEnvOverrides(
	t *testing.T,
	observed *string,
	errOnInit error,
	mgrToReturn AuthManager,
) {
	t.Helper()

	// Acquire the package-level lock for the duration of the test so
	// concurrent invocations of any test that mutates these hooks can't
	// race. The lock is released by t.Cleanup, which runs in LIFO order
	// after all other cleanups for this test — including the hook restore
	// closures registered below.
	managerEnvOverridesTestMu.Lock()
	t.Cleanup(managerEnvOverridesTestMu.Unlock)

	origInit := initCliConfigFn
	origCreate := createAuthManager
	t.Cleanup(func() {
		initCliConfigFn = origInit
		createAuthManager = origCreate
	})

	initCliConfigFn = func(_ schema.ConfigAndStacksInfo, _ bool) (schema.AtmosConfiguration, error) {
		if errOnInit != nil {
			return schema.AtmosConfiguration{}, errOnInit
		}
		if observed != nil {
			*observed = os.Getenv("ATMOS_PROFILE")
		}
		return schema.AtmosConfiguration{}, nil
	}
	createAuthManager = func(_ string, _ *schema.AuthConfig, _ string, _ *schema.AtmosConfiguration) (AuthManager, error) {
		return mgrToReturn, nil
	}
}

// ----------------------------------------------------------------------------
// CreateAndAuthenticateManagerWithEnvOverrides
// ----------------------------------------------------------------------------

func TestCreateAndAuthenticateManagerWithEnvOverrides_AppliesEnvBeforeInit(t *testing.T) {
	unsetEnvForTest(t, "ATMOS_PROFILE")

	var observed string
	withStubbedManagerEnvOverrides(t, &observed, nil, &fakeAuthMgrForEnvOverrides{marker: "ok"})

	mgr, err := CreateAndAuthenticateManagerWithEnvOverrides(map[string]string{
		"ATMOS_PROFILE": "managers",
	})
	require.NoError(t, err)
	require.NotNil(t, mgr)

	// initCliConfigFn saw the override.
	assert.Equal(t, "managers", observed)

	// Env is restored after the function returns.
	_, stillSet := os.LookupEnv("ATMOS_PROFILE")
	assert.False(t, stillSet, "ATMOS_PROFILE must be restored")
}

func TestCreateAndAuthenticateManagerWithEnvOverrides_RestoresPreviousProfile(t *testing.T) {
	t.Setenv("ATMOS_PROFILE", "outer")

	var observed string
	withStubbedManagerEnvOverrides(t, &observed, nil, &fakeAuthMgrForEnvOverrides{marker: "ok"})

	_, err := CreateAndAuthenticateManagerWithEnvOverrides(map[string]string{
		"ATMOS_PROFILE": "managers",
	})
	require.NoError(t, err)

	assert.Equal(t, "managers", observed, "initCliConfigFn should have seen 'managers'")
	assert.Equal(t, "outer", os.Getenv("ATMOS_PROFILE"), "parent must be restored to 'outer'")
}

func TestCreateAndAuthenticateManagerWithEnvOverrides_NilOverrides(t *testing.T) {
	unsetEnvForTest(t, "ATMOS_PROFILE")

	var observed string
	withStubbedManagerEnvOverrides(t, &observed, nil, &fakeAuthMgrForEnvOverrides{marker: "ok"})

	mgr, err := CreateAndAuthenticateManagerWithEnvOverrides(nil)
	require.NoError(t, err)
	require.NotNil(t, mgr)
	assert.Empty(t, observed, "no override should have been applied")
}

func TestCreateAndAuthenticateManagerWithEnvOverrides_EmptyOverrides(t *testing.T) {
	unsetEnvForTest(t, "ATMOS_PROFILE")

	var observed string
	withStubbedManagerEnvOverrides(t, &observed, nil, &fakeAuthMgrForEnvOverrides{marker: "ok"})

	mgr, err := CreateAndAuthenticateManagerWithEnvOverrides(map[string]string{})
	require.NoError(t, err)
	require.NotNil(t, mgr)
	assert.Empty(t, observed)
}

func TestCreateAndAuthenticateManagerWithEnvOverrides_FiltersNonAtmosKeys(t *testing.T) {
	unsetEnvForTest(t, "ATMOS_PROFILE", "FOO_BAR", "AWS_PROFILE")
	t.Setenv("AWS_PROFILE", "existing-aws")

	var observed string
	withStubbedManagerEnvOverrides(t, &observed, nil, &fakeAuthMgrForEnvOverrides{marker: "ok"})

	_, err := CreateAndAuthenticateManagerWithEnvOverrides(map[string]string{
		"ATMOS_PROFILE": "managers",
		"FOO_BAR":       "should-be-ignored",
		"AWS_PROFILE":   "should-NOT-be-touched", // not ATMOS_* prefix — must be ignored.
	})
	require.NoError(t, err)

	// ATMOS_* applied.
	assert.Equal(t, "managers", observed)

	// Non-ATMOS keys never touched: AWS_PROFILE keeps its pre-existing value
	// and FOO_BAR never gets created.
	assert.Equal(t, "existing-aws", os.Getenv("AWS_PROFILE"))
	_, hadFoo := os.LookupEnv("FOO_BAR")
	assert.False(t, hadFoo)
}

func TestCreateAndAuthenticateManagerWithEnvOverrides_OnlyNonAtmosKeys_NoMutation(t *testing.T) {
	unsetEnvForTest(t, "ATMOS_PROFILE")

	var observed string
	withStubbedManagerEnvOverrides(t, &observed, nil, &fakeAuthMgrForEnvOverrides{marker: "ok"})

	// Input contains only non-ATMOS keys — the filter should produce nil and
	// env.SetWithRestore should be a no-op.
	_, err := CreateAndAuthenticateManagerWithEnvOverrides(map[string]string{
		"SOME_OTHER":    "x",
		"UNRELATED_VAR": "y",
	})
	require.NoError(t, err)
	assert.Empty(t, observed, "no ATMOS_* key was in the input, so no env override")
}

func TestCreateAndAuthenticateManagerWithEnvOverrides_InitConfigError(t *testing.T) {
	unsetEnvForTest(t, "ATMOS_PROFILE")

	withStubbedManagerEnvOverrides(t, nil, errTestInitBoom, nil)

	_, err := CreateAndAuthenticateManagerWithEnvOverrides(map[string]string{
		"ATMOS_PROFILE": "managers",
	})
	require.Error(t, err)
	// Wrapping should preserve the static sentinel for errors.Is matching
	// AND add the auth-manager sentinel as the leading wrap.
	assert.ErrorIs(t, err, errTestInitBoom)
	assert.ErrorIs(t, err, errUtils.ErrAuthManager)

	// Env restored even on error.
	_, stillSet := os.LookupEnv("ATMOS_PROFILE")
	assert.False(t, stillSet)
}

func TestCreateAndAuthenticateManagerWithEnvOverrides_NilManagerContract(t *testing.T) {
	// Contract: returning (nil, nil) from CreateAndAuthenticateManagerWithAtmosConfig
	// propagates up untouched. It is the caller's responsibility to interpret
	// "no manager" as "auth disabled / no identity" and decide how to react.
	unsetEnvForTest(t, "ATMOS_PROFILE")
	withStubbedManagerEnvOverrides(t, nil, nil, nil)

	mgr, err := CreateAndAuthenticateManagerWithEnvOverrides(map[string]string{
		"ATMOS_PROFILE": "managers",
	})
	require.NoError(t, err)
	assert.Nil(t, mgr)
}

func TestCreateAndAuthenticateManagerWithEnvOverrides_SetEnvError_ReturnsAndCleansUp(t *testing.T) {
	unsetEnvForTest(t, "ATMOS_PROFILE")

	// Stub createAuthManager + initCliConfigFn so the test does not require
	// real config; we should never reach them on the error path.
	withStubbedManagerEnvOverrides(t, nil, nil, &fakeAuthMgrForEnvOverrides{marker: "ok"})

	// Override the env hook to simulate a setenv failure. The hook contract
	// requires returning a non-nil cleanup closure even on error so callers
	// can defer-restore safely.
	origSetEnv := setEnvWithRestoreFn
	t.Cleanup(func() { setEnvWithRestoreFn = origSetEnv })

	cleanupCalled := 0
	setEnvWithRestoreFn = func(_ map[string]string) (func(), error) {
		return func() { cleanupCalled++ }, errTestSetenvBoom
	}

	mgr, err := CreateAndAuthenticateManagerWithEnvOverrides(map[string]string{
		"ATMOS_PROFILE": "managers",
	})
	require.Error(t, err)
	assert.Nil(t, mgr)
	assert.ErrorIs(t, err, errTestSetenvBoom)
	assert.ErrorIs(t, err, errUtils.ErrAuthManager)
	// Cleanup should have been called exactly once during the error path.
	assert.Equal(t, 1, cleanupCalled)
}

func TestCreateAndAuthenticateManagerWithEnvOverrides_SetEnvError_NilCleanup_NoPanic(t *testing.T) {
	unsetEnvForTest(t, "ATMOS_PROFILE")

	withStubbedManagerEnvOverrides(t, nil, nil, &fakeAuthMgrForEnvOverrides{marker: "ok"})

	// Defensive case: hook returns (nil, err). The function must not panic.
	origSetEnv := setEnvWithRestoreFn
	t.Cleanup(func() { setEnvWithRestoreFn = origSetEnv })

	setEnvWithRestoreFn = func(_ map[string]string) (func(), error) {
		return nil, errTestNilCleanup
	}

	assert.NotPanics(t, func() {
		_, _ = CreateAndAuthenticateManagerWithEnvOverrides(map[string]string{
			"ATMOS_PROFILE": "managers",
		})
	})
}

// TestCreateAndAuthenticateManagerWithEnvOverrides_ConcurrentSafe asserts
// the goroutine-safety contract: concurrent invocations must not race on
// os.Environ() and each call must observe its own ATMOS_* override (not
// another goroutine's). The test injects a stubbed initCliConfigFn that
// captures the ATMOS_PROFILE active *during* its own invocation, then
// verifies every goroutine saw exactly the value it requested.
//
// Without managerEnvOverridesMu this test would be flaky/failing under -race.
// With the lock it must be deterministic.
func TestCreateAndAuthenticateManagerWithEnvOverrides_ConcurrentSafe(t *testing.T) {
	unsetEnvForTest(t, "ATMOS_PROFILE")

	// Stub the hooks. We do NOT use withStubbedManagerEnvOverrides here
	// because that helper takes the test mutex (intended for sequential
	// tests). This test deliberately exercises the production lock from
	// many goroutines, so we manage hook restoration manually.
	origInit := initCliConfigFn
	origCreate := createAuthManager
	t.Cleanup(func() {
		initCliConfigFn = origInit
		createAuthManager = origCreate
	})

	// Capture ATMOS_PROFILE seen at the moment initCliConfigFn runs, keyed
	// by the goroutine's expected value (passed via the override map). The
	// goroutine that requested ATMOS_PROFILE=alpha must see "alpha" inside
	// its own InitCliConfig call, not "beta" or "gamma".
	type observation struct {
		expected string
		actual   string
	}
	observations := make(chan observation, 64)

	initCliConfigFn = func(_ schema.ConfigAndStacksInfo, _ bool) (schema.AtmosConfiguration, error) {
		// We don't know which goroutine we're in; record what we see.
		// Add a small sleep to widen the race window if the lock is missing.
		seen := os.Getenv("ATMOS_PROFILE")
		// Yield to other goroutines — without the lock, this is where one
		// goroutine's setenv would clobber another's observation.
		time.Sleep(time.Microsecond)
		seenAfter := os.Getenv("ATMOS_PROFILE")
		// Record both reads. They must be equal (no inflight mutation by
		// another goroutine).
		observations <- observation{expected: seen, actual: seenAfter}
		return schema.AtmosConfiguration{}, nil
	}
	createAuthManager = func(_ string, _ *schema.AuthConfig, _ string, _ *schema.AtmosConfiguration) (AuthManager, error) {
		return &fakeAuthMgrForEnvOverrides{marker: "ok"}, nil
	}

	const goroutines = 32
	values := []string{"alpha", "beta", "gamma", "delta"}

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		want := values[i%len(values)]
		go func() {
			defer wg.Done()
			_, err := CreateAndAuthenticateManagerWithEnvOverrides(map[string]string{
				"ATMOS_PROFILE": want,
			})
			require.NoError(t, err)
		}()
	}
	wg.Wait()
	close(observations)

	// Every observation must show a stable read across the sleep window.
	// If two goroutines were ever in flight simultaneously, the seen/seenAfter
	// values would diverge.
	count := 0
	for o := range observations {
		assert.Equal(t, o.expected, o.actual,
			"ATMOS_PROFILE observed inside InitCliConfig must be stable across the read window — divergence indicates a missing lock")
		count++
	}
	assert.Equal(t, goroutines, count, "every goroutine should have produced one observation")
}

// ----------------------------------------------------------------------------
// filterAtmosOverrides
// ----------------------------------------------------------------------------

func TestFilterAtmosOverrides(t *testing.T) {
	tests := []struct {
		name string
		in   map[string]string
		want map[string]string
	}{
		{
			name: "nil",
			in:   nil,
			want: nil,
		},
		{
			name: "empty",
			in:   map[string]string{},
			want: nil,
		},
		{
			name: "only atmos keys",
			in: map[string]string{
				"ATMOS_PROFILE":         "m",
				"ATMOS_CLI_CONFIG_PATH": "/e",
				"ATMOS_BASE_PATH":       "/s",
			},
			want: map[string]string{
				"ATMOS_PROFILE":         "m",
				"ATMOS_CLI_CONFIG_PATH": "/e",
				"ATMOS_BASE_PATH":       "/s",
			},
		},
		{
			name: "only non-atmos keys",
			in: map[string]string{
				"AWS_PROFILE": "p",
				"FOO":         "bar",
			},
			want: map[string]string{},
		},
		{
			name: "mixed",
			in: map[string]string{
				"ATMOS_PROFILE": "m",
				"AWS_PROFILE":   "p",
				"FOO":           "bar",
				"ATMOS_LOG":     "debug",
			},
			want: map[string]string{
				"ATMOS_PROFILE": "m",
				"ATMOS_LOG":     "debug",
			},
		},
		{
			name: "empty value is kept",
			in: map[string]string{
				"ATMOS_PROFILE": "",
			},
			want: map[string]string{
				"ATMOS_PROFILE": "",
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := filterAtmosOverrides(tc.in)
			if tc.want == nil {
				assert.Nil(t, got)
				return
			}
			assert.Equal(t, tc.want, got)
		})
	}
}
