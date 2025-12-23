package ui

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResourceState_String(t *testing.T) {
	tests := []struct {
		state    ResourceState
		expected string
	}{
		{ResourceStatePending, "pending"},
		{ResourceStateRefreshing, "refreshing"},
		{ResourceStateInProgress, "in_progress"},
		{ResourceStateComplete, "complete"},
		{ResourceStateError, "error"},
		{ResourceState(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := tt.state.String()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPhase_String(t *testing.T) {
	tests := []struct {
		phase    Phase
		expected string
	}{
		{PhaseInitializing, "initializing"},
		{PhaseRefreshing, "refreshing"},
		{PhasePlanning, "planning"},
		{PhaseApplying, "applying"},
		{PhaseComplete, "complete"},
		{PhaseError, "error"},
		{Phase(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := tt.phase.String()
			assert.Equal(t, tt.expected, result)
		})
	}
}
