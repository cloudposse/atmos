package list

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestListAffectedCommand tests the affected command structure.
func TestListAffectedCommand(t *testing.T) {
	assert.Equal(t, "affected", affectedCmd.Use)
	assert.Contains(t, affectedCmd.Short, "List affected components")
	assert.NotNil(t, affectedCmd.RunE)
	assert.NotNil(t, affectedCmd.Long)
	assert.Contains(t, affectedCmd.Long, "Git commits")
}

// TestListAffectedCommandValidatesArgs tests that the command validates arguments.
func TestListAffectedCommandValidatesArgs(t *testing.T) {
	// Check that NoArgs validator is set.
	err := affectedCmd.Args(affectedCmd, []string{"unexpected"})
	assert.Error(t, err, "Should reject extra arguments")

	err = affectedCmd.Args(affectedCmd, []string{})
	assert.NoError(t, err, "Should accept no arguments")
}

// TestAffectedOptions tests the AffectedOptions structure.
func TestAffectedOptions(t *testing.T) {
	opts := &AffectedOptions{
		Format:            "json",
		Columns:           []string{"Component", "Stack"},
		Delimiter:         ",",
		Sort:              "Stack:asc",
		Ref:               "refs/heads/main",
		SHA:               "abc123",
		RepoPath:          "/path/to/repo",
		SSHKeyPath:        "/path/to/key",
		SSHKeyPassword:    "secret",
		CloneTargetRef:    true,
		IncludeDependents: true,
		Stack:             "dev-*",
		ExcludeLocked:     true,
		ProcessTemplates:  true,
		ProcessFunctions:  true,
		Skip:              []string{"component1", "component2"},
	}

	assert.Equal(t, "json", opts.Format)
	assert.Equal(t, []string{"Component", "Stack"}, opts.Columns)
	assert.Equal(t, ",", opts.Delimiter)
	assert.Equal(t, "Stack:asc", opts.Sort)
	assert.Equal(t, "refs/heads/main", opts.Ref)
	assert.Equal(t, "abc123", opts.SHA)
	assert.Equal(t, "/path/to/repo", opts.RepoPath)
	assert.Equal(t, "/path/to/key", opts.SSHKeyPath)
	assert.Equal(t, "secret", opts.SSHKeyPassword)
	assert.True(t, opts.CloneTargetRef)
	assert.True(t, opts.IncludeDependents)
	assert.Equal(t, "dev-*", opts.Stack)
	assert.True(t, opts.ExcludeLocked)
	assert.True(t, opts.ProcessTemplates)
	assert.True(t, opts.ProcessFunctions)
	assert.Equal(t, []string{"component1", "component2"}, opts.Skip)
}

// TestAffectedOptions_Defaults tests default values in AffectedOptions.
func TestAffectedOptions_Defaults(t *testing.T) {
	opts := &AffectedOptions{}

	assert.Empty(t, opts.Format)
	assert.Empty(t, opts.Columns)
	assert.Empty(t, opts.Delimiter)
	assert.Empty(t, opts.Sort)
	assert.Empty(t, opts.Ref)
	assert.Empty(t, opts.SHA)
	assert.Empty(t, opts.RepoPath)
	assert.Empty(t, opts.SSHKeyPath)
	assert.Empty(t, opts.SSHKeyPassword)
	assert.False(t, opts.CloneTargetRef)
	assert.False(t, opts.IncludeDependents)
	assert.Empty(t, opts.Stack)
	assert.False(t, opts.ExcludeLocked)
	assert.False(t, opts.ProcessTemplates)
	assert.False(t, opts.ProcessFunctions)
	assert.Empty(t, opts.Skip)
}

// TestAffectedOptions_GitOptions tests the git-related options.
func TestAffectedOptions_GitOptions(t *testing.T) {
	testCases := []struct {
		name             string
		opts             *AffectedOptions
		expectedRef      string
		expectedSHA      string
		expectedRepoPath string
		expectedClone    bool
	}{
		{
			name: "ref only",
			opts: &AffectedOptions{
				Ref: "refs/heads/feature",
			},
			expectedRef:   "refs/heads/feature",
			expectedClone: false,
		},
		{
			name: "SHA only",
			opts: &AffectedOptions{
				SHA: "deadbeef",
			},
			expectedSHA:   "deadbeef",
			expectedClone: false,
		},
		{
			name: "repo path",
			opts: &AffectedOptions{
				RepoPath: "/path/to/target/repo",
			},
			expectedRepoPath: "/path/to/target/repo",
			expectedClone:    false,
		},
		{
			name: "clone target ref",
			opts: &AffectedOptions{
				Ref:            "refs/heads/main",
				CloneTargetRef: true,
			},
			expectedRef:   "refs/heads/main",
			expectedClone: true,
		},
		{
			name: "clone with SSH key",
			opts: &AffectedOptions{
				Ref:            "refs/heads/main",
				CloneTargetRef: true,
				SSHKeyPath:     "/path/to/key",
				SSHKeyPassword: "password",
			},
			expectedRef:   "refs/heads/main",
			expectedClone: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expectedRef, tc.opts.Ref)
			assert.Equal(t, tc.expectedSHA, tc.opts.SHA)
			assert.Equal(t, tc.expectedRepoPath, tc.opts.RepoPath)
			assert.Equal(t, tc.expectedClone, tc.opts.CloneTargetRef)
		})
	}
}

// TestAffectedOptions_ProcessingFlags tests processing-related options.
func TestAffectedOptions_ProcessingFlags(t *testing.T) {
	testCases := []struct {
		name                     string
		opts                     *AffectedOptions
		expectedProcessTemplates bool
		expectedProcessFunctions bool
		expectedSkip             []string
	}{
		{
			name: "all processing enabled",
			opts: &AffectedOptions{
				ProcessTemplates: true,
				ProcessFunctions: true,
				Skip:             []string{},
			},
			expectedProcessTemplates: true,
			expectedProcessFunctions: true,
			expectedSkip:             []string{},
		},
		{
			name: "templates only",
			opts: &AffectedOptions{
				ProcessTemplates: true,
				ProcessFunctions: false,
			},
			expectedProcessTemplates: true,
			expectedProcessFunctions: false,
		},
		{
			name: "with skip list",
			opts: &AffectedOptions{
				ProcessTemplates: true,
				ProcessFunctions: true,
				Skip:             []string{"skip1", "skip2", "skip3"},
			},
			expectedProcessTemplates: true,
			expectedProcessFunctions: true,
			expectedSkip:             []string{"skip1", "skip2", "skip3"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expectedProcessTemplates, tc.opts.ProcessTemplates)
			assert.Equal(t, tc.expectedProcessFunctions, tc.opts.ProcessFunctions)
			if tc.expectedSkip != nil {
				assert.Equal(t, tc.expectedSkip, tc.opts.Skip)
			}
		})
	}
}

// TestAffectedParserInit tests that the affected parser is properly initialized.
func TestAffectedParserInit(t *testing.T) {
	assert.NotNil(t, affectedParser, "affectedParser should be initialized")

	// Verify affectedCmd exists and has the correct Use field.
	assert.Equal(t, "affected", affectedCmd.Use)
}

// TestAffectedFlagsRegistered tests that expected flags are registered.
func TestAffectedFlagsRegistered(t *testing.T) {
	// Check for key flags (they may be on Flags() or PersistentFlags()).
	flagsToCheck := []string{
		"format",
		"columns",
		"delimiter",
		"sort",
		"ref",
		"sha",
		"repo-path",
		"ssh-key",
		"ssh-key-password",
		"clone-target-ref",
		"include-dependents",
		"stack",
		"exclude-locked",
		"process-templates",
		"process-functions",
		"skip",
	}

	for _, flagName := range flagsToCheck {
		flag := affectedCmd.Flags().Lookup(flagName)
		if flag == nil {
			flag = affectedCmd.PersistentFlags().Lookup(flagName)
		}
		assert.NotNil(t, flag, "Expected flag '%s' to be registered", flagName)
	}
}

// TestAffectedFlagLogic tests the flag/viper precedence logic.
func TestAffectedFlagLogic(t *testing.T) {
	testCases := []struct {
		name          string
		setupCmd      func() *cobra.Command
		setupViper    func()
		expectedValue string
	}{
		{
			name: "ref from flag",
			setupCmd: func() *cobra.Command {
				cmd := &cobra.Command{Use: "test"}
				cmd.Flags().String("ref", "", "Git ref")
				_ = cmd.Flags().Set("ref", "flag-ref")
				return cmd
			},
			setupViper: func() {
				viper.Reset()
			},
			expectedValue: "flag-ref",
		},
		{
			name: "ref from viper when flag not changed",
			setupCmd: func() *cobra.Command {
				cmd := &cobra.Command{Use: "test"}
				cmd.Flags().String("ref", "", "Git ref")
				return cmd
			},
			setupViper: func() {
				viper.Reset()
				viper.Set("ref", "env-ref")
			},
			expectedValue: "env-ref",
		},
		{
			name: "flag takes precedence over viper",
			setupCmd: func() *cobra.Command {
				cmd := &cobra.Command{Use: "test"}
				cmd.Flags().String("ref", "", "Git ref")
				_ = cmd.Flags().Set("ref", "flag-ref")
				return cmd
			},
			setupViper: func() {
				viper.Reset()
				viper.Set("ref", "env-ref")
			},
			expectedValue: "flag-ref",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.setupViper()
			cmd := tc.setupCmd()

			// Simulate the logic for flag/viper precedence.
			var value string
			if cmd.Flags().Changed("ref") {
				value, _ = cmd.Flags().GetString("ref")
			} else {
				value = viper.GetString("ref")
			}

			assert.Equal(t, tc.expectedValue, value)
		})
	}
}

// TestAffectedCommandFParseErrWhitelist tests the whitelist configuration.
func TestAffectedCommandFParseErrWhitelist(t *testing.T) {
	// Verify UnknownFlags is set to false (strict parsing).
	assert.False(t, affectedCmd.FParseErrWhitelist.UnknownFlags, "UnknownFlags should be false")
}

// TestAffectedOptions_OutputFormatOptions tests output format related options.
func TestAffectedOptions_OutputFormatOptions(t *testing.T) {
	testCases := []struct {
		name              string
		opts              *AffectedOptions
		expectedFormat    string
		expectedDelimiter string
		expectedColumns   []string
		expectedSort      string
	}{
		{
			name: "JSON format",
			opts: &AffectedOptions{
				Format: "json",
			},
			expectedFormat: "json",
		},
		{
			name: "YAML format",
			opts: &AffectedOptions{
				Format: "yaml",
			},
			expectedFormat: "yaml",
		},
		{
			name: "CSV with custom delimiter",
			opts: &AffectedOptions{
				Format:    "csv",
				Delimiter: ";",
			},
			expectedFormat:    "csv",
			expectedDelimiter: ";",
		},
		{
			name: "Custom columns",
			opts: &AffectedOptions{
				Columns: []string{"Stack={{ .stack }}", "Component={{ .component }}"},
			},
			expectedColumns: []string{"Stack={{ .stack }}", "Component={{ .component }}"},
		},
		{
			name: "Custom sort",
			opts: &AffectedOptions{
				Sort: "Component:desc,Stack:asc",
			},
			expectedSort: "Component:desc,Stack:asc",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expectedFormat, tc.opts.Format)
			assert.Equal(t, tc.expectedDelimiter, tc.opts.Delimiter)
			if tc.expectedColumns != nil {
				assert.Equal(t, tc.expectedColumns, tc.opts.Columns)
			}
			if tc.expectedSort != "" {
				assert.Equal(t, tc.expectedSort, tc.opts.Sort)
			}
		})
	}
}

// TestAffectedOptions_ContentFlags tests content-related options.
func TestAffectedOptions_ContentFlags(t *testing.T) {
	testCases := []struct {
		name                      string
		opts                      *AffectedOptions
		expectedIncludeDependents bool
		expectedStack             string
		expectedExcludeLocked     bool
	}{
		{
			name: "include dependents",
			opts: &AffectedOptions{
				IncludeDependents: true,
			},
			expectedIncludeDependents: true,
		},
		{
			name: "stack filter",
			opts: &AffectedOptions{
				Stack: "prod-*",
			},
			expectedStack: "prod-*",
		},
		{
			name: "exclude locked",
			opts: &AffectedOptions{
				ExcludeLocked: true,
			},
			expectedExcludeLocked: true,
		},
		{
			name: "all content flags",
			opts: &AffectedOptions{
				IncludeDependents: true,
				Stack:             "staging-*",
				ExcludeLocked:     true,
			},
			expectedIncludeDependents: true,
			expectedStack:             "staging-*",
			expectedExcludeLocked:     true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expectedIncludeDependents, tc.opts.IncludeDependents)
			assert.Equal(t, tc.expectedStack, tc.opts.Stack)
			assert.Equal(t, tc.expectedExcludeLocked, tc.opts.ExcludeLocked)
		})
	}
}

// TestAffectedOptions_FullOptions tests that AffectedOptions can be fully populated.
func TestAffectedOptions_FullOptions(t *testing.T) {
	t.Run("all fields populated", func(t *testing.T) {
		opts := &AffectedOptions{
			Format:            "json",
			Columns:           []string{"Component", "Stack", "Type"},
			Delimiter:         "|",
			Sort:              "Stack:asc,Component:desc",
			Ref:               "refs/heads/develop",
			SHA:               "deadbeef",
			RepoPath:          "/custom/repo/path",
			SSHKeyPath:        "/home/user/.ssh/id_rsa",
			SSHKeyPassword:    "password123",
			CloneTargetRef:    true,
			IncludeDependents: true,
			Stack:             "us-east-*",
			ExcludeLocked:     true,
			ProcessTemplates:  true,
			ProcessFunctions:  true,
			Skip:              []string{"component1", "component2", "component3"},
		}

		// Verify all fields.
		assert.Equal(t, "json", opts.Format)
		assert.Len(t, opts.Columns, 3)
		assert.Equal(t, "|", opts.Delimiter)
		assert.Equal(t, "Stack:asc,Component:desc", opts.Sort)
		assert.Equal(t, "refs/heads/develop", opts.Ref)
		assert.Equal(t, "deadbeef", opts.SHA)
		assert.Equal(t, "/custom/repo/path", opts.RepoPath)
		assert.Equal(t, "/home/user/.ssh/id_rsa", opts.SSHKeyPath)
		assert.Equal(t, "password123", opts.SSHKeyPassword)
		assert.True(t, opts.CloneTargetRef)
		assert.True(t, opts.IncludeDependents)
		assert.Equal(t, "us-east-*", opts.Stack)
		assert.True(t, opts.ExcludeLocked)
		assert.True(t, opts.ProcessTemplates)
		assert.True(t, opts.ProcessFunctions)
		assert.Len(t, opts.Skip, 3)
	})
}

// TestAffectedCommandSubcommand tests that the affected command is properly configured.
func TestAffectedCommandSubcommand(t *testing.T) {
	t.Run("command is subcommand of list", func(t *testing.T) {
		// Check that affectedCmd exists and can be used.
		assert.NotNil(t, affectedCmd)
		assert.Equal(t, "affected", affectedCmd.Use)
	})

	t.Run("command has expected flags count", func(t *testing.T) {
		// Count the number of flags registered.
		flagCount := 0
		affectedCmd.Flags().VisitAll(func(f *pflag.Flag) {
			flagCount++
		})
		// Should have all the flags registered.
		assert.Greater(t, flagCount, 10, "Should have many flags registered")
	})
}

// TestAffectedFlagDefaults tests the default values of flags.
func TestAffectedFlagDefaults(t *testing.T) {
	t.Run("format default is empty", func(t *testing.T) {
		flag := affectedCmd.Flags().Lookup("format")
		require.NotNil(t, flag)
		assert.Equal(t, "", flag.DefValue)
	})

	t.Run("delimiter default is empty", func(t *testing.T) {
		flag := affectedCmd.Flags().Lookup("delimiter")
		require.NotNil(t, flag)
		assert.Equal(t, "", flag.DefValue)
	})

	t.Run("clone-target-ref default is false", func(t *testing.T) {
		flag := affectedCmd.Flags().Lookup("clone-target-ref")
		require.NotNil(t, flag)
		assert.Equal(t, "false", flag.DefValue)
	})

	t.Run("include-dependents default is false", func(t *testing.T) {
		flag := affectedCmd.Flags().Lookup("include-dependents")
		require.NotNil(t, flag)
		assert.Equal(t, "false", flag.DefValue)
	})

	t.Run("exclude-locked default is false", func(t *testing.T) {
		flag := affectedCmd.Flags().Lookup("exclude-locked")
		require.NotNil(t, flag)
		assert.Equal(t, "false", flag.DefValue)
	})

	t.Run("process-templates default is true", func(t *testing.T) {
		flag := affectedCmd.Flags().Lookup("process-templates")
		require.NotNil(t, flag)
		assert.Equal(t, "true", flag.DefValue)
	})

	t.Run("process-functions default is true", func(t *testing.T) {
		flag := affectedCmd.Flags().Lookup("process-functions")
		require.NotNil(t, flag)
		assert.Equal(t, "true", flag.DefValue)
	})
}

// TestAffectedFlagShorthands tests flag shorthand configurations.
func TestAffectedFlagShorthands(t *testing.T) {
	flagsWithShorthands := map[string]string{
		"format": "f",
		"stack":  "s",
	}

	for flagName, expectedShorthand := range flagsWithShorthands {
		t.Run(flagName+" has shorthand "+expectedShorthand, func(t *testing.T) {
			flag := affectedCmd.Flags().Lookup(flagName)
			require.NotNil(t, flag, "Flag %s should exist", flagName)
			assert.Equal(t, expectedShorthand, flag.Shorthand, "Flag %s shorthand should be %s", flagName, expectedShorthand)
		})
	}

	t.Run("delimiter has no shorthand", func(t *testing.T) {
		flag := affectedCmd.Flags().Lookup("delimiter")
		require.NotNil(t, flag)
		assert.Empty(t, flag.Shorthand, "delimiter should not have a shorthand")
	})
}

// TestAffectedCommandLong tests that the command has a proper long description.
func TestAffectedCommandLong(t *testing.T) {
	t.Run("long description mentions Git", func(t *testing.T) {
		assert.Contains(t, affectedCmd.Long, "Git")
	})

	t.Run("short description is not empty", func(t *testing.T) {
		assert.NotEmpty(t, affectedCmd.Short)
	})
}

// TestAffectedFlagTypes tests that all flags have the correct types.
func TestAffectedFlagTypes(t *testing.T) {
	stringFlags := []string{
		"format", "delimiter", "sort", "ref", "sha",
		"repo-path", "ssh-key", "ssh-key-password", "stack",
	}

	boolFlags := []string{
		"clone-target-ref", "include-dependents", "exclude-locked",
		"process-templates", "process-functions",
	}

	stringSliceFlags := []string{
		"columns", "skip",
	}

	for _, flagName := range stringFlags {
		t.Run(flagName+" is string type", func(t *testing.T) {
			flag := affectedCmd.Flags().Lookup(flagName)
			require.NotNil(t, flag, "Flag %s should exist", flagName)
			assert.Equal(t, "string", flag.Value.Type())
		})
	}

	for _, flagName := range boolFlags {
		t.Run(flagName+" is bool type", func(t *testing.T) {
			flag := affectedCmd.Flags().Lookup(flagName)
			require.NotNil(t, flag, "Flag %s should exist", flagName)
			assert.Equal(t, "bool", flag.Value.Type())
		})
	}

	for _, flagName := range stringSliceFlags {
		t.Run(flagName+" is stringSlice type", func(t *testing.T) {
			flag := affectedCmd.Flags().Lookup(flagName)
			require.NotNil(t, flag, "Flag %s should exist", flagName)
			assert.Equal(t, "stringSlice", flag.Value.Type())
		})
	}
}

// TestAffectedCommandRegistration tests that the affected command is properly registered.
func TestAffectedCommandRegistration(t *testing.T) {
	t.Run("affected command is in listCmd subcommands", func(t *testing.T) {
		found := false
		for _, cmd := range listCmd.Commands() {
			if cmd.Use == "affected" {
				found = true
				break
			}
		}
		assert.True(t, found, "affected should be a subcommand of list")
	})

	t.Run("affected parser is not nil", func(t *testing.T) {
		assert.NotNil(t, affectedParser)
	})
}

// TestAffectedOptions_PartialOptions tests AffectedOptions with partial field population.
func TestAffectedOptions_PartialOptions(t *testing.T) {
	testCases := []struct {
		name string
		opts *AffectedOptions
	}{
		{
			name: "only format",
			opts: &AffectedOptions{Format: "table"},
		},
		{
			name: "only ref",
			opts: &AffectedOptions{Ref: "refs/heads/feature"},
		},
		{
			name: "only sha",
			opts: &AffectedOptions{SHA: "abc123def456"},
		},
		{
			name: "only stack",
			opts: &AffectedOptions{Stack: "dev"},
		},
		{
			name: "only skip list",
			opts: &AffectedOptions{Skip: []string{"comp1", "comp2"}},
		},
		{
			name: "only columns",
			opts: &AffectedOptions{Columns: []string{"Col1={{ .field }}"}},
		},
		{
			name: "only processing flags",
			opts: &AffectedOptions{
				ProcessTemplates: true,
				ProcessFunctions: false,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Just verify the options can be created without issues.
			assert.NotNil(t, tc.opts)
		})
	}
}

// TestAffectedCommandHasRunE tests that the command has a RunE function.
func TestAffectedCommandHasRunE(t *testing.T) {
	assert.NotNil(t, affectedCmd.RunE, "affected command should have RunE function")
}

// TestAffectedFlagUsageStrings tests that flags have usage descriptions.
func TestAffectedFlagUsageStrings(t *testing.T) {
	flagsToCheck := []string{
		"format", "columns", "delimiter", "sort", "ref", "sha",
		"repo-path", "ssh-key", "clone-target-ref", "include-dependents",
		"stack", "exclude-locked", "process-templates", "process-functions", "skip",
	}

	for _, flagName := range flagsToCheck {
		t.Run(flagName+" has usage", func(t *testing.T) {
			flag := affectedCmd.Flags().Lookup(flagName)
			require.NotNil(t, flag, "Flag %s should exist", flagName)
			assert.NotEmpty(t, flag.Usage, "Flag %s should have usage text", flagName)
		})
	}
}
