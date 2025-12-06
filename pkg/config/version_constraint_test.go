package config

import (
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/version"
)

func TestValidateVersionConstraint(t *testing.T) {
	tests := []struct {
		name              string
		currentVersion    string
		constraint        schema.VersionConstraint
		envEnforcement    string
		expectError       bool
		expectedSentinel  error
		expectWarningOnly bool
	}{
		// No constraint specified.
		{
			name:           "no constraint specified",
			currentVersion: "2.5.0",
			constraint: schema.VersionConstraint{
				Require: "",
			},
			expectError: false,
		},

		// Constraint satisfied.
		{
			name:           "constraint satisfied minimum",
			currentVersion: "2.5.0",
			constraint: schema.VersionConstraint{
				Require:     ">=1.0.0",
				Enforcement: "fatal",
			},
			expectError: false,
		},
		{
			name:           "constraint satisfied range",
			currentVersion: "2.5.0",
			constraint: schema.VersionConstraint{
				Require:     ">=2.0.0, <3.0.0",
				Enforcement: "fatal",
			},
			expectError: false,
		},

		// Constraint not satisfied with fatal enforcement.
		{
			name:           "fatal unsatisfied too old",
			currentVersion: "1.0.0",
			constraint: schema.VersionConstraint{
				Require:     ">=2.5.0",
				Enforcement: "fatal",
			},
			expectError:      true,
			expectedSentinel: errUtils.ErrVersionConstraint,
		},
		{
			name:           "fatal unsatisfied too new",
			currentVersion: "3.0.0",
			constraint: schema.VersionConstraint{
				Require:     ">=2.0.0, <3.0.0",
				Enforcement: "fatal",
			},
			expectError:      true,
			expectedSentinel: errUtils.ErrVersionConstraint,
		},

		// Default enforcement is fatal.
		{
			name:           "default enforcement is fatal",
			currentVersion: "1.0.0",
			constraint: schema.VersionConstraint{
				Require: ">=99.0.0",
				// No enforcement specified - should default to fatal.
			},
			expectError:      true,
			expectedSentinel: errUtils.ErrVersionConstraint,
		},

		// Warn enforcement - no error returned.
		{
			name:           "warn unsatisfied logs warning no error",
			currentVersion: "1.0.0",
			constraint: schema.VersionConstraint{
				Require:     ">=99.0.0",
				Enforcement: "warn",
			},
			expectError:       false,
			expectWarningOnly: true,
		},

		// Silent enforcement - no validation.
		{
			name:           "silent unsatisfied no output",
			currentVersion: "1.0.0",
			constraint: schema.VersionConstraint{
				Require:     ">=99.0.0",
				Enforcement: "silent",
			},
			expectError: false,
		},

		// Environment variable override to warn.
		{
			name:           "env override fatal to warn",
			currentVersion: "1.0.0",
			constraint: schema.VersionConstraint{
				Require:     ">=99.0.0",
				Enforcement: "fatal",
			},
			envEnforcement:    "warn",
			expectError:       false,
			expectWarningOnly: true,
		},

		// Environment variable override to silent.
		{
			name:           "env override fatal to silent",
			currentVersion: "1.0.0",
			constraint: schema.VersionConstraint{
				Require:     ">=99.0.0",
				Enforcement: "fatal",
			},
			envEnforcement: "silent",
			expectError:    false,
		},

		// Invalid constraint syntax.
		{
			name:           "invalid constraint syntax",
			currentVersion: "2.5.0",
			constraint: schema.VersionConstraint{
				Require:     "invalid>>2.0",
				Enforcement: "fatal",
			},
			expectError:      true,
			expectedSentinel: errUtils.ErrInvalidVersionConstraint,
		},

		// Custom message included in error.
		{
			name:           "custom message included",
			currentVersion: "1.0.0",
			constraint: schema.VersionConstraint{
				Require:     ">=99.0.0",
				Enforcement: "fatal",
				Message:     "Please contact #infrastructure for upgrade assistance.",
			},
			expectError:      true,
			expectedSentinel: errUtils.ErrVersionConstraint,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original version and restore after test.
			originalVersion := version.Version
			defer func() { version.Version = originalVersion }()
			version.Version = tt.currentVersion

			// Set environment variable if specified.
			if tt.envEnforcement != "" {
				t.Setenv("ATMOS_VERSION_ENFORCEMENT", tt.envEnforcement)
			}

			err := validateVersionConstraint(tt.constraint)

			if tt.expectError {
				assert.Error(t, err, "expected error but got none")
				if tt.expectedSentinel != nil {
					assert.True(t, errors.Is(err, tt.expectedSentinel),
						"expected error to wrap %v, got %v", tt.expectedSentinel, err)
				}
				// Verify exit code is set.
				exitCode := errUtils.GetExitCode(err)
				assert.Equal(t, 1, exitCode, "expected exit code 1")
			} else {
				assert.NoError(t, err, "unexpected error: %v", err)
			}
		})
	}
}

func TestValidateVersionConstraint_ExitCode(t *testing.T) {
	// Save original version and restore after test.
	originalVersion := version.Version
	defer func() { version.Version = originalVersion }()
	version.Version = "1.0.0"

	constraint := schema.VersionConstraint{
		Require:     ">=99.0.0",
		Enforcement: "fatal",
	}

	err := validateVersionConstraint(constraint)
	assert.Error(t, err)

	// Verify exit code is extracted correctly.
	exitCode := errUtils.GetExitCode(err)
	assert.Equal(t, 1, exitCode)
}

func TestValidateVersionConstraint_InvalidSyntaxExitCode(t *testing.T) {
	// Save original version and restore after test.
	originalVersion := version.Version
	defer func() { version.Version = originalVersion }()
	version.Version = "2.5.0"

	constraint := schema.VersionConstraint{
		Require:     ">=",
		Enforcement: "fatal",
	}

	err := validateVersionConstraint(constraint)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrInvalidVersionConstraint))

	// Verify exit code is extracted correctly.
	exitCode := errUtils.GetExitCode(err)
	assert.Equal(t, 1, exitCode)
}

func TestValidateVersionConstraint_EnvOverridePrecedence(t *testing.T) {
	// Save original version and restore after test.
	originalVersion := version.Version
	defer func() { version.Version = originalVersion }()
	version.Version = "1.0.0"

	// Test that env var takes precedence over config.
	tests := []struct {
		name          string
		configEnforce string
		envEnforce    string
		expectError   bool
		expectSilent  bool
	}{
		{
			name:          "env silent overrides config fatal",
			configEnforce: "fatal",
			envEnforce:    "silent",
			expectError:   false,
			expectSilent:  true,
		},
		{
			name:          "env warn overrides config fatal",
			configEnforce: "fatal",
			envEnforce:    "warn",
			expectError:   false,
		},
		{
			name:          "env fatal overrides config silent",
			configEnforce: "silent",
			envEnforce:    "fatal",
			expectError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("ATMOS_VERSION_ENFORCEMENT", tt.envEnforce)
			defer os.Unsetenv("ATMOS_VERSION_ENFORCEMENT")

			constraint := schema.VersionConstraint{
				Require:     ">=99.0.0",
				Enforcement: tt.configEnforce,
			}

			err := validateVersionConstraint(constraint)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCheckConfig_WithVersionConstraint(t *testing.T) {
	// Save original version and restore after test.
	originalVersion := version.Version
	defer func() { version.Version = originalVersion }()
	version.Version = "1.0.0"

	// Test that checkConfig propagates version constraint errors.
	atmosConfig := schema.AtmosConfiguration{
		Stacks: schema.Stacks{
			BasePath:      "/some/path",
			IncludedPaths: []string{"/some/path"},
		},
		Version: schema.Version{
			Constraint: schema.VersionConstraint{
				Require:     ">=99.0.0",
				Enforcement: "fatal",
			},
		},
	}

	err := checkConfig(atmosConfig, false)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrVersionConstraint))
}

func TestCheckConfig_WithoutVersionConstraint(t *testing.T) {
	// Test that checkConfig passes when no constraint is specified.
	atmosConfig := schema.AtmosConfiguration{
		Stacks: schema.Stacks{
			BasePath:      "/some/path",
			IncludedPaths: []string{"/some/path"},
		},
		// No version constraint.
	}

	err := checkConfig(atmosConfig, false)
	assert.NoError(t, err)
}
