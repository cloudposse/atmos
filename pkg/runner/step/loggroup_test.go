package step

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestGroupLabel(t *testing.T) {
	tests := []struct {
		name    string
		stepN   string
		command string
		want    string
	}{
		{name: "name preferred", stepN: "init", command: "terraform init", want: "init"},
		{name: "command fallback when name empty", stepN: "", command: "terraform init", want: "terraform init"},
		{name: "name whitespace falls back to command", stepN: "   ", command: "apply", want: "apply"},
		{name: "both empty", stepN: "", command: "", want: ""},
		{name: "trims name", stepN: "  deploy  ", command: "c", want: "deploy"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, groupLabel(tt.stepN, tt.command))
		})
	}
}

func TestRunGrouped_PassthroughWhenInactive(t *testing.T) {
	// With CI grouping inactive (empty config, ci disabled), RunGrouped must run
	// fn and propagate its result unchanged.
	sentinel := errors.New("boom")
	err := RunGrouped(&schema.AtmosConfiguration{}, "name", "command", func() error { return sentinel })
	require.ErrorIs(t, err, sentinel)

	called := false
	require.NoError(t, RunGrouped(&schema.AtmosConfiguration{}, "name", "command", func() error {
		called = true
		return nil
	}))
	assert.True(t, called)
}
