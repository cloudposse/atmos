package cmd

import (
	"os"
	"testing"

	log "github.com/charmbracelet/log"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/tools/gotcha/pkg/constants"
)

func TestCreateStreamCommand(t *testing.T) {
	logger := log.New(os.Stderr)
	cmd := createStreamCommand(logger)

	// Verify command properties
	assert.NotNil(t, cmd)
	assert.Equal(t, "stream [path]", cmd.Use)
	assert.Contains(t, cmd.Short, "Stream test results")
	assert.Contains(t, cmd.Long, "Execute go test")
	assert.Contains(t, cmd.Example, "gotcha stream")

	// Verify command settings
	assert.True(t, cmd.SilenceUsage)
	assert.True(t, cmd.SilenceErrors)
	// Note: Can't compare function pointers directly for Args
	assert.NotNil(t, cmd.Args)

	// Verify RunE function is set
	assert.NotNil(t, cmd.RunE)
}

func TestAddTestExecutionFlags(t *testing.T) {
	cmd := &cobra.Command{}
	addTestExecutionFlags(cmd)

	// Test that all expected flags are present
	tests := []struct {
		flag      string
		shorthand string
		defValue  string
		usage     string
	}{
		{"run", "r", "", "Run only tests matching regular expression"},
		{"timeout", "t", "10m", "Test timeout"},
		{"short", "s", "false", "Run smaller tests"},
		{"race", "", "false", "Enable race detector"},
		{"count", "", "1", "Run tests this many times"},
		{"shuffle", "", "false", "Shuffle test order"},
		{"verbose", "v", "false", "Verbose output"},
	}

	for _, tt := range tests {
		t.Run(tt.flag, func(t *testing.T) {
			flag := cmd.Flags().Lookup(tt.flag)
			require.NotNil(t, flag, "Flag %s should exist", tt.flag)
			assert.Equal(t, tt.defValue, flag.DefValue, "Default value mismatch for flag %s", tt.flag)

			if tt.shorthand != "" {
				short := cmd.Flags().ShorthandLookup(tt.shorthand)
				assert.NotNil(t, short, "Shorthand %s should exist for flag %s", tt.shorthand, tt.flag)
				assert.Equal(t, flag, short, "Shorthand should reference the same flag")
			}
		})
	}
}

func TestAddCoverageFlags(t *testing.T) {
	cmd := &cobra.Command{}
	addCoverageFlags(cmd)

	tests := []struct {
		flag     string
		defValue string
	}{
		{"cover", "false"},
		{"coverprofile", ""},
		{"coverpkg", ""},
	}

	for _, tt := range tests {
		t.Run(tt.flag, func(t *testing.T) {
			flag := cmd.Flags().Lookup(tt.flag)
			require.NotNil(t, flag, "Flag %s should exist", tt.flag)
			assert.Equal(t, tt.defValue, flag.DefValue, "Default value mismatch for flag %s", tt.flag)
		})
	}
}

func TestAddPackageSelectionFlags(t *testing.T) {
	cmd := &cobra.Command{}
	addPackageSelectionFlags(cmd)

	tests := []struct {
		flag     string
		defValue string
		usage    string
	}{
		{"include", "", "Include packages matching regex patterns"},
		{"exclude", "", "Exclude packages matching regex patterns"},
	}

	for _, tt := range tests {
		t.Run(tt.flag, func(t *testing.T) {
			flag := cmd.Flags().Lookup(tt.flag)
			require.NotNil(t, flag, "Flag %s should exist", tt.flag)
			assert.Equal(t, tt.defValue, flag.DefValue, "Default value mismatch for flag %s", tt.flag)
			assert.Contains(t, flag.Usage, tt.usage, "Usage description mismatch for flag %s", tt.flag)
		})
	}
}

func TestAddOutputControlFlags(t *testing.T) {
	cmd := &cobra.Command{}
	addOutputControlFlags(cmd)

	tests := []struct {
		flag      string
		shorthand string
		defValue  string
	}{
		{"show", "", "all"},
		{"format", "f", constants.FormatTerminal},
		{"output", "o", ""},
		{"alert", "", "false"},
		{"verbosity", "", "standard"},
	}

	for _, tt := range tests {
		t.Run(tt.flag, func(t *testing.T) {
			flag := cmd.Flags().Lookup(tt.flag)
			require.NotNil(t, flag, "Flag %s should exist", tt.flag)
			assert.Equal(t, tt.defValue, flag.DefValue, "Default value mismatch for flag %s", tt.flag)

			if tt.shorthand != "" {
				short := cmd.Flags().ShorthandLookup(tt.shorthand)
				assert.NotNil(t, short, "Shorthand %s should exist for flag %s", tt.shorthand, tt.flag)
			}
		})
	}
}

func TestAddCIIntegrationFlags(t *testing.T) {
	cmd := &cobra.Command{}
	addCIIntegrationFlags(cmd)

	tests := []struct {
		flag     string
		defValue string
	}{
		{"ci", "false"},
		{"post-comment", ""},
		{"github-token", ""},
		{"comment-uuid", ""},
		{"exclude-mocks", "true"},
	}

	for _, tt := range tests {
		t.Run(tt.flag, func(t *testing.T) {
			flag := cmd.Flags().Lookup(tt.flag)
			require.NotNil(t, flag, "Flag %s should exist", tt.flag)
			assert.Equal(t, tt.defValue, flag.DefValue, "Default value mismatch for flag %s", tt.flag)
		})
	}
}

func TestNewStreamCmd(t *testing.T) {
	logger := log.New(os.Stderr)
	cmd := newStreamCmd(logger)

	// Verify the command is properly constructed
	assert.NotNil(t, cmd)
	assert.Equal(t, "stream [path]", cmd.Use)

	// Verify all flag groups were added
	flagGroups := [][]string{
		// Test execution flags
		{"run", "timeout", "short", "race", "count", "shuffle", "verbose"},
		// Coverage flags
		{"cover", "coverprofile", "coverpkg"},
		// Package selection flags
		{"include", "exclude"},
		// Output control flags
		{"show", "format", "output", "alert", "verbosity"},
		// CI integration flags
		{"ci", "post-comment", "github-token", "comment-uuid", "exclude-mocks"},
	}

	for _, group := range flagGroups {
		for _, flagName := range group {
			flag := cmd.Flags().Lookup(flagName)
			assert.NotNil(t, flag, "Flag %s should be added to stream command", flagName)
		}
	}
}

func TestStreamCommandIntegration(t *testing.T) {
	// Test that the stream command integrates properly with the rest of the system

	// Test that we can parse various flag combinations without errors
	tests := []struct {
		name string
		args []string
	}{
		{"basic usage", []string{}},
		{"with path", []string{"./pkg/..."}},
		{"with run flag", []string{"--run=TestConfig"}},
		{"with short timeout", []string{"--timeout=1m", "-s"}},
		{"with coverage", []string{"--cover", "--coverprofile=coverage.out"}},
		{"with filters", []string{"--include=api", "--exclude=mock"}},
		{"with output format", []string{"--format=json", "--output=results.json"}},
		{"with CI mode", []string{"--ci", "--post-comment=adaptive"}},
		{"complex combination", []string{
			"./internal/...",
			"--run=TestIntegration",
			"--cover",
			"--format=markdown",
			"--ci",
			"--post-comment=on-failure",
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a new command for each test to avoid flag conflicts
			testLogger := log.New(os.Stderr)
			cmd := newStreamCmd(testLogger)
			cmd.SetArgs(tt.args)

			// Parse flags only (don't execute)
			err := cmd.ParseFlags(tt.args)
			assert.NoError(t, err, "Should parse flags without error")
		})
	}
}

func TestStreamCommandShorthands(t *testing.T) {
	logger := log.New(os.Stderr)
	cmd := newStreamCmd(logger)

	// Test that shorthands work properly
	shorthands := map[string]string{
		"r": "run",
		"t": "timeout",
		"s": "short",
		"v": "verbose",
		"f": "format",
		"o": "output",
	}

	for short, long := range shorthands {
		shortFlag := cmd.Flags().ShorthandLookup(short)
		longFlag := cmd.Flags().Lookup(long)

		assert.NotNil(t, shortFlag, "Shorthand -%s should exist", short)
		assert.NotNil(t, longFlag, "Long flag --%s should exist", long)
		assert.Equal(t, longFlag, shortFlag, "Shorthand -%s should reference --%s", short, long)
	}
}
