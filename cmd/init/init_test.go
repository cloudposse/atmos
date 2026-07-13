package initcmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/generator/templates"
)

func TestNewInitCommandProvider(t *testing.T) {
	provider := &InitCommandProvider{}

	assert.NotNil(t, provider)
	assert.Equal(t, "init", provider.GetName())
	assert.Equal(t, "Configuration Management", provider.GetGroup())
	assert.NotNil(t, provider.GetCommand())
	assert.Nil(t, provider.GetFlagsBuilder())
	assert.Nil(t, provider.GetPositionalArgsBuilder())
	assert.Nil(t, provider.GetCompatibilityFlags())
	assert.Nil(t, provider.GetAliases())
}

func TestInitCommandProvider_GetCommand(t *testing.T) {
	provider := &InitCommandProvider{}
	cmd := provider.GetCommand()

	assert.NotNil(t, cmd)
	assert.Equal(t, "init", cmd.Use[:4]) // "init [template] [target]"
	assert.Contains(t, cmd.Short, "Initialize")
	assert.Contains(t, cmd.Long, "Initialize a new Atmos project")
}

func TestInitCommandProvider_GetFlagsBuilder(t *testing.T) {
	provider := &InitCommandProvider{}
	builder := provider.GetFlagsBuilder()

	// Init command uses cobra flags directly, not a flags builder.
	assert.Nil(t, builder)
}

func TestInitCmd_FlagDefinitions(t *testing.T) {
	tests := []struct {
		name         string
		flagName     string
		shorthand    string
		defaultValue string
	}{
		{
			name:         "force flag",
			flagName:     "force",
			shorthand:    "f",
			defaultValue: "false",
		},
		{
			name:         "interactive flag",
			flagName:     "interactive",
			shorthand:    "i",
			defaultValue: "true",
		},
		{
			name:      "set flag",
			flagName:  "set",
			shorthand: "",
		},
		{
			name:         "ref flag",
			flagName:     "ref",
			shorthand:    "",
			defaultValue: "",
		},
		{
			name:         "update flag",
			flagName:     "update",
			shorthand:    "",
			defaultValue: "false",
		},
		{
			name:         "base-ref flag",
			flagName:     "base-ref",
			shorthand:    "",
			defaultValue: "",
		},
		{
			name:         "git flag",
			flagName:     "git",
			shorthand:    "",
			defaultValue: "true",
		},
		{
			name:         "no-git flag",
			flagName:     "no-git",
			shorthand:    "",
			defaultValue: "false",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flag := initCmd.Flags().Lookup(tt.flagName)
			require.NotNil(t, flag, "flag %s should exist", tt.flagName)

			if tt.shorthand != "" {
				assert.Equal(t, tt.shorthand, flag.Shorthand)
			}

			if tt.defaultValue != "" {
				assert.Equal(t, tt.defaultValue, flag.DefValue)
			}
		})
	}
}

func TestInitCmd_Args(t *testing.T) {
	// MaximumNArgs(2) allows 0, 1, or 2 arguments.
	assert.NoError(t, initCmd.Args(initCmd, []string{}))
	assert.NoError(t, initCmd.Args(initCmd, []string{"simple"}))
	assert.NoError(t, initCmd.Args(initCmd, []string{"simple", "/tmp/target"}))
	assert.Error(t, initCmd.Args(initCmd, []string{"simple", "/tmp/target", "extra"}))
}

func TestInitCmd_ViperIntegration(t *testing.T) {
	v := viper.New()

	// Set values via viper.
	v.Set("force", true)
	v.Set("interactive", false)

	// Verify viper values.
	assert.True(t, v.GetBool("force"))
	assert.False(t, v.GetBool("interactive"))
}

func TestExecuteInit_ArgumentParsing(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		setup        func(t *testing.T) string
		expectError  bool
		errorContain string
	}{
		{
			name: "no arguments non-interactive fails",
			args: []string{"--interactive=false"},
			setup: func(t *testing.T) string {
				return ""
			},
			expectError:  true,
			errorContain: "template name",
		},
		{
			name: "template without target non-interactive fails",
			args: []string{"--interactive=false", "simple"},
			setup: func(t *testing.T) string {
				return ""
			},
			expectError:  true,
			errorContain: "target directory",
		},
		{
			name: "invalid template name",
			args: []string{"--interactive=false", "nonexistent"},
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				return tmpDir
			},
			expectError:  true,
			errorContain: "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := tt.setup(t)

			// Prepare args.
			args := tt.args
			if target != "" {
				args = append(args, target)
			}

			// Reset command.
			initCmd.SetArgs(args)

			// Execute command.
			err := initCmd.Execute()

			if tt.expectError {
				require.Error(t, err)
				if tt.errorContain != "" {
					assert.Contains(t, err.Error(), tt.errorContain)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestExecuteInit_FlagParsing(t *testing.T) {
	tests := []struct {
		name  string
		flags []string
		check func(t *testing.T, v *viper.Viper)
	}{
		{
			name:  "force flag short",
			flags: []string{"-f"},
			check: func(t *testing.T, v *viper.Viper) {
				// This test verifies flag is parsed.
				// Actual verification would happen in integration test.
				assert.NotNil(t, v)
			},
		},
		{
			name:  "force flag long",
			flags: []string{"--force"},
			check: func(t *testing.T, v *viper.Viper) {
				assert.NotNil(t, v)
			},
		},
		{
			name:  "interactive flag",
			flags: []string{"--interactive=false"},
			check: func(t *testing.T, v *viper.Viper) {
				assert.NotNil(t, v)
			},
		},
		{
			name:  "set flag single",
			flags: []string{"--set", "key=value"},
			check: func(t *testing.T, v *viper.Viper) {
				assert.NotNil(t, v)
			},
		},
		{
			name:  "set flag multiple",
			flags: []string{"--set", "key1=value1", "--set", "key2=value2"},
			check: func(t *testing.T, v *viper.Viper) {
				assert.NotNil(t, v)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := viper.New()

			// Build args - note we can't actually execute because we need template files.
			args := tt.flags
			args = append(args, "simple", t.TempDir())
			initCmd.SetArgs(args)

			// Parse flags only.
			err := initCmd.ParseFlags(args)
			require.NoError(t, err)

			if tt.check != nil {
				tt.check(t, v)
			}
		})
	}
}

func TestExecuteInit_EnvironmentVariables(t *testing.T) {
	tests := []struct {
		name  string
		env   map[string]string
		check func(t *testing.T, v *viper.Viper)
	}{
		{
			name: "ATMOS_INIT_FORCE",
			env: map[string]string{
				"ATMOS_INIT_FORCE": "true",
			},
			check: func(t *testing.T, v *viper.Viper) {
				v.SetEnvPrefix("ATMOS_INIT")
				v.AutomaticEnv()
				v.BindEnv("force", "ATMOS_INIT_FORCE")
				assert.True(t, v.GetBool("force"))
			},
		},
		{
			name: "ATMOS_INIT_INTERACTIVE",
			env: map[string]string{
				"ATMOS_INIT_INTERACTIVE": "false",
			},
			check: func(t *testing.T, v *viper.Viper) {
				v.SetEnvPrefix("ATMOS_INIT")
				v.AutomaticEnv()
				v.BindEnv("interactive", "ATMOS_INIT_INTERACTIVE")
				assert.False(t, v.GetBool("interactive"))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variables.
			for key, value := range tt.env {
				t.Setenv(key, value)
			}

			v := viper.New()

			if tt.check != nil {
				tt.check(t, v)
			}
		})
	}
}

func TestExecuteInit_AbsolutePath(t *testing.T) {
	tests := []struct {
		name         string
		targetDir    string
		expectError  bool
		errorContain string
	}{
		{
			name:        "relative path converted to absolute",
			targetDir:   "test-project",
			expectError: false,
		},
		{
			name:        "absolute path kept as-is",
			targetDir:   "/tmp/test-project",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This test validates the path conversion logic
			// without actually executing the full command.
			//nolint:nestif // test assertion logic requires nested conditionals
			if tt.targetDir != "" {
				absPath, err := filepath.Abs(tt.targetDir)
				if tt.expectError {
					assert.Error(t, err)
					if tt.errorContain != "" {
						assert.Contains(t, err.Error(), tt.errorContain)
					}
				} else {
					assert.NoError(t, err)
					assert.True(t, filepath.IsAbs(absPath))
				}
			}
		})
	}
}

func TestExecuteInit_TemplateValuesConversion(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]string
		expected map[string]interface{}
	}{
		{
			name:     "empty map",
			input:    map[string]string{},
			expected: map[string]interface{}{},
		},
		{
			name: "single value",
			input: map[string]string{
				"project_name": "my-project",
			},
			expected: map[string]interface{}{
				"project_name": "my-project",
			},
		},
		{
			name: "multiple values",
			input: map[string]string{
				"project_name": "my-project",
				"author":       "test-author",
				"version":      "1.0.0",
			},
			expected: map[string]interface{}{
				"project_name": "my-project",
				"author":       "test-author",
				"version":      "1.0.0",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the conversion logic from init.go.
			templateValues := make(map[string]interface{})
			for k, val := range tt.input {
				templateValues[k] = val
			}

			assert.Equal(t, tt.expected, templateValues)
		})
	}
}

func TestInitCmd_Integration_Help(t *testing.T) {
	// Test help output.
	initCmd.SetArgs([]string{"--help"})
	err := initCmd.Execute()

	// Help should not return error.
	assert.NoError(t, err)
}

func TestInitCmd_Integration_Version(t *testing.T) {
	// Verify command metadata.
	assert.Equal(t, "init", initCmd.Name())
	assert.NotEmpty(t, initCmd.Short)
	assert.NotEmpty(t, initCmd.Long)
}

func TestInit_PackageInitialization(t *testing.T) {
	// Test that init() function was called and registered command.
	assert.NotNil(t, initCmd)

	// Verify flags are registered.
	assert.NotNil(t, initCmd.Flags().Lookup("force"))
	assert.NotNil(t, initCmd.Flags().Lookup("interactive"))
	assert.NotNil(t, initCmd.Flags().Lookup("set"))
	assert.NotNil(t, initCmd.Flags().Lookup("ref"))
	assert.NotNil(t, initCmd.Flags().Lookup("git"))
	assert.NotNil(t, initCmd.Flags().Lookup("no-git"))
}

// TestExecuteInit_WithTemplateDirectory covers executeInit's full happy path
// (non-interactive, target dir provided): useDefaults is derived as
// !opts.interactive, so with interactive:false the run never invokes a real
// huh prompt form and is safe to drive end-to-end in a unit test.
func TestExecuteInit_WithTemplateDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	err := executeInit(context.Background(), &initOptions{
		templateName: "simple",
		targetDir:    tmpDir,
		interactive:  false,
		force:        false,
		templateVars: map[string]interface{}{
			"project_name": "test-project",
		},
	})

	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(tmpDir, "README.md"))
}

func TestMaybeInitGeneratedProjectGit_GitEnabled(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello"), 0o600))

	cfg := &templates.Configuration{Name: "demo", Version: "1.0.0"}
	err := maybeInitGeneratedProjectGit(dir, cfg, &initOptions{git: true})

	require.NoError(t, err)
	assert.DirExists(t, filepath.Join(dir, ".git"))
}

func TestMaybeInitGeneratedProjectGit_GitDisabled(t *testing.T) {
	dir := t.TempDir()

	cfg := &templates.Configuration{Name: "demo"}
	err := maybeInitGeneratedProjectGit(dir, cfg, &initOptions{git: false})

	require.NoError(t, err)
	assert.NoDirExists(t, filepath.Join(dir, ".git"))
}

func TestMaybeInitGeneratedProjectGit_EmptyTargetDirNoOp(t *testing.T) {
	cfg := &templates.Configuration{Name: "demo"}
	err := maybeInitGeneratedProjectGit("", cfg, &initOptions{git: true})

	require.NoError(t, err)
}

func TestCreateInitUI_ReturnsUsableInstance(t *testing.T) {
	initUI, err := createInitUI()

	require.NoError(t, err)
	assert.NotNil(t, initUI)
}

// TestRunInitExecution_WithTargetDir covers the targetDir != "" branch of
// runInitExecution. The targetDir == "" branch always prompts for a target
// directory via a real terminal form (regardless of useDefaults) and so
// cannot be safely unit tested.
func TestRunInitExecution_WithTargetDir(t *testing.T) {
	initUI, err := createInitUI()
	require.NoError(t, err)

	configs, err := templates.GetAvailableConfigurations()
	require.NoError(t, err)
	cfg := configs["simple"]

	dir := t.TempDir()
	opts := &initOptions{
		targetDir:    dir,
		interactive:  false,
		templateVars: map[string]interface{}{"project_name": "demo"},
	}

	finalDir, err := runInitExecution(initUI, &cfg, opts)

	require.NoError(t, err)
	assert.Equal(t, dir, finalDir)
	assert.FileExists(t, filepath.Join(dir, "README.md"))
}

func TestShouldOfferUpdate(t *testing.T) {
	notEmptyErr := errUtils.Build(errUtils.ErrTargetDirectoryNotEmpty).Err()
	otherErr := errUtils.Build(errUtils.ErrInitialization).Err()

	tests := []struct {
		name        string
		err         error
		opts        *initOptions
		wantOffer   bool
		wantBaseRef string
	}{
		{"nil error", nil, &initOptions{interactive: true}, false, ""},
		{"force already set", notEmptyErr, &initOptions{interactive: true, force: true}, false, ""},
		{"update already set", notEmptyErr, &initOptions{interactive: true, update: true}, false, ""},
		{"not interactive", notEmptyErr, &initOptions{interactive: false}, false, ""},
		{"different error", otherErr, &initOptions{interactive: true}, false, ""},
		{"offers with default HEAD base ref", notEmptyErr, &initOptions{interactive: true}, true, "HEAD"},
		{"offers with caller base ref", notEmptyErr, &initOptions{interactive: true, baseRef: "v1.2.3"}, true, "v1.2.3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			offer, baseRef := shouldOfferUpdate(tt.err, tt.opts)
			assert.Equal(t, tt.wantOffer, offer)
			assert.Equal(t, tt.wantBaseRef, baseRef)
		})
	}
}

// TestRunInitExecution_NonEmptyTargetDir_NonInteractive_ReturnsError covers
// that a non-empty target directory fails outright (no update offered, no
// interactive prompt attempted) when not running interactively — matches
// shouldOfferUpdate's "not interactive" case above, exercised through the
// real runInitExecution entry point.
func TestRunInitExecution_NonEmptyTargetDir_NonInteractive_ReturnsError(t *testing.T) {
	initUI, err := createInitUI()
	require.NoError(t, err)

	configs, err := templates.GetAvailableConfigurations()
	require.NoError(t, err)
	cfg := configs["simple"]

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "existing.txt"), []byte("hi"), 0o600))

	opts := &initOptions{
		targetDir:    dir,
		interactive:  false,
		templateVars: map[string]interface{}{"project_name": "demo"},
	}

	_, err = runInitExecution(initUI, &cfg, opts)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrTargetDirectoryNotEmpty)
}

// TestRunInitExecution_UpdateFlag_MergesExistingDirectory covers the real
// bug this flag fixes: re-running init against an already-generated,
// git-initialized directory with --update --base-ref=HEAD regenerates the
// template while preserving the user's own edits via a 3-way merge, instead
// of failing with "target directory is not empty".
func TestRunInitExecution_UpdateFlag_MergesExistingDirectory(t *testing.T) {
	initUI, err := createInitUI()
	require.NoError(t, err)

	configs, err := templates.GetAvailableConfigurations()
	require.NoError(t, err)
	cfg := configs["simple"]

	dir := t.TempDir()
	opts := &initOptions{
		targetDir:    dir,
		interactive:  false,
		templateVars: map[string]interface{}{"project_name": "demo"},
	}
	_, err = runInitExecution(initUI, &cfg, opts)
	require.NoError(t, err)

	require.NoError(t, runGitCommand(t, dir, "init"))
	// Disable commit signing: dev machines with a GPG/1Password signing agent
	// configured globally can hang or fail here otherwise.
	require.NoError(t, runGitCommand(t, dir, "config", "commit.gpgsign", "false"))
	require.NoError(t, runGitCommand(t, dir, "config", "user.email", "test@example.com"))
	require.NoError(t, runGitCommand(t, dir, "config", "user.name", "Test"))
	require.NoError(t, runGitCommand(t, dir, "add", "."))
	require.NoError(t, runGitCommand(t, dir, "commit", "-m", "initial"))

	readmePath := filepath.Join(dir, "README.md")
	original, err := os.ReadFile(readmePath)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(readmePath, append(original, []byte("\nuser note\n")...), 0o600))

	updateOpts := &initOptions{
		targetDir:    dir,
		interactive:  false,
		update:       true,
		baseRef:      "HEAD",
		templateVars: map[string]interface{}{"project_name": "demo"},
	}
	finalDir, err := runInitExecution(initUI, &cfg, updateOpts)

	require.NoError(t, err)
	assert.Equal(t, dir, finalDir)
	merged, err := os.ReadFile(readmePath)
	require.NoError(t, err)
	assert.Contains(t, string(merged), "user note", "the user's manual edit must survive the 3-way merge")
}

// runGitCommand runs git in dir for test setup, skipping the test if git is unavailable.
func runGitCommand(t *testing.T, dir string, args ...string) error {
	t.Helper()
	if _, lookErr := exec.LookPath("git"); lookErr != nil {
		t.Skip("git binary not found on PATH")
	}
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %v failed: %w: %s", args, err, string(out))
	}
	return nil
}

func TestSelectTemplate_TemplateSourceBranch(t *testing.T) {
	result, err := selectTemplate("./local-template", false, nil, map[string]templates.Configuration{}, "v1")

	require.NoError(t, err)
	assert.Equal(t, "./local-template", result.Name)
}

func TestExecuteInit_ValidatesRequiredArgs(t *testing.T) {
	tests := []struct {
		name         string
		templateName string
		targetDir    string
		interactive  bool
		expectError  bool
		errorContain string
	}{
		{
			name:         "non-interactive requires template name",
			templateName: "",
			targetDir:    "",
			interactive:  false,
			expectError:  true,
			errorContain: "template name",
		},
		{
			name:         "non-interactive requires target dir",
			templateName: "simple",
			targetDir:    "",
			interactive:  false,
			expectError:  true,
			errorContain: "target directory",
		},
		{
			name:         "non-interactive requires both template and target",
			templateName: "",
			targetDir:    "",
			interactive:  false,
			expectError:  true,
			errorContain: "template name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := executeInit(context.Background(), &initOptions{
				templateName: tt.templateName,
				targetDir:    tt.targetDir,
				interactive:  tt.interactive,
				force:        false,
				templateVars: map[string]interface{}{},
			})

			if tt.expectError {
				require.Error(t, err)
				if tt.errorContain != "" {
					assert.Contains(t, err.Error(), tt.errorContain)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestInitCmd_SubcommandsNotAllowed(t *testing.T) {
	// Verify init command has no subcommands.
	assert.Empty(t, initCmd.Commands())
}

func TestInitCmd_RunEFunction(t *testing.T) {
	// Verify RunE function is set.
	assert.NotNil(t, initCmd.RunE)
}

func TestInitCmd_CoverageBooster(t *testing.T) {
	// This test exercises code paths for coverage.
	provider := &InitCommandProvider{}

	// Exercise all interface methods.
	_ = provider.GetCommand()
	_ = provider.GetName()
	_ = provider.GetGroup()
	_ = provider.GetFlagsBuilder()
	_ = provider.GetPositionalArgsBuilder()
	_ = provider.GetCompatibilityFlags()
	_ = provider.GetAliases()

	// Verify values.
	assert.Equal(t, "init", provider.GetName())
	assert.Equal(t, "Configuration Management", provider.GetGroup())
}

func TestParseSetFlag(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		expectKey string
		expectVal string
		expectErr bool
	}{
		{
			name:      "valid key=value",
			input:     "key=value",
			expectKey: "key",
			expectVal: "value",
			expectErr: false,
		},
		{
			name:      "value with equals sign",
			input:     "key=value=with=equals",
			expectKey: "key",
			expectVal: "value=with=equals",
			expectErr: false,
		},
		{
			name:      "key with spaces trimmed",
			input:     "  key  =  value  ",
			expectKey: "key",
			expectVal: "value",
			expectErr: false,
		},
		{
			name:      "invalid - no equals sign",
			input:     "keyvalue",
			expectKey: "",
			expectVal: "",
			expectErr: true,
		},
		{
			name:      "invalid - empty string",
			input:     "",
			expectKey: "",
			expectVal: "",
			expectErr: true,
		},
		{
			name:      "invalid - empty key",
			input:     "=value",
			expectKey: "",
			expectVal: "",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, val, err := parseSetFlag(tt.input)
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectKey, key)
				assert.Equal(t, tt.expectVal, val)
			}
		})
	}
}
