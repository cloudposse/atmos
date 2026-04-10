package auth

import (
	"context"
	"errors"
	"fmt"
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
// the goroutine-safety contract on two independent vectors:
//
//  1. Per-goroutine isolation: each goroutine that requests
//     ATMOS_PROFILE=X MUST observe X inside its own InitCliConfig call —
//     not some other goroutine's value. The test gives every goroutine a
//     UNIQUE want value, so any cross-goroutine hijack manifests as a
//     missing want and an unexpected extra observation.
//  2. Read stability inside the stub: a stub that performs two reads of
//     ATMOS_PROFILE separated by a tiny sleep MUST see the same value for
//     both, otherwise another goroutine mutated os.Environ during the
//     window.
//
// All worker errors are routed back to the test goroutine via channels.
// Per testify docs, require.* / t.Fatal* must NOT be invoked from worker
// goroutines because runtime.Goexit only exits the current goroutine and
// leaves t in an inconsistent state.
//
// Without managerEnvOverridesMu both vectors fail under -race. With the
// lock the test must be deterministic.
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

	const goroutines = 32

	// Each goroutine gets a UNIQUE want value. Under correct serialization
	// the stub MUST observe exactly this value during the goroutine's own
	// InitCliConfig call. Without the lock, a goroutine could observe any
	// other goroutine's value — which the multiset comparison below detects.
	wants := make([]string, goroutines)
	for i := 0; i < goroutines; i++ {
		wants[i] = fmt.Sprintf("profile-%02d", i)
	}

	observed := make(chan string, goroutines)
	workerErrs := make(chan error, goroutines*2)

	initCliConfigFn = func(_ schema.ConfigAndStacksInfo, _ bool) (schema.AtmosConfiguration, error) {
		// Vector 2: read the env twice with a sleep between to widen the
		// race window. Under serialization the two reads must agree.
		before := os.Getenv("ATMOS_PROFILE")
		time.Sleep(time.Microsecond)
		after := os.Getenv("ATMOS_PROFILE")
		if before != after {
			// Forward the divergence as a worker error so the main goroutine
			// can fail the test. We do NOT call t.Errorf from here because
			// the stub may run on a worker goroutine.
			workerErrs <- fmt.Errorf("ATMOS_PROFILE diverged inside InitCliConfig: before=%q after=%q", before, after)
		}
		// Record what this stub call observed for the multiset check.
		observed <- after
		return schema.AtmosConfiguration{}, nil
	}
	createAuthManager = func(_ string, _ *schema.AuthConfig, _ string, _ *schema.AtmosConfiguration) (AuthManager, error) {
		return &fakeAuthMgrForEnvOverrides{marker: "ok"}, nil
	}

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for _, want := range wants {
		want := want // capture per-iteration
		go func() {
			defer wg.Done()
			_, err := CreateAndAuthenticateManagerWithEnvOverrides(map[string]string{
				"ATMOS_PROFILE": want,
			})
			if err != nil {
				// Route the error back to the main goroutine — never call
				// require.* / t.Fatal* from a worker goroutine.
				workerErrs <- fmt.Errorf("worker want=%s: %w", want, err)
			}
		}()
	}
	wg.Wait()
	close(observed)
	close(workerErrs)

	// Surface any worker / divergence errors first. Drained on the test
	// goroutine, so t.Errorf is safe here.
	for err := range workerErrs {
		t.Errorf("%v", err)
	}

	// Vector 1: multiset comparison. Build a set of expected wants, then
	// walk the observed channel marking each off. Anything missing means
	// some goroutine's read was hijacked; anything extra (or duplicated)
	// means a goroutine observed a value it didn't request.
	expectedSet := make(map[string]bool, goroutines)
	for _, w := range wants {
		expectedSet[w] = false
	}

	var extras []string
	for o := range observed {
		seen, known := expectedSet[o]
		if !known {
			extras = append(extras, fmt.Sprintf("%q (unknown)", o))
			continue
		}
		if seen {
			extras = append(extras, fmt.Sprintf("%q (duplicate)", o))
			continue
		}
		expectedSet[o] = true
	}

	var missing []string
	for w, seen := range expectedSet {
		if !seen {
			missing = append(missing, w)
		}
	}

	assert.Empty(t, missing,
		"every goroutine must have observed its own ATMOS_PROFILE under serialization; missing wants indicate the lock is broken")
	assert.Empty(t, extras,
		"no goroutine should observe a value it did not request; extras indicate cross-goroutine env leakage")
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
