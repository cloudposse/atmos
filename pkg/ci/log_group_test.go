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

	m := &logGroupMockProvider{mockProvider: mockProvider{name: "cap", detected: true}}
	Register(m)

	end := StartLogGroup(" hook checkov ")
	require.Len(t, m.started, 1)
	assert.Equal(t, "hook checkov", m.started[0])

	end()
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
