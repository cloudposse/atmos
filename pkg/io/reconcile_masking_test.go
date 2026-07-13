package io

import (
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
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

// TestReconcileMaskingRegistersLateConfig covers CLIs that initialize global I/O before
// atmos.yaml is loaded. ReconcileMasking must apply final literals, patterns, and replacement.
func TestReconcileMaskingRegistersLateConfig(t *testing.T) {
	t.Cleanup(func() {
		Reset()
		viper.Reset()
	})

	Reset()
	viper.Reset()

	viper.Set("mask", true)
	require.NoError(t, Initialize())
	assert.Equal(t, "prefix demo-key-ABCD1234EFGH5678 suffix", MaskString("prefix demo-key-ABCD1234EFGH5678 suffix"))

	viper.Set("settings", schema.AtmosSettings{
		Terminal: schema.Terminal{
			Mask: schema.MaskSettings{
				Enabled:     true,
				Replacement: "[REDACTED]",
				Patterns:    []string{`demo-key-[A-Za-z0-9]+`},
				Literals:    []string{"super-secret-demo-value"},
			},
		},
	})
	viper.Set("settings.terminal.mask.enabled", true)
	viper.Set("settings.terminal.mask.replacement", "[REDACTED]")
	viper.Set("settings.terminal.mask.patterns", []string{`demo-key-[A-Za-z0-9]+`})
	viper.Set("settings.terminal.mask.literals", []string{"super-secret-demo-value"})

	ReconcileMasking()

	masked := MaskString("prefix demo-key-ABCD1234EFGH5678 and super-secret-demo-value suffix")
	assert.Equal(t, "prefix [REDACTED] and [REDACTED] suffix", masked)
	assert.NotContains(t, masked, "demo-key-ABCD1234EFGH5678")
	assert.NotContains(t, masked, "super-secret-demo-value")
}
