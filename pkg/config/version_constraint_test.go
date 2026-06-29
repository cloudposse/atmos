package config

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/version"
)

func captureVersionConstraintLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	t.Setenv("NO_COLOR", "1")

	originalLogger := log.Default()
	buffer := &bytes.Buffer{}
	testLogger := log.New()
	testLogger.SetOutput(buffer)
	testLogger.SetReportTimestamp(false)
	log.SetDefault(testLogger)
	t.Cleanup(func() { log.SetDefault(originalLogger) })

	return buffer
}

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

func TestValidateVersionConstraint_ExplicitOverrideWarnings(t *testing.T) {
	tests := []struct {
		name             string
		currentVersion   string
		constraint       schema.VersionConstraint
		envKey           string
		envValue         string
		expectErr        error
		expectWarning    bool
		warningContains  []string
		warningOmits     []string
		configVersionUse string
		osArgs           []string
	}{
		{
			name:           "internal explicit override bypasses invalid current version",
			currentVersion: "test",
			constraint: schema.VersionConstraint{
				Require:     ">=1.216.0",
				Enforcement: "fatal",
			},
			envKey:        version.VersionUseEnvVar,
			envValue:      "ref:main",
			expectWarning: true,
			warningContains: []string{
				"Atmos version constraint could not be evaluated",
				`required=">=1.216.0"`,
				"current=test",
				"override=ref:main",
				"bypassing version constraint enforcement because an explicit version override was requested",
				"invalid current version",
			},
		},
		{
			name:           "public explicit override bypasses unsatisfied constraint",
			currentVersion: "1.0.0",
			constraint: schema.VersionConstraint{
				Require:     ">=99.0.0",
				Enforcement: "fatal",
			},
			envKey:        version.UseVersionEnvVar,
			envValue:      "1.100.0",
			expectWarning: true,
			warningContains: []string{
				"Atmos version constraint not satisfied",
				`required=">=99.0.0"`,
				"current=1.0.0",
				"override=1.100.0",
				"bypassing version constraint enforcement because an explicit version override was requested",
			},
		},
		{
			name:           "raw use-version flag bypasses unsatisfied constraint before reexec env is set",
			currentVersion: "1.0.0",
			constraint: schema.VersionConstraint{
				Require:     ">=99.0.0",
				Enforcement: "fatal",
			},
			osArgs:        []string{"atmos", "list", "stacks", "--use-version=ref:main"},
			expectWarning: true,
			warningContains: []string{
				"Atmos version constraint not satisfied",
				"current=1.0.0",
				"override=ref:main",
			},
		},
		{
			name:           "legacy explicit override bypasses unsatisfied constraint",
			currentVersion: "1.0.0",
			constraint: schema.VersionConstraint{
				Require:     ">=99.0.0",
				Enforcement: "fatal",
			},
			envKey:        version.VersionEnvVar,
			envValue:      "1.100.0",
			expectWarning: true,
			warningContains: []string{
				"Atmos version constraint not satisfied",
				"override=1.100.0",
			},
		},
		{
			name:           "explicit override satisfied constraint does not warn",
			currentVersion: "2.0.0",
			constraint: schema.VersionConstraint{
				Require:     ">=1.0.0",
				Enforcement: "fatal",
			},
			envKey:        version.UseVersionEnvVar,
			envValue:      "2.0.0",
			expectWarning: false,
			warningOmits: []string{
				"Atmos version constraint",
				"bypassing version constraint enforcement",
			},
		},
		{
			name:           "explicit override keeps invalid constraint syntax fatal",
			currentVersion: "test",
			constraint: schema.VersionConstraint{
				Require:     ">=",
				Enforcement: "fatal",
			},
			envKey:        version.VersionUseEnvVar,
			envValue:      "ref:main",
			expectErr:     errUtils.ErrInvalidVersionConstraint,
			expectWarning: false,
			warningOmits: []string{
				"bypassing version constraint enforcement",
			},
		},
		{
			name:           "explicit override respects silent enforcement",
			currentVersion: "test",
			constraint: schema.VersionConstraint{
				Require:     ">=99.0.0",
				Enforcement: "silent",
			},
			envKey:        version.VersionUseEnvVar,
			envValue:      "ref:main",
			expectWarning: false,
			warningOmits: []string{
				"Atmos version constraint",
				"bypassing version constraint enforcement",
			},
		},
		{
			name:           "config version use is not treated as explicit override",
			currentVersion: "1.0.0",
			constraint: schema.VersionConstraint{
				Require:     ">=99.0.0",
				Enforcement: "fatal",
			},
			configVersionUse: "ref:main",
			expectErr:        errUtils.ErrVersionConstraint,
			expectWarning:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalVersion := version.Version
			defer func() { version.Version = originalVersion }()
			version.Version = tt.currentVersion

			if tt.envKey != "" {
				t.Setenv(tt.envKey, tt.envValue)
			}
			if tt.osArgs != nil {
				originalArgs := os.Args
				os.Args = tt.osArgs
				t.Cleanup(func() { os.Args = originalArgs })
			}

			logs := captureVersionConstraintLogs(t)

			var err error
			if tt.configVersionUse != "" {
				err = checkConfig(schema.AtmosConfiguration{
					Version: schema.Version{
						Use:        tt.configVersionUse,
						Constraint: tt.constraint,
					},
				}, false)
			} else {
				err = validateVersionConstraint(tt.constraint)
			}

			if tt.expectErr != nil {
				assert.Error(t, err)
				assert.True(t, errors.Is(err, tt.expectErr), "expected error to wrap %v, got %v", tt.expectErr, err)
			} else {
				assert.NoError(t, err)
			}

			output := logs.String()
			if tt.expectWarning {
				assert.NotEmpty(t, strings.TrimSpace(output), "expected warning output")
			} else {
				assert.Empty(t, strings.TrimSpace(output), "expected no warning output")
			}
			for _, want := range tt.warningContains {
				assert.Contains(t, output, want)
			}
			for _, omitted := range tt.warningOmits {
				assert.NotContains(t, output, omitted)
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
