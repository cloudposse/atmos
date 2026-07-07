package cmd

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeHelpTopicArgs(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		expectedArgs   []string
		expectedTopic  helpTopic
		expectedRaw    string
		expectedValid  bool
		expectedSet    bool
		expectedChange bool
	}{
		{
			name:           "usage topic",
			args:           []string{"terraform", "plan", "--help=usage"},
			expectedArgs:   []string{"terraform", "plan", "--help"},
			expectedTopic:  helpTopicUsage,
			expectedRaw:    "usage",
			expectedValid:  true,
			expectedSet:    true,
			expectedChange: true,
		},
		{
			name:           "flags topic",
			args:           []string{"terraform", "plan", "--help=flags"},
			expectedArgs:   []string{"terraform", "plan", "--help"},
			expectedTopic:  helpTopicFlags,
			expectedRaw:    "flags",
			expectedValid:  true,
			expectedSet:    true,
			expectedChange: true,
		},
		{
			name:           "all topic",
			args:           []string{"terraform", "plan", "--help=all"},
			expectedArgs:   []string{"terraform", "plan", "--help"},
			expectedTopic:  helpTopicAll,
			expectedRaw:    "all",
			expectedValid:  true,
			expectedSet:    true,
			expectedChange: true,
		},
		{
			name:           "uppercase topic",
			args:           []string{"terraform", "plan", "--help=USAGE"},
			expectedArgs:   []string{"terraform", "plan", "--help"},
			expectedTopic:  helpTopicUsage,
			expectedRaw:    "usage",
			expectedValid:  true,
			expectedSet:    true,
			expectedChange: true,
		},
		{
			name:           "unknown topic",
			args:           []string{"terraform", "plan", "--help=advanced"},
			expectedArgs:   []string{"terraform", "plan", "--help"},
			expectedTopic:  helpTopic("advanced"),
			expectedRaw:    "advanced",
			expectedValid:  false,
			expectedSet:    true,
			expectedChange: true,
		},
		{
			name:           "bare help unchanged",
			args:           []string{"terraform", "plan", "--help"},
			expectedArgs:   []string{"terraform", "plan", "--help"},
			expectedValid:  true,
			expectedChange: false,
		},
		{
			name:           "short help unchanged",
			args:           []string{"terraform", "plan", "-h"},
			expectedArgs:   []string{"terraform", "plan", "-h"},
			expectedValid:  true,
			expectedChange: false,
		},
		{
			name:           "bool help unchanged",
			args:           []string{"terraform", "plan", "--help=true"},
			expectedArgs:   []string{"terraform", "plan", "--help=true"},
			expectedValid:  true,
			expectedChange: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args, request, changed := normalizeHelpTopicArgs(tt.args)
			assert.Equal(t, tt.expectedArgs, args)
			assert.Equal(t, tt.expectedChange, changed)
			assert.Equal(t, tt.expectedTopic, request.topic)
			assert.Equal(t, tt.expectedRaw, request.raw)
			assert.Equal(t, tt.expectedSet, request.explicit)
			assert.Equal(t, tt.expectedValid, request.valid)
		})
	}
}

func TestTopicHelpRendering_DefaultShowsLocalFlagsOnly(t *testing.T) {
	output := renderTopicHelpForTest(t, helpTopicRequest{valid: true}, testHelpCommand(t))

	assert.Contains(t, output, "FLAGS")
	assert.Contains(t, output, "--local")
	assert.NotContains(t, output, "GLOBAL FLAGS")
	assert.NotContains(t, output, "--global")
	assert.Contains(t, output, "--help=usage")
	assert.Contains(t, output, "--help=all")
}

func TestTopicHelpRendering_UsageShowsOnlyUsageAndExamples(t *testing.T) {
	output := renderTopicHelpForTest(t, helpTopicRequest{topic: helpTopicUsage, explicit: true, valid: true}, testHelpCommand(t))

	assert.Contains(t, output, "USAGE")
	assert.Contains(t, output, "EXAMPLES")
	assert.Contains(t, output, "atmos test child --local")
	assert.NotContains(t, output, "FLAGS")
	assert.NotContains(t, output, "--global")
}

func TestTopicHelpRendering_FlagsShowsLocalFlagsOnly(t *testing.T) {
	output := renderTopicHelpForTest(t, helpTopicRequest{topic: helpTopicFlags, explicit: true, valid: true}, testHelpCommand(t))

	assert.Contains(t, output, "FLAGS")
	assert.Contains(t, output, "--local")
	assert.NotContains(t, output, "GLOBAL FLAGS")
	assert.NotContains(t, output, "--global")
	assert.NotContains(t, output, "USAGE")
	assert.NotContains(t, output, "EXAMPLES")
}

func TestTopicHelpRendering_AllIncludesGlobalFlags(t *testing.T) {
	output := renderTopicHelpForTest(t, helpTopicRequest{topic: helpTopicAll, explicit: true, valid: true}, testHelpCommand(t))

	assert.Contains(t, output, "FLAGS")
	assert.Contains(t, output, "--local")
	assert.Contains(t, output, "GLOBAL FLAGS")
	assert.Contains(t, output, "--global")
}

func TestTopicHelpRendering_RootDefaultFiltersPersistentGlobals(t *testing.T) {
	root := &cobra.Command{
		Use:   "atmos",
		Short: "root command",
	}
	root.PersistentFlags().Bool("global", false, "global flag")
	root.Flags().Bool("version", false, "version flag")

	output := renderTopicHelpForTest(t, helpTopicRequest{valid: true}, root)

	assert.Contains(t, output, "FLAGS")
	assert.Contains(t, output, "--version")
	assert.NotContains(t, output, "--global")
}

func TestCommandSpecificFlagSetNilCommand(t *testing.T) {
	assert.Nil(t, commandSpecificFlagSet(nil))
}

func TestPrintLocalFlagsOnlySkipsCommandsWithoutLocalFlags(t *testing.T) {
	var buf bytes.Buffer
	renderer := lipgloss.NewRenderer(&buf)
	styles := createHelpStyles(renderer)
	cmd := &cobra.Command{Use: "empty"}

	printLocalFlagsOnly(&buf, cmd, nil, &styles)

	assert.Empty(t, buf.String())
}

func TestPreprocessHelpTopicArgs(t *testing.T) {
	originalArgs := os.Args
	originalTopic := currentHelpTopic
	t.Cleanup(func() {
		os.Args = originalArgs
		currentHelpTopic = originalTopic
		RootCmd.SetArgs(nil)
	})

	tests := []struct {
		name         string
		args         []string
		expectedArgs []string
		expected     helpTopicRequest
	}{
		{
			name:         "no arguments",
			args:         []string{"atmos"},
			expectedArgs: []string{"atmos"},
			expected:     helpTopicRequest{valid: true},
		},
		{
			name:         "bare help unchanged",
			args:         []string{"atmos", "--help"},
			expectedArgs: []string{"atmos", "--help"},
			expected:     helpTopicRequest{valid: true},
		},
		{
			name:         "topic normalized",
			args:         []string{"atmos", "terraform", "plan", "--help=usage"},
			expectedArgs: []string{"atmos", "terraform", "plan", "--help"},
			expected: helpTopicRequest{
				topic:    helpTopicUsage,
				raw:      "usage",
				explicit: true,
				valid:    true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Args = append([]string(nil), tt.args...)
			currentHelpTopic = helpTopicRequest{topic: helpTopic("stale"), explicit: true}
			RootCmd.SetArgs(nil)

			preprocessHelpTopicArgs()

			assert.Equal(t, tt.expectedArgs, os.Args)
			assert.Equal(t, tt.expected, currentHelpTopic)
		})
	}
}

func TestTopicHelpRendering_InvalidTopicExits(t *testing.T) {
	if os.Getenv("ATMOS_TEST_INVALID_HELP_TOPIC") == "1" {
		var buf bytes.Buffer
		renderer := lipgloss.NewRenderer(&buf)
		styles := createHelpStyles(renderer)
		ctx := &helpRenderContext{
			writer: &buf,
			styles: &styles,
		}
		printHelpForTopic(ctx, testHelpCommand(t), helpTopicRequest{
			topic:    helpTopic("advanced"),
			raw:      "advanced",
			explicit: true,
			valid:    false,
		})
		return
	}

	execPath, err := exec.LookPath(os.Args[0])
	require.NoError(t, err)

	cmd := exec.Command(execPath, "-test.run=^TestTopicHelpRendering_InvalidTopicExits$")
	cmd.Env = append(os.Environ(), "ATMOS_TEST_INVALID_HELP_TOPIC=1", "NO_COLOR=1")
	output, err := cmd.CombinedOutput()

	var exitErr *exec.ExitError
	require.ErrorAs(t, err, &exitErr)
	assert.Equal(t, 1, exitErr.ExitCode())
	assert.True(t, strings.Contains(string(output), "unknown help topic") || strings.Contains(string(output), "Invalid Help Topic"))
}

func testHelpCommand(t *testing.T) *cobra.Command {
	t.Helper()

	root := &cobra.Command{
		Use:   "atmos",
		Short: "root command",
	}
	root.PersistentFlags().Bool("global", false, "global flag")

	child := &cobra.Command{
		Use:     "child",
		Short:   "child command",
		Long:    "Child command long description.",
		Example: "  atmos test child --local",
		Run:     func(cmd *cobra.Command, args []string) {},
	}
	child.Flags().Bool("local", false, "local flag")
	root.AddCommand(child)

	found, _, err := root.Find([]string{"child"})
	require.NoError(t, err)
	require.Equal(t, child, found)
	return child
}

func renderTopicHelpForTest(t *testing.T, topic helpTopicRequest, cmd *cobra.Command) string {
	t.Helper()
	t.Setenv(envNoColor, valueOne)

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	applyColoredHelpTemplateForTopic(cmd, topic)
	require.NoError(t, cmd.Help())
	return buf.String()
}
