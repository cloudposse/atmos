package io

import (
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReconcileMasking_FlagDisablesAfterInit reproduces the bug where the global masker is
// created (with masking enabled) before the --mask flag is parsed; ReconcileMasking must then
// pick up `--mask=false` (viper "mask"=false) and disable masking.
func TestReconcileMasking_FlagDisablesAfterInit(t *testing.T) {
	t.Cleanup(func() {
		Reset()
		viper.Reset()
	})

	Reset()
	viper.Reset()

	// Simulate early init: masking enabled (flag not yet parsed → default true).
	viper.Set("mask", true)
	require.NoError(t, Initialize())
	require.True(t, MaskingEnabled(), "masking should start enabled")

	// Simulate flags parsed: --mask=false now visible to viper.
	viper.Set("mask", false)
	ReconcileMasking()

	assert.False(t, MaskingEnabled(), "ReconcileMasking must disable masking when --mask=false")
}

// TestReconcileMasking_EnablesWhenFlagTrue covers the reverse direction.
func TestReconcileMasking_EnablesWhenFlagTrue(t *testing.T) {
	t.Cleanup(func() {
		Reset()
		viper.Reset()
	})

	Reset()
	viper.Reset()

	viper.Set("mask", false)
	require.NoError(t, Initialize())
	require.False(t, MaskingEnabled(), "masking should start disabled")

	viper.Set("mask", true)
	ReconcileMasking()

	assert.True(t, MaskingEnabled(), "ReconcileMasking must enable masking when --mask=true")
}

// TestMaskerSetEnabled exercises the masker's enabled toggle and that Mask honors it.
func TestMaskerSetEnabled(t *testing.T) {
	m := newMasker(&Config{}).(*masker)
	m.RegisterSecret("super-secret-value")

	assert.Contains(t, m.Mask("x super-secret-value y"), m.Replacement())

	m.SetEnabled(false)
	assert.Equal(t, "x super-secret-value y", m.Mask("x super-secret-value y"))

	m.SetEnabled(true)
	assert.Contains(t, m.Mask("x super-secret-value y"), m.Replacement())
}
