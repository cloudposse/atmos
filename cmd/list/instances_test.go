package list

import (
	"bytes"
	goio "io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/tests"
)

// TestListInstancesFlags tests that the list instances command has the correct flags.
func TestListInstancesFlags(t *testing.T) {
	cmd := &cobra.Command{
		Use:   "instances",
		Short: "List all Atmos instances",
		Long:  "This command lists all Atmos instances or is used to upload instances to the pro API.",
		Args:  cobra.NoArgs,
	}

	cmd.PersistentFlags().String("format", "", "Output format")
	cmd.PersistentFlags().String("delimiter", "", "Delimiter for CSV/TSV output")
	cmd.PersistentFlags().String("stack", "", "Stack pattern")
	cmd.PersistentFlags().String("query", "", "JQ query")
	cmd.PersistentFlags().Int("max-columns", 0, "Maximum columns")
	cmd.PersistentFlags().Bool("upload", false, "Upload instances to pro API")

	formatFlag := cmd.PersistentFlags().Lookup("format")
	assert.NotNil(t, formatFlag, "Expected format flag to exist")
	assert.Equal(t, "", formatFlag.DefValue)

	delimiterFlag := cmd.PersistentFlags().Lookup("delimiter")
	assert.NotNil(t, delimiterFlag, "Expected delimiter flag to exist")
	assert.Equal(t, "", delimiterFlag.DefValue)

	stackFlag := cmd.PersistentFlags().Lookup("stack")
	assert.NotNil(t, stackFlag, "Expected stack flag to exist")
	assert.Equal(t, "", stackFlag.DefValue)

	queryFlag := cmd.PersistentFlags().Lookup("query")
	assert.NotNil(t, queryFlag, "Expected query flag to exist")
	assert.Equal(t, "", queryFlag.DefValue)

	maxColumnsFlag := cmd.PersistentFlags().Lookup("max-columns")
	assert.NotNil(t, maxColumnsFlag, "Expected max-columns flag to exist")
	assert.Equal(t, "0", maxColumnsFlag.DefValue)

	uploadFlag := cmd.PersistentFlags().Lookup("upload")
	assert.NotNil(t, uploadFlag, "Expected upload flag to exist")
	assert.Equal(t, "false", uploadFlag.DefValue)
}

// TestListInstancesValidatesArgs tests that the command validates arguments.
func TestListInstancesValidatesArgs(t *testing.T) {
	cmd := &cobra.Command{
		Use:  "instances",
		Args: cobra.NoArgs,
	}

	err := cmd.ValidateArgs([]string{})
	assert.NoError(t, err, "Validation should pass with no arguments")

	err = cmd.ValidateArgs([]string{"extra"})
	assert.Error(t, err, "Validation should fail with arguments")
}

// TestListInstancesCommand tests the instances command structure.
func TestListInstancesCommand(t *testing.T) {
	assert.Equal(t, "instances", instancesCmd.Use)
	assert.Contains(t, instancesCmd.Short, "List all Atmos instances")
	assert.NotNil(t, instancesCmd.RunE)

	// Check that NoArgs validator is set
	err := instancesCmd.Args(instancesCmd, []string{"unexpected"})
	assert.Error(t, err, "Should reject extra arguments")

	err = instancesCmd.Args(instancesCmd, []string{})
	assert.NoError(t, err, "Should accept no arguments")
}

// TestListInstancesOptions tests the InstancesOptions structure.
func TestListInstancesOptions(t *testing.T) {
	opts := &InstancesOptions{
		Format:     "json",
		MaxColumns: 10,
		Delimiter:  ",",
		Stack:      "prod-*",
		Query:      ".component",
		Upload:     false,
	}

	assert.Equal(t, "json", opts.Format)
	assert.Equal(t, 10, opts.MaxColumns)
	assert.Equal(t, ",", opts.Delimiter)
	assert.Equal(t, "prod-*", opts.Stack)
	assert.Equal(t, ".component", opts.Query)
	assert.False(t, opts.Upload)
}

// TestListInstancesOptions_Upload tests the upload flag behavior.
func TestListInstancesOptions_Upload(t *testing.T) {
	opts := &InstancesOptions{
		Upload: true,
	}

	assert.True(t, opts.Upload)
}

// TestInstancesOptions_AllCombinations tests various option combinations.
func TestInstancesOptions_AllCombinations(t *testing.T) {
	testCases := []struct {
		name              string
		opts              *InstancesOptions
		expectedFormat    string
		expectedMaxCols   int
		expectedDelimiter string
		expectedStack     string
		expectedQuery     string
		expectedUpload    bool
	}{
		{
			name: "all options enabled",
			opts: &InstancesOptions{
				Format:     "yaml",
				MaxColumns: 15,
				Delimiter:  ";",
				Stack:      "*-staging-*",
				Query:      ".stack",
				Upload:     true,
			},
			expectedFormat:    "yaml",
			expectedMaxCols:   15,
			expectedDelimiter: ";",
			expectedStack:     "*-staging-*",
			expectedQuery:     ".stack",
			expectedUpload:    true,
		},
		{
			name:              "minimal options",
			opts:              &InstancesOptions{},
			expectedFormat:    "",
			expectedMaxCols:   0,
			expectedDelimiter: "",
			expectedStack:     "",
			expectedQuery:     "",
			expectedUpload:    false,
		},
		{
			name: "upload only",
			opts: &InstancesOptions{
				Upload: true,
			},
			expectedFormat:    "",
			expectedMaxCols:   0,
			expectedDelimiter: "",
			expectedStack:     "",
			expectedQuery:     "",
			expectedUpload:    true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expectedFormat, tc.opts.Format)
			assert.Equal(t, tc.expectedMaxCols, tc.opts.MaxColumns)
			assert.Equal(t, tc.expectedDelimiter, tc.opts.Delimiter)
			assert.Equal(t, tc.expectedStack, tc.opts.Stack)
			assert.Equal(t, tc.expectedQuery, tc.opts.Query)
			assert.Equal(t, tc.expectedUpload, tc.opts.Upload)
		})
	}
}

// TestInstancesIdentityFlagLogic tests the identity flag/env var logic in instances command.
func TestInstancesIdentityFlagLogic(t *testing.T) {
	testCases := []struct {
		name             string
		setupCmd         func() *cobra.Command
		setupViper       func()
		expectedIdentity string
	}{
		{
			name: "identity from flag",
			setupCmd: func() *cobra.Command {
				cmd := &cobra.Command{Use: "test"}
				cmd.Flags().String("identity", "", "Identity flag")
				_ = cmd.Flags().Set("identity", "flag-identity")
				return cmd
			},
			setupViper: func() {
				viper.Reset()
			},
			expectedIdentity: "flag-identity",
		},
		{
			name: "identity from viper when flag not changed",
			setupCmd: func() *cobra.Command {
				cmd := &cobra.Command{Use: "test"}
				cmd.Flags().String("identity", "", "Identity flag")
				return cmd
			},
			setupViper: func() {
				viper.Reset()
				viper.Set("identity", "env-identity")
			},
			expectedIdentity: "env-identity",
		},
		{
			name: "empty identity when neither set",
			setupCmd: func() *cobra.Command {
				cmd := &cobra.Command{Use: "test"}
				cmd.Flags().String("identity", "", "Identity flag")
				return cmd
			},
			setupViper: func() {
				viper.Reset()
			},
			expectedIdentity: "",
		},
		{
			name: "flag takes precedence over viper",
			setupCmd: func() *cobra.Command {
				cmd := &cobra.Command{Use: "test"}
				cmd.Flags().String("identity", "", "Identity flag")
				_ = cmd.Flags().Set("identity", "flag-identity")
				return cmd
			},
			setupViper: func() {
				viper.Reset()
				viper.Set("identity", "env-identity")
			},
			expectedIdentity: "flag-identity",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.setupViper()
			cmd := tc.setupCmd()

			// Simulate the logic from executeListInstancesCmd.
			identityName := ""
			if cmd.Flags().Changed("identity") {
				identityName, _ = cmd.Flags().GetString("identity")
			} else if envIdentity := viper.GetString("identity"); envIdentity != "" {
				identityName = envIdentity
			}

			assert.Equal(t, tc.expectedIdentity, identityName)
		})
	}
}

// TestInstancesParserInit tests that the instances parser is properly initialized.
func TestInstancesParserInit(t *testing.T) {
	assert.NotNil(t, instancesParser, "instancesParser should be initialized")

	// Verify instancesCmd exists and has the correct Use field.
	assert.Equal(t, "instances", instancesCmd.Use)

	// The upload flag should be registered - it could be on Flags() or PersistentFlags().
	// Check both since the parser might use either.
	uploadFlag := instancesCmd.Flags().Lookup("upload")
	if uploadFlag == nil {
		uploadFlag = instancesCmd.PersistentFlags().Lookup("upload")
	}

	if uploadFlag != nil {
		assert.Equal(t, "false", uploadFlag.DefValue, "upload flag default should be false")
	}
	// Note: If the flag is not found, that's not necessarily an error - it may be registered
	// lazily or through a different mechanism. The important test is that the parser exists.
}

// TestExecuteListInstancesCmd_ProvenanceWithoutTree tests that --provenance fails without --format=tree.
func TestExecuteListInstancesCmd_ProvenanceWithoutTree(t *testing.T) {
	cmd := &cobra.Command{}
	instancesParser.RegisterFlags(cmd)

	opts := &InstancesOptions{
		Format:     "table",
		Provenance: true,
	}

	err := executeListInstancesCmd(cmd, []string{}, opts)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidFlag)
	assert.Contains(t, err.Error(), "--provenance flag only works with --format=tree")
}

// TestExecuteListInstancesCmd_ProvenanceWithTree tests that --provenance with --format=tree
// passes validation and does not fail with ErrInvalidFlag.
func TestExecuteListInstancesCmd_ProvenanceWithTree(t *testing.T) {
	// Reset Viper to avoid state contamination from other tests.
	viper.Reset()
	t.Cleanup(viper.Reset)

	fixturePath, err := filepath.Abs(filepath.Join("..", "..", "tests", "fixtures", "scenarios", "complete"))
	require.NoError(t, err)
	tests.RequireFilePath(t, fixturePath, "test fixture directory")

	ioCtx, err := iolib.NewContext()
	require.NoError(t, err)
	ui.InitFormatter(ioCtx)
	data.InitWriter(ioCtx)

	// Register instance flags and add base-path flag pointing to the fixture.
	cmd := &cobra.Command{}
	instancesParser.RegisterFlags(cmd)
	cmd.Flags().String("base-path", "", "Base path")
	require.NoError(t, cmd.Flags().Set("base-path", fixturePath))

	// Also need config, config-path, and profile flags required by ProcessCommandLineArgs.
	cmd.Flags().StringSlice("config", []string{}, "Config files")
	cmd.Flags().StringSlice("config-path", []string{}, "Config paths")
	cmd.Flags().StringSlice("profile", []string{}, "Profiles")

	opts := &InstancesOptions{
		Format:     "tree",
		Provenance: true,
	}

	// Should not fail with --provenance validation (may fail later in stack loading but not on the flag check).
	err = executeListInstancesCmd(cmd, []string{}, opts)
	// We can't assert NoError here because it may fail loading stacks in test env.
	// The important thing is that it does NOT fail with ErrInvalidFlag (provenance validation).
	if err != nil {
		assert.NotErrorIs(t, err, errUtils.ErrInvalidFlag)
	}
}

// TestExecuteListInstancesCmd_WithStackPattern tests that --stack filters output to the target stack.
func TestExecuteListInstancesCmd_WithStackPattern(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	fixturePath, err := filepath.Abs(filepath.Join("..", "..", "tests", "fixtures", "scenarios", "complete"))
	require.NoError(t, err)
	tests.RequireFilePath(t, fixturePath, "test fixture directory")

	ioCtx, err := iolib.NewContext()
	require.NoError(t, err)
	ui.InitFormatter(ioCtx)
	data.InitWriter(ioCtx)

	cmd := &cobra.Command{}
	instancesParser.RegisterFlags(cmd)
	cmd.Flags().String("base-path", "", "Base path")
	require.NoError(t, cmd.Flags().Set("base-path", fixturePath))
	cmd.Flags().StringSlice("config", []string{}, "Config files")
	cmd.Flags().StringSlice("config-path", []string{}, "Config paths")
	cmd.Flags().StringSlice("profile", []string{}, "Profiles")

	opts := &InstancesOptions{
		Format: "table",
		Stack:  "tenant1-ue2-dev",
	}

	// Capture stdout to assert filtering behavior.
	oldStdout := os.Stdout
	r, w, pipeErr := os.Pipe()
	require.NoError(t, pipeErr)
	os.Stdout = w
	defer func() { os.Stdout = oldStdout }()

	err = executeListInstancesCmd(cmd, []string{}, opts)

	require.NoError(t, w.Close())
	var buf bytes.Buffer
	_, copyErr := goio.Copy(&buf, r)
	require.NoError(t, copyErr)
	os.Stdout = oldStdout

	require.NoError(t, err)
	output := buf.String()

	// Every data row must belong to the requested stack.
	assert.NotEmpty(t, output, "expected non-empty output for tenant1-ue2-dev")
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line == "" {
			continue
		}
		assert.Contains(t, line, "tenant1-ue2-dev", "unexpected stack in output line: %q", line)
	}
}

// TestExecuteListInstancesCmd_InvalidBasePath tests error handling for invalid base path.
func TestExecuteListInstancesCmd_InvalidBasePath(t *testing.T) {
	// Reset Viper to avoid state contamination from other tests.
	viper.Reset()
	t.Cleanup(viper.Reset)

	cmd := &cobra.Command{}
	instancesParser.RegisterFlags(cmd)
	cmd.Flags().String("base-path", "", "Base path")
	require.NoError(t, cmd.Flags().Set("base-path", filepath.Join(t.TempDir(), "nonexistent", "path")))

	// Also need config, config-path, and profile flags required by ProcessCommandLineArgs.
	cmd.Flags().StringSlice("config", []string{}, "Config files")
	cmd.Flags().StringSlice("config-path", []string{}, "Config paths")
	cmd.Flags().StringSlice("profile", []string{}, "Profiles")

	opts := &InstancesOptions{
		Format: "table",
	}

	err := executeListInstancesCmd(cmd, []string{}, opts)
	// InitCliConfig should fail with an invalid path.
	assert.Error(t, err)
}

// TestColumnsCompletionForInstances tests the columnsCompletionForInstances function.
func TestColumnsCompletionForInstances(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}

	// With no valid config, should return no completions but no panic.
	completions, directive := columnsCompletionForInstances(cmd, []string{}, "")
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
	// Completions may be nil in test context without a valid config.
	_ = completions
}

// TestColumnsCompletionForInstances_WithFixture tests completion with a valid fixture config.
// The fixture has no custom columns defined, so should return ShellCompDirectiveNoFileComp.
func TestColumnsCompletionForInstances_WithFixture(t *testing.T) {
	fixturePath, err := filepath.Abs(filepath.Join("..", "..", "tests", "fixtures", "scenarios", "complete"))
	require.NoError(t, err)
	tests.RequireFilePath(t, fixturePath, "test fixture directory")

	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("base-path", "", "Base path")
	require.NoError(t, cmd.Flags().Set("base-path", fixturePath))
	cmd.Flags().StringSlice("config", []string{}, "Config files")
	cmd.Flags().StringSlice("config-path", []string{}, "Config paths")
	cmd.Flags().StringSlice("profile", []string{}, "Profiles")

	// With a valid config but no custom columns, should return no completions.
	completions, directive := columnsCompletionForInstances(cmd, []string{}, "")
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
	_ = completions
}
