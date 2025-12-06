package version

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateConstraint(t *testing.T) {
	tests := []struct {
		name           string
		currentVersion string
		constraint     string
		expectPass     bool
		expectError    bool
	}{
		// Empty constraint always passes.
		{
			name:           "empty constraint always passes",
			currentVersion: "1.0.0",
			constraint:     "",
			expectPass:     true,
			expectError:    false,
		},

		// Minimum version constraints (>=).
		{
			name:           "minimum version satisfied",
			currentVersion: "2.6.0",
			constraint:     ">=2.5.0",
			expectPass:     true,
			expectError:    false,
		},
		{
			name:           "minimum version exact match",
			currentVersion: "2.5.0",
			constraint:     ">=2.5.0",
			expectPass:     true,
			expectError:    false,
		},
		{
			name:           "minimum version not satisfied",
			currentVersion: "2.4.0",
			constraint:     ">=2.5.0",
			expectPass:     false,
			expectError:    false,
		},

		// Maximum version constraints (<).
		{
			name:           "maximum version satisfied",
			currentVersion: "2.9.0",
			constraint:     "<3.0.0",
			expectPass:     true,
			expectError:    false,
		},
		{
			name:           "maximum version not satisfied boundary",
			currentVersion: "3.0.0",
			constraint:     "<3.0.0",
			expectPass:     false,
			expectError:    false,
		},
		{
			name:           "maximum version not satisfied above",
			currentVersion: "3.1.0",
			constraint:     "<3.0.0",
			expectPass:     false,
			expectError:    false,
		},

		// Range constraints (>=, <).
		{
			name:           "range satisfied middle",
			currentVersion: "2.7.0",
			constraint:     ">=2.5.0, <3.0.0",
			expectPass:     true,
			expectError:    false,
		},
		{
			name:           "range satisfied lower bound",
			currentVersion: "2.5.0",
			constraint:     ">=2.5.0, <3.0.0",
			expectPass:     true,
			expectError:    false,
		},
		{
			name:           "range not satisfied too old",
			currentVersion: "2.4.0",
			constraint:     ">=2.5.0, <3.0.0",
			expectPass:     false,
			expectError:    false,
		},
		{
			name:           "range not satisfied too new",
			currentVersion: "3.1.0",
			constraint:     ">=2.5.0, <3.0.0",
			expectPass:     false,
			expectError:    false,
		},

		// Pessimistic constraints (~>).
		// Note: hashicorp/go-version uses:
		//   ~>2.5   means >=2.5.0, <3.0.0 (allows any 2.x >= 2.5)
		//   ~>2.5.3 means >=2.5.3, <2.6.0 (allows any 2.5.x >= 2.5.3)
		{
			name:           "pessimistic two-segment satisfied same minor",
			currentVersion: "2.5.3",
			constraint:     "~>2.5",
			expectPass:     true,
			expectError:    false,
		},
		{
			name:           "pessimistic two-segment satisfied next minor",
			currentVersion: "2.9.0",
			constraint:     "~>2.5",
			expectPass:     true,
			expectError:    false,
		},
		{
			name:           "pessimistic two-segment not satisfied next major",
			currentVersion: "3.0.0",
			constraint:     "~>2.5",
			expectPass:     false,
			expectError:    false,
		},
		{
			name:           "pessimistic three-segment satisfied",
			currentVersion: "2.5.5",
			constraint:     "~>2.5.3",
			expectPass:     true,
			expectError:    false,
		},
		{
			name:           "pessimistic three-segment not satisfied next minor",
			currentVersion: "2.6.0",
			constraint:     "~>2.5.3",
			expectPass:     false,
			expectError:    false,
		},

		// Exact version match.
		{
			name:           "exact version match",
			currentVersion: "2.5.0",
			constraint:     "2.5.0",
			expectPass:     true,
			expectError:    false,
		},
		{
			name:           "exact version mismatch patch",
			currentVersion: "2.5.1",
			constraint:     "2.5.0",
			expectPass:     false,
			expectError:    false,
		},
		{
			name:           "exact version mismatch minor",
			currentVersion: "2.6.0",
			constraint:     "2.5.0",
			expectPass:     false,
			expectError:    false,
		},

		// Exclusion constraints (!=).
		{
			name:           "exclusion satisfied",
			currentVersion: "2.6.0",
			constraint:     "!=2.7.0",
			expectPass:     true,
			expectError:    false,
		},
		{
			name:           "exclusion not satisfied",
			currentVersion: "2.7.0",
			constraint:     "!=2.7.0",
			expectPass:     false,
			expectError:    false,
		},

		// Complex constraints.
		{
			name:           "complex constraint satisfied",
			currentVersion: "2.8.0",
			constraint:     ">=2.5.0, !=2.7.0, <3.0.0",
			expectPass:     true,
			expectError:    false,
		},
		{
			name:           "complex constraint not satisfied excluded version",
			currentVersion: "2.7.0",
			constraint:     ">=2.5.0, !=2.7.0, <3.0.0",
			expectPass:     false,
			expectError:    false,
		},
		{
			name:           "complex constraint not satisfied too old",
			currentVersion: "2.4.0",
			constraint:     ">=2.5.0, !=2.7.0, <3.0.0",
			expectPass:     false,
			expectError:    false,
		},

		// Invalid constraint syntax.
		{
			name:           "invalid constraint syntax",
			currentVersion: "2.5.0",
			constraint:     "invalid>>2.0",
			expectPass:     false,
			expectError:    true,
		},
		{
			name:           "malformed constraint",
			currentVersion: "2.5.0",
			constraint:     ">=",
			expectPass:     false,
			expectError:    true,
		},

		// Invalid current version.
		{
			name:           "invalid current version",
			currentVersion: "not-semver",
			constraint:     ">=2.5.0",
			expectPass:     false,
			expectError:    true,
		},

		// Edge cases.
		{
			name:           "prerelease version",
			currentVersion: "2.5.0-beta.1",
			constraint:     ">=2.5.0",
			expectPass:     false,
			expectError:    false,
		},
		{
			name:           "version with build metadata",
			currentVersion: "2.5.0+build.123",
			constraint:     ">=2.5.0",
			expectPass:     true,
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original version and restore after test.
			originalVersion := Version
			defer func() { Version = originalVersion }()

			Version = tt.currentVersion

			pass, err := ValidateConstraint(tt.constraint)

			if tt.expectError {
				assert.Error(t, err, "expected error but got none")
			} else {
				assert.NoError(t, err, "unexpected error: %v", err)
			}

			assert.Equal(t, tt.expectPass, pass, "expected pass=%v, got pass=%v", tt.expectPass, pass)
		})
	}
}

func TestValidateConstraint_EmptyVersion(t *testing.T) {
	// Test with empty version string (invalid).
	originalVersion := Version
	defer func() { Version = originalVersion }()

	Version = ""

	pass, err := ValidateConstraint(">=1.0.0")
	assert.Error(t, err, "expected error for empty version")
	assert.False(t, pass, "expected pass=false for empty version")
}

func TestValidateConstraint_ConcurrentAccess(t *testing.T) {
	// Ensure the function is safe for concurrent calls.
	originalVersion := Version
	defer func() { Version = originalVersion }()

	Version = "2.5.0"

	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			pass, err := ValidateConstraint(">=2.0.0, <3.0.0")
			assert.NoError(t, err)
			assert.True(t, pass)
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}
