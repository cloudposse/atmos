package env

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// unsetForTest unsets the given env vars and registers cleanup that restores
// them to their original state. Use when t.Setenv isn't appropriate because
// the test asserts the variable is *unset* after some operation.
func unsetForTest(t *testing.T, keys ...string) {
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

func TestSetWithRestore_SetsAndRestores_NoPreviousValue(t *testing.T) {
	unsetForTest(t, "TEST_SETRESTORE_A", "TEST_SETRESTORE_B")

	restore, err := SetWithRestore(map[string]string{
		"TEST_SETRESTORE_A": "alpha",
		"TEST_SETRESTORE_B": "beta",
	})
	require.NoError(t, err)

	assert.Equal(t, "alpha", os.Getenv("TEST_SETRESTORE_A"))
	assert.Equal(t, "beta", os.Getenv("TEST_SETRESTORE_B"))

	restore()

	// Both were unset before the call, so both should be unset after restore.
	_, hadA := os.LookupEnv("TEST_SETRESTORE_A")
	_, hadB := os.LookupEnv("TEST_SETRESTORE_B")
	assert.False(t, hadA, "key with no previous value must be unset on restore")
	assert.False(t, hadB, "key with no previous value must be unset on restore")
}

func TestSetWithRestore_RestoresPreviousValue(t *testing.T) {
	t.Setenv("TEST_SETRESTORE_KEEP", "original")

	restore, err := SetWithRestore(map[string]string{
		"TEST_SETRESTORE_KEEP": "overridden",
	})
	require.NoError(t, err)
	assert.Equal(t, "overridden", os.Getenv("TEST_SETRESTORE_KEEP"))

	restore()
	assert.Equal(t, "original", os.Getenv("TEST_SETRESTORE_KEEP"))
}

func TestSetWithRestore_MixedPreviousState(t *testing.T) {
	t.Setenv("TEST_SETRESTORE_SET", "before")
	unsetForTest(t, "TEST_SETRESTORE_UNSET")

	restore, err := SetWithRestore(map[string]string{
		"TEST_SETRESTORE_SET":   "after",
		"TEST_SETRESTORE_UNSET": "new",
	})
	require.NoError(t, err)

	assert.Equal(t, "after", os.Getenv("TEST_SETRESTORE_SET"))
	assert.Equal(t, "new", os.Getenv("TEST_SETRESTORE_UNSET"))

	restore()

	// Previously-set key restored to its old value.
	assert.Equal(t, "before", os.Getenv("TEST_SETRESTORE_SET"))
	// Previously-unset key is unset again.
	_, had := os.LookupEnv("TEST_SETRESTORE_UNSET")
	assert.False(t, had)
}

func TestSetWithRestore_EmptyAndNilMap_NoOp(t *testing.T) {
	t.Setenv("TEST_SETRESTORE_STABLE", "stable")

	restore, err := SetWithRestore(nil)
	require.NoError(t, err)
	assert.Equal(t, "stable", os.Getenv("TEST_SETRESTORE_STABLE"))
	restore()
	assert.Equal(t, "stable", os.Getenv("TEST_SETRESTORE_STABLE"))

	restore2, err := SetWithRestore(map[string]string{})
	require.NoError(t, err)
	assert.Equal(t, "stable", os.Getenv("TEST_SETRESTORE_STABLE"))
	restore2()
	assert.Equal(t, "stable", os.Getenv("TEST_SETRESTORE_STABLE"))
}

func TestSetWithRestore_RestoreIsIdempotent(t *testing.T) {
	unsetForTest(t, "TEST_SETRESTORE_IDEMPOTENT")

	restore, err := SetWithRestore(map[string]string{
		"TEST_SETRESTORE_IDEMPOTENT": "value",
	})
	require.NoError(t, err)
	assert.Equal(t, "value", os.Getenv("TEST_SETRESTORE_IDEMPOTENT"))

	restore()
	_, had := os.LookupEnv("TEST_SETRESTORE_IDEMPOTENT")
	assert.False(t, had)

	// A second invocation must not re-mutate state. Prove this by setting the
	// var to a new value between restores: the second restore should leave it
	// alone, not unset it again.
	t.Setenv("TEST_SETRESTORE_IDEMPOTENT", "set-by-test")
	restore()
	assert.Equal(t, "set-by-test", os.Getenv("TEST_SETRESTORE_IDEMPOTENT"))
}

func TestSetWithRestore_SetenvError_ReturnsCleanup(t *testing.T) {
	// Swap setenvFn for one that always fails. The function must still
	// return a usable cleanup closure so callers can defer-restore safely.
	origSetenv := setenvFn
	t.Cleanup(func() { setenvFn = origSetenv })

	setenvErr := errSentinel("setenv refused")
	setenvFn = func(_, _ string) error { return setenvErr }

	cleanup, err := SetWithRestore(map[string]string{
		"TEST_SETRESTORE_FAIL": "value",
	})
	require.Error(t, err)
	require.NotNil(t, cleanup, "cleanup must be returned even on setenv failure so callers can defer-restore")
	// Underlying error preserved.
	assert.ErrorIs(t, err, setenvErr)

	// Cleanup must be safe to invoke even though no value was actually set.
	assert.NotPanics(t, cleanup)
}

// errSentinel is a tiny error type used to verify ErrorIs in setenv-error tests.
type errSentinel string

func (e errSentinel) Error() string { return string(e) }

func TestSetWithRestore_EmptyValue_IsSet(t *testing.T) {
	// Setting a key to "" is distinct from unsetting it.
	unsetForTest(t, "TEST_SETRESTORE_EMPTY")

	restore, err := SetWithRestore(map[string]string{
		"TEST_SETRESTORE_EMPTY": "",
	})
	require.NoError(t, err)

	val, had := os.LookupEnv("TEST_SETRESTORE_EMPTY")
	assert.True(t, had, "setting to empty string must make the key exist")
	assert.Empty(t, val)

	restore()
	_, had = os.LookupEnv("TEST_SETRESTORE_EMPTY")
	assert.False(t, had)
}
