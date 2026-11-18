package mocks

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Regression: a one-value Return panicked in ChooseWithDefault, unlike the other methods.
func TestPrompterChooseWithDefaultToleratesMissingErrorValue(t *testing.T) {
	p := &Prompter{}
	p.On("ChooseWithDefault", "pick", "alice", []string{"alice", "bob"}).Return("alice")

	got, err := p.ChooseWithDefault("pick", "alice", []string{"alice", "bob"})
	require.NoError(t, err)
	assert.Equal(t, "alice", got)
}

func TestPrompterChooseWithDefaultReturnsConfiguredError(t *testing.T) {
	errFailed := errors.New("prompt failed")
	p := &Prompter{}
	p.On("ChooseWithDefault", "pick", "", []string(nil)).Return("", errFailed).Once()
	p.On("ChooseWithDefault", "pick", "x", []string(nil)).Return(
		func(string, string, []string) (string, error) { return "from-callback", nil }).Once()

	_, err := p.ChooseWithDefault("pick", "", nil)
	require.ErrorIs(t, err, errFailed)

	// The callback path is unchanged.
	got, err := p.ChooseWithDefault("pick", "x", nil)
	require.NoError(t, err)
	assert.Equal(t, "from-callback", got)
}
