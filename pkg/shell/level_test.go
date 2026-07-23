package shell

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLevel(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		expected int
	}{
		{name: "no env var set", envValue: "", expected: 0},
		{name: "level 1", envValue: "1", expected: 1},
		{name: "level 5", envValue: "5", expected: 5},
		{name: "invalid value returns 0", envValue: "invalid", expected: 0},
		{name: "negative value", envValue: "-1", expected: -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				t.Setenv(LevelEnvVar, tt.envValue)
			} else {
				os.Unsetenv(LevelEnvVar)
			}

			result := Level()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSetLevel(t *testing.T) {
	tests := []struct {
		name  string
		level int
	}{
		{name: "set level 0", level: 0},
		{name: "set level 1", level: 1},
		{name: "set level 10", level: 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := SetLevel(tt.level)
			require.NoError(t, err)

			result := Level()
			assert.Equal(t, tt.level, result)

			// Clean up.
			os.Unsetenv(LevelEnvVar)
		})
	}
}

func TestDecrementLevel(t *testing.T) {
	tests := []struct {
		name     string
		initial  int
		expected int
	}{
		{name: "decrement from 3 to 2", initial: 3, expected: 2},
		{name: "decrement from 1 to 0", initial: 1, expected: 0},
		{name: "decrement from 0 stays 0", initial: 0, expected: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := SetLevel(tt.initial)
			require.NoError(t, err)

			DecrementLevel()

			result := Level()
			assert.Equal(t, tt.expected, result)

			// Clean up.
			os.Unsetenv(LevelEnvVar)
		})
	}
}
