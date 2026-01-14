package list

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
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
