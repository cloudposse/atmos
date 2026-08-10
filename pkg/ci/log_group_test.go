package ci

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type logGroupMockProvider struct {
	mockProvider
	started []string
	ended   int
}

func (m *logGroupMockProvider) StartLogGroup(title string) error {
	m.started = append(m.started, title)
	return nil
}

func (m *logGroupMockProvider) EndLogGroup() error {
	m.ended++
	return nil
}

func TestStartLogGroup_DispatchesToCapableProvider(t *testing.T) {
	restore := SwapRegistryForTest()
	defer restore()
	// See registerGrouping's comment in loggroup_test.go: this test binary can
	// itself run as a child of a CI-grouped step, in which case the sentinel
	// is already set and StartLogGroup would silently no-op below.
	t.Setenv(logGroupSentinelEnvVar, "")

	m := &logGroupMockProvider{mockProvider: mockProvider{name: "cap", detected: true}}
	Register(m)

	end := StartLogGroup(" hook checkov ")
	// Cleanup, not a bare defer: guarantees end() (and its logGroupDepth
	// decrement) runs even if the require.Len below fails and aborts the test
	// via t.FailNow() before reaching the original inline end() call further
	// down. A leaked logGroupDepth is process-wide -- it silently no-ops every
	// later test's StartLogGroup/Group call for the rest of this test binary.
	t.Cleanup(func() {
		end()
		assert.Equal(t, 1, m.ended)
	})

	require.Len(t, m.started, 1)
	assert.Equal(t, "hook checkov", m.started[0])
}

func TestStartLogGroup_NoopsWhenGroupAlreadyActive(t *testing.T) {
	restore := SwapRegistryForTest()
	defer restore()
	// The outer Group call below must actually open a group for this test to
	// exercise the nested no-op path; an inherited sentinel would make it
	// no-op instead (see registerGrouping's comment in loggroup_test.go).
	t.Setenv(logGroupSentinelEnvVar, "")

	m := &logGroupMockProvider{mockProvider: mockProvider{name: "cap", detected: true}}
	Register(m)

	err := Group(modeConfig(GroupModeAuto), DimensionPhase, "outer", func() error {
		StartLogGroup("inner")()
		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, []string{"outer"}, m.started)
	assert.Equal(t, 1, m.ended)
}

func TestStartLogGroup_NoopCases(t *testing.T) {
	t.Run("empty title", func(t *testing.T) {
		restore := SwapRegistryForTest()
		defer restore()

		m := &logGroupMockProvider{mockProvider: mockProvider{name: "cap", detected: true}}
		Register(m)
		StartLogGroup("   ")()
		assert.Empty(t, m.started)
		assert.Zero(t, m.ended)
	})

	t.Run("no provider", func(t *testing.T) {
		restore := SwapRegistryForTest()
		defer restore()
		assert.NotPanics(t, func() { StartLogGroup("hook")() })
	})

	t.Run("provider lacks capability", func(t *testing.T) {
		restore := SwapRegistryForTest()
		defer restore()

		Register(&mockProvider{name: "plain", detected: true})
		assert.NotPanics(t, func() { StartLogGroup("hook")() })
	})
}
