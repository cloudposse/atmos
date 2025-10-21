package utils

import (
	"testing"
)

func TestGetNextShellLevel(t *testing.T) {
	tests := []struct {
		name          string
		initialEnv    string
		expectedLevel int
		expectErr     bool
	}{
		{
			name:          "No initial shell level",
			initialEnv:    "",
			expectedLevel: 1,
			expectErr:     false,
		},
		{
			name:          "Valid initial shell level",
			initialEnv:    "3",
			expectedLevel: 4,
			expectErr:     false,
		},
		{
			name:          "Exceeding max shell depth",
			initialEnv:    "10",
			expectedLevel: 0,
			expectErr:     true,
		},
		{
			name:          "Invalid shell level format",
			initialEnv:    "invalid",
			expectedLevel: 0,
			expectErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up the environment variable
			if tt.initialEnv != "" {
				t.Setenv("ATMOS_SHLVL", tt.initialEnv)
			}

			// Call the function
			level, err := GetNextShellLevel()

			// Check for errors
			if (err != nil) != tt.expectErr {
				t.Errorf("expected error: %v, got: %v", tt.expectErr, err)
			}

			// Check the returned shell level
			if !tt.expectErr && level != tt.expectedLevel {
				t.Errorf("expected level: %d, got: %d", tt.expectedLevel, level)
			}
		})
	}
}
