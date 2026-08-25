package schema

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDecodeTerraformFlags_NilOrEmpty asserts that DecodeTerraformFlags returns a
// zero-value TerraformFlags (all fields unset) for nil and empty-map inputs, so
// callers can treat "no flags configured" and "empty flags block" identically.
func TestDecodeTerraformFlags_NilOrEmpty(t *testing.T) {
	got, err := DecodeTerraformFlags(nil)
	require.NoError(t, err)
	assert.Equal(t, TerraformFlags{}, got)

	got, err = DecodeTerraformFlags(map[string]any{})
	require.NoError(t, err)
	assert.Equal(t, TerraformFlags{}, got)
}

// TestDecodeTerraformFlags_FullShape decodes every supported field at once, including
// the pointer-vs-nil semantics for lock/parallelism/refresh.
func TestDecodeTerraformFlags_FullShape(t *testing.T) {
	raw := map[string]any{
		"lock_timeout":     "5m",
		"lock":             false,
		"parallelism":      4,
		"refresh":          false,
		"compact_warnings": true,
	}
	got, err := DecodeTerraformFlags(raw)
	require.NoError(t, err)

	assert.Equal(t, "5m", got.LockTimeout)
	require.NotNil(t, got.Lock)
	assert.False(t, *got.Lock)
	require.NotNil(t, got.Parallelism)
	assert.Equal(t, 4, *got.Parallelism)
	require.NotNil(t, got.Refresh)
	assert.False(t, *got.Refresh)
	assert.True(t, got.CompactWarnings)
}

// TestDecodeTerraformFlags_PartialConfig verifies that unset fields stay at their zero
// value (nil for pointers, "" for LockTimeout, false for CompactWarnings) — the common
// shape for a minimal `flags: { lock_timeout: "30s" }` block.
func TestDecodeTerraformFlags_PartialConfig(t *testing.T) {
	got, err := DecodeTerraformFlags(map[string]any{"lock_timeout": "30s"})
	require.NoError(t, err)

	assert.Equal(t, "30s", got.LockTimeout)
	assert.Nil(t, got.Lock)
	assert.Nil(t, got.Parallelism)
	assert.Nil(t, got.Refresh)
	assert.False(t, got.CompactWarnings)
}

// TestDecodeTerraformFlags_WeaklyTypedNumbers covers the JSON round-trip case (e.g. a
// value decoded through encoding/json produces float64, not int) — WeaklyTypedInput
// must coerce it into Parallelism's *int.
func TestDecodeTerraformFlags_WeaklyTypedNumbers(t *testing.T) {
	got, err := DecodeTerraformFlags(map[string]any{"parallelism": float64(8)})
	require.NoError(t, err)
	require.NotNil(t, got.Parallelism)
	assert.Equal(t, 8, *got.Parallelism)
}

// TestDecodeTerraformFlags_NotAMap covers the type-guard branch — a non-map raw value
// must produce an error joined with ErrInvalidTerraformFlagsConfig.
func TestDecodeTerraformFlags_NotAMap(t *testing.T) {
	_, err := DecodeTerraformFlags("nope")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidTerraformFlagsConfig))
}

// TestDecodeTerraformFlags_UnknownFieldsIgnored documents that DecodeTerraformFlags
// itself ignores unrecognized keys rather than erroring. This is deliberate: it's also
// called from internal/exec's tolerant stack-name-candidate search, which treats any
// error as "wrong candidate, try the next one" — an error here would be silently
// swallowed into a confusing "component not found" message. Typo detection is a separate
// concern; see TestValidateTerraformFlagsKeys.
func TestDecodeTerraformFlags_UnknownFieldsIgnored(t *testing.T) {
	got, err := DecodeTerraformFlags(map[string]any{
		"lock_timeout":           "1m",
		"this_is_not_a_real_key": "ignored",
	})
	require.NoError(t, err)
	assert.Equal(t, "1m", got.LockTimeout)
}

// TestValidateTerraformFlagsKeys_UnknownKeyErrors verifies that a typo like `lock_timout`
// is caught by this separate validation function, since DecodeTerraformFlags itself
// silently ignores it (see TestDecodeTerraformFlags_UnknownFieldsIgnored). Discovered via
// field-testing PR #2992, where such a typo was silently ignored end-to-end with no error
// or warning anywhere in the default `terraform plan` workflow.
func TestValidateTerraformFlagsKeys_UnknownKeyErrors(t *testing.T) {
	err := ValidateTerraformFlagsKeys(map[string]any{
		"lock_timeout": "1m",
		"lock_timout":  "5m",
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidTerraformFlagsConfig))
	assert.Contains(t, err.Error(), "lock_timout")
}

// TestValidateTerraformFlagsKeys_AllKnownKeysValid verifies every real field name passes.
func TestValidateTerraformFlagsKeys_AllKnownKeysValid(t *testing.T) {
	err := ValidateTerraformFlagsKeys(map[string]any{
		"lock_timeout":     "5m",
		"lock":             true,
		"parallelism":      4,
		"refresh":          false,
		"compact_warnings": true,
	})
	require.NoError(t, err)
}

// TestValidateTerraformFlagsKeys_NilOrNonMap verifies this function defers entirely to
// DecodeTerraformFlags for nil/non-map inputs rather than erroring itself.
func TestValidateTerraformFlagsKeys_NilOrNonMap(t *testing.T) {
	assert.NoError(t, ValidateTerraformFlagsKeys(nil))
	assert.NoError(t, ValidateTerraformFlagsKeys("not-a-map"))
}
