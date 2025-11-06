package terraform

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/flags"
)

// TestCompatibilityAliases_PerCommand tests that each terraform subcommand
// returns the correct set of compatibility aliases.
func TestCompatibilityAliases_PerCommand(t *testing.T) {
	tests := []struct {
		name              string
		subcommand        string
		expectedFlags     []string // Flags that MUST be present
		unexpectedFlags   []string // Flags that MUST NOT be present
		minFlagCount      int      // Minimum number of flags expected
	}{
		{
			name:       "plan - supports planning flags",
			subcommand: "plan",
			expectedFlags: []string{
				"-var", "-var-file", "-out", "-target", "-replace",
				"-destroy", "-refresh-only", "-json", "-no-color",
			},
			unexpectedFlags: []string{"-upgrade", "-backend-config", "-raw", "-force"},
			minFlagCount:    12,
		},
		{
			name:       "apply - supports apply flags",
			subcommand: "apply",
			expectedFlags: []string{
				"-auto-approve", "-var", "-var-file", "-target",
				"-replace", "-json", "-no-color",
			},
			unexpectedFlags: []string{"-out", "-upgrade", "-backend-config", "-raw"},
			minFlagCount:    10,
		},
		{
			name:       "init - supports init flags only",
			subcommand: "init",
			expectedFlags: []string{
				"-upgrade", "-backend-config", "-reconfigure",
				"-migrate-state", "-json", "-no-color",
			},
			unexpectedFlags: []string{"-var", "-var-file", "-out", "-target", "-auto-approve"},
			minFlagCount:    10,
		},
		{
			name:       "output - supports output flags only",
			subcommand: "output",
			expectedFlags: []string{
				"-json", "-raw", "-no-color", "-state",
			},
			unexpectedFlags: []string{"-var", "-var-file", "-out", "-target", "-upgrade"},
			minFlagCount:    4,
		},
		{
			name:       "destroy - supports destroy flags",
			subcommand: "destroy",
			expectedFlags: []string{
				"-auto-approve", "-var", "-var-file", "-target",
				"-json", "-no-color",
			},
			unexpectedFlags: []string{"-out", "-upgrade", "-backend-config", "-raw"},
			minFlagCount:    9,
		},
		{
			name:       "validate - supports minimal flags",
			subcommand: "validate",
			expectedFlags: []string{
				"-json", "-no-color",
			},
			unexpectedFlags: []string{"-var", "-var-file", "-out", "-target", "-upgrade"},
			minFlagCount:    2,
		},
		{
			name:       "import - supports import flags",
			subcommand: "import",
			expectedFlags: []string{
				"-var", "-var-file", "-config", "-input",
				"-state", "-no-color",
			},
			unexpectedFlags: []string{"-out", "-upgrade", "-backend-config", "-auto-approve"},
			minFlagCount:    7,
		},
		{
			name:       "fmt - supports fmt flags",
			subcommand: "fmt",
			expectedFlags: []string{
				"-list", "-write", "-diff", "-check", "-recursive", "-no-color",
			},
			unexpectedFlags: []string{"-var", "-var-file", "-out", "-target"},
			minFlagCount:    6,
		},
		{
			name:       "unknown command - only allows -no-color",
			subcommand: "unknown-command",
			expectedFlags: []string{
				"-no-color",
			},
			unexpectedFlags: []string{"-var", "-var-file", "-out", "-target", "-upgrade"},
			minFlagCount:    1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			aliases := CompatibilityAliases(tt.subcommand)

			// Check minimum flag count.
			assert.GreaterOrEqual(t, len(aliases), tt.minFlagCount,
				"Expected at least %d flags for %s, got %d", tt.minFlagCount, tt.subcommand, len(aliases))

			// Check expected flags are present.
			for _, flag := range tt.expectedFlags {
				assert.Contains(t, aliases, flag,
					"Expected flag %s to be supported by %s", flag, tt.subcommand)

				// Verify all compatibility aliases use AppendToSeparated behavior.
				alias := aliases[flag]
				assert.Equal(t, flags.AppendToSeparated, alias.Behavior,
					"Flag %s should use AppendToSeparated behavior", flag)
				assert.Empty(t, alias.Target,
					"Flag %s should have empty Target (pass-through flag)", flag)
			}

			// Check unexpected flags are NOT present.
			for _, flag := range tt.unexpectedFlags {
				assert.NotContains(t, aliases, flag,
					"Flag %s should NOT be supported by %s", flag, tt.subcommand)
			}
		})
	}
}

// TestCompatibilityAliases_NoCobraShorthands verifies that compatibility aliases
// never include Cobra native shorthands like -s (--stack) or -i (--identity).
func TestCompatibilityAliases_NoCobraShorthands(t *testing.T) {
	forbiddenShorthands := []string{"-s", "-i"}

	subcommands := []string{
		"plan", "apply", "destroy", "init", "output", "import",
		"validate", "show", "state", "fmt", "graph", "taint",
		"console", "providers", "get", "test", "version",
	}

	for _, subcommand := range subcommands {
		t.Run(subcommand, func(t *testing.T) {
			aliases := CompatibilityAliases(subcommand)

			for _, shorthand := range forbiddenShorthands {
				assert.NotContains(t, aliases, shorthand,
					"Compatibility aliases for %s should NOT include Cobra shorthand %s", subcommand, shorthand)
			}
		})
	}
}

// TestCompatibilityAliases_CommonFlags tests that common flags are consistently
// available across commands that should support them.
func TestCompatibilityAliases_CommonFlags(t *testing.T) {
	tests := []struct {
		name              string
		subcommands       []string
		expectedCommonFlags []string
	}{
		{
			name: "var flags in planning commands",
			subcommands: []string{"plan", "apply", "destroy", "import", "console", "test"},
			expectedCommonFlags: []string{"-var", "-var-file"},
		},
		{
			name: "no-color everywhere",
			subcommands: []string{
				"plan", "apply", "destroy", "init", "output", "import",
				"validate", "show", "state", "fmt", "graph",
			},
			expectedCommonFlags: []string{"-no-color"},
		},
		{
			name: "lock flags in stateful commands",
			subcommands: []string{"plan", "apply", "destroy", "import", "state", "taint"},
			expectedCommonFlags: []string{"-lock", "-lock-timeout"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, subcommand := range tt.subcommands {
				aliases := CompatibilityAliases(subcommand)

				for _, flag := range tt.expectedCommonFlags {
					assert.Contains(t, aliases, flag,
						"Expected common flag %s to be supported by %s", flag, subcommand)
				}
			}
		})
	}
}

// TestMergeMaps tests the mergeMaps helper function.
func TestMergeMaps(t *testing.T) {
	tests := []struct {
		name     string
		maps     []map[string]flags.CompatibilityAlias
		expected map[string]flags.CompatibilityAlias
	}{
		{
			name: "merge two maps",
			maps: []map[string]flags.CompatibilityAlias{
				{
					"-var":  {Behavior: flags.AppendToSeparated, Target: ""},
					"-json": {Behavior: flags.AppendToSeparated, Target: ""},
				},
				{
					"-out":    {Behavior: flags.AppendToSeparated, Target: ""},
					"-target": {Behavior: flags.AppendToSeparated, Target: ""},
				},
			},
			expected: map[string]flags.CompatibilityAlias{
				"-var":    {Behavior: flags.AppendToSeparated, Target: ""},
				"-json":   {Behavior: flags.AppendToSeparated, Target: ""},
				"-out":    {Behavior: flags.AppendToSeparated, Target: ""},
				"-target": {Behavior: flags.AppendToSeparated, Target: ""},
			},
		},
		{
			name: "later map overrides earlier",
			maps: []map[string]flags.CompatibilityAlias{
				{
					"-var": {Behavior: flags.AppendToSeparated, Target: ""},
				},
				{
					"-var": {Behavior: flags.MapToAtmosFlag, Target: "--var"},
				},
			},
			expected: map[string]flags.CompatibilityAlias{
				"-var": {Behavior: flags.MapToAtmosFlag, Target: "--var"},
			},
		},
		{
			name:     "empty maps",
			maps:     []map[string]flags.CompatibilityAlias{},
			expected: map[string]flags.CompatibilityAlias{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mergeMaps(tt.maps...)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestCompatibilityAliases_RealWorldScenarios tests realistic usage scenarios.
func TestCompatibilityAliases_RealWorldScenarios(t *testing.T) {
	tests := []struct {
		name            string
		subcommand      string
		flags           []string // Flags user would pass
		shouldBeAllowed bool
	}{
		{
			name:       "plan with vars and out",
			subcommand: "plan",
			flags:      []string{"-var", "-var-file", "-out"},
			shouldBeAllowed: true,
		},
		{
			name:       "init with out flag - should fail",
			subcommand: "init",
			flags:      []string{"-out"},
			shouldBeAllowed: false,
		},
		{
			name:       "output with var flag - should fail",
			subcommand: "output",
			flags:      []string{"-var"},
			shouldBeAllowed: false,
		},
		{
			name:       "apply with auto-approve",
			subcommand: "apply",
			flags:      []string{"-auto-approve"},
			shouldBeAllowed: true,
		},
		{
			name:       "validate with var flag - should fail",
			subcommand: "validate",
			flags:      []string{"-var"},
			shouldBeAllowed: false,
		},
		{
			name:       "import with var and state",
			subcommand: "import",
			flags:      []string{"-var", "-var-file", "-state"},
			shouldBeAllowed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			aliases := CompatibilityAliases(tt.subcommand)

			for _, flag := range tt.flags {
				if tt.shouldBeAllowed {
					assert.Contains(t, aliases, flag,
						"Flag %s should be allowed for %s", flag, tt.subcommand)
				} else {
					assert.NotContains(t, aliases, flag,
						"Flag %s should NOT be allowed for %s", flag, tt.subcommand)
				}
			}
		})
	}
}

// TestCompatibilityAliases_OpenTofuFlags tests that OpenTofu-specific flags are supported.
func TestCompatibilityAliases_OpenTofuFlags(t *testing.T) {
	tests := []struct {
		name        string
		subcommand  string
		flags       []string
		description string
	}{
		{
			name:        "plan with OpenTofu exclude flags",
			subcommand:  "plan",
			flags:       []string{"-exclude", "-exclude-file"},
			description: "OpenTofu exclude flags should be supported in plan",
		},
		{
			name:        "plan with OpenTofu consolidate flags",
			subcommand:  "plan",
			flags:       []string{"-consolidate-warnings", "-consolidate-errors"},
			description: "OpenTofu message consolidation flags should be supported in plan",
		},
		{
			name:        "plan with OpenTofu concise and show-sensitive",
			subcommand:  "plan",
			flags:       []string{"-concise", "-show-sensitive", "-deprecation"},
			description: "OpenTofu output control flags should be supported in plan",
		},
		{
			name:        "plan with OpenTofu target-file",
			subcommand:  "plan",
			flags:       []string{"-target-file"},
			description: "OpenTofu target-file flag should be supported in plan",
		},
		{
			name:        "apply with OpenTofu flags",
			subcommand:  "apply",
			flags:       []string{"-consolidate-warnings", "-consolidate-errors", "-concise", "-show-sensitive", "-deprecation"},
			description: "OpenTofu flags should be supported in apply",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			aliases := CompatibilityAliases(tt.subcommand)

			for _, flag := range tt.flags {
				assert.Contains(t, aliases, flag,
					"OpenTofu flag %s should be supported by %s", flag, tt.subcommand)

				// Verify all OpenTofu flags use AppendToSeparated behavior.
				alias := aliases[flag]
				assert.Equal(t, flags.AppendToSeparated, alias.Behavior,
					"OpenTofu flag %s should use AppendToSeparated behavior", flag)
				assert.Empty(t, alias.Target,
					"OpenTofu flag %s should have empty Target (pass-through flag)", flag)
			}
		})
	}
}

