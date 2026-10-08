package ci

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockSuppressingProvider is a grouping-capable provider that also implements
// provider.LogGroupingSuppressor, modeling a detected GitHub provider running
// inside a legacy action whose stdout must stay free of CI metadata.
type mockSuppressingProvider struct {
	*mockGroupingProvider
	suppress bool
}

func (m *mockSuppressingProvider) SuppressLogGrouping() bool { return m.suppress }

// registerSuppressing installs a detected, grouping-capable provider whose
// SuppressLogGrouping() returns suppress, for the duration of t.
func registerSuppressing(t *testing.T, suppress bool) *mockSuppressingProvider {
	t.Helper()
	restore := SwapRegistryForTest()
	t.Cleanup(restore)
	t.Setenv(logGroupSentinelEnvVar, "") // Deterministic regardless of how the test binary was launched.

	p := &mockSuppressingProvider{
		mockGroupingProvider: &mockGroupingProvider{mockProvider: &mockProvider{name: "mock-suppressing", detected: true}},
		suppress:             suppress,
	}
	Register(p)
	return p
}

// When the detected provider suppresses grouping, ci.Group still runs fn but
// emits no group markers, and grouping is reported unavailable.
func TestGroup_SuppressedProviderEmitsNoMarkers(t *testing.T) {
	p := registerSuppressing(t, true)

	called := false
	err := Group(modeConfig(""), DimensionStep, "terraform init", func() error {
		called = true
		return nil
	})
	require.NoError(t, err)

	assert.True(t, called, "fn must still run when grouping is suppressed")
	assert.Empty(t, p.started, "no ::group:: marker may be emitted when suppressed")
	assert.Zero(t, p.ended, "no ::endgroup:: marker may be emitted when suppressed")

	assert.False(t, GroupingEnabled(modeConfig("")), "grouping must be reported unavailable when suppressed")
	assert.False(t, ShouldPropagateLogGroupSentinel(modeConfig(""), DimensionStep),
		"the sentinel must not propagate when grouping is suppressed")
}

// ci.StartLogGroup is a no-op (no markers, no-op closer) when the provider
// suppresses grouping.
func TestStartLogGroup_SuppressedProviderIsNoOp(t *testing.T) {
	p := registerSuppressing(t, true)

	end := StartLogGroup("hook policy:check")
	require.NotNil(t, end)
	end()

	assert.Empty(t, p.started, "StartLogGroup must emit no ::group:: marker when suppressed")
	assert.Zero(t, p.ended)
}

// Negative control: the same provider with suppression off groups normally, so
// the tests above prove suppression (not some other condition) disables it.
func TestGroup_UnsuppressedProviderStillGroups(t *testing.T) {
	p := registerSuppressing(t, false)

	err := Group(modeConfig(""), DimensionStep, "terraform init", func() error { return nil })
	require.NoError(t, err)

	assert.Equal(t, []string{"terraform init"}, p.started)
	assert.Equal(t, 1, p.ended)
	assert.True(t, GroupingEnabled(modeConfig("")))
}
