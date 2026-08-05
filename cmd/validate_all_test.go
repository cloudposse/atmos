package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestRunValidationTasksContinuesAndSummarizes(t *testing.T) {
	_ = NewTestKit(t)

	firstErr := errors.New("first failure")
	secondErr := errors.New("second failure")
	order := make([]string, 0, 4)

	results, err := runValidationTasks([]validationTask{
		{
			name: "schema",
			applicable: func() (bool, error) {
				return true, nil
			},
			run: func() error {
				order = append(order, "schema")
				return firstErr
			},
		},
		{
			name: "stacks",
			applicable: func() (bool, error) {
				return false, nil
			},
			run: func() error {
				t.Fatal("skipped validation task must not run")
				return nil
			},
		},
		{
			name: "editorconfig",
			applicable: func() (bool, error) {
				return true, nil
			},
			run: func() error {
				order = append(order, "editorconfig")
				return nil
			},
		},
		{
			name: "ci",
			applicable: func() (bool, error) {
				return true, nil
			},
			run: func() error {
				order = append(order, "ci")
				return secondErr
			},
		},
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrValidationFailed)
	assert.ErrorIs(t, err, firstErr)
	assert.ErrorIs(t, err, secondErr)
	assert.Equal(t, 1, errUtils.GetExitCode(err))
	assert.Equal(t, []string{"schema", "editorconfig", "ci"}, order)
	assert.Equal(t, []validationTaskResult{
		{name: "schema", status: validationTaskFailed},
		{name: "stacks", status: validationTaskSkipped},
		{name: "editorconfig", status: validationTaskPassed},
		{name: "ci", status: validationTaskFailed},
	}, results)
	assert.Equal(t, "Validation summary:\n  schema: failed\n  stacks: skipped\n  editorconfig: passed\n  ci: failed", formatValidationSummary(results))
	assert.Equal(t, "## Atmos validation\n\n| Validator | Result |\n| --- | --- |\n| schema | ❌ failed |\n| stacks | ⏭️ skipped |\n| editorconfig | ✅ passed |\n| ci | ❌ failed |\n", formatValidationSummaryMarkdown(results))
}

func TestWriteValidationCISummary(t *testing.T) {
	originalConfig := atmosConfig
	originalWriter := validationCISummaryWriter
	t.Cleanup(func() {
		atmosConfig = originalConfig
		validationCISummaryWriter = originalWriter
	})

	atmosConfig = schema.AtmosConfiguration{CI: schema.CIConfig{Enabled: true}}
	var got string
	validationCISummaryWriter = func(summary string) error {
		got = summary
		return nil
	}
	writeValidationCISummary([]validationTaskResult{{name: "schema", status: validationTaskPassed}})
	assert.Equal(t, "## Atmos validation\n\n| Validator | Result |\n| --- | --- |\n| schema | ✅ passed |\n", got)

	atmosConfig.CI.Enabled = false
	got = ""
	writeValidationCISummary([]validationTaskResult{{name: "schema", status: validationTaskPassed}})
	assert.Empty(t, got)
}

func TestValidationApplicability(t *testing.T) {
	_ = NewTestKit(t)

	originalAtmosConfig := atmosConfig
	t.Cleanup(func() {
		atmosConfig = originalAtmosConfig
	})

	projectDir := t.TempDir()
	t.Chdir(projectDir)
	atmosConfig = schema.AtmosConfiguration{}

	t.Run("skips absent optional project validators", func(t *testing.T) {
		editorConfigApplicable, err := editorConfigValidationApplicable()
		require.NoError(t, err)
		assert.False(t, editorConfigApplicable)

		githubActionsApplicable, err := githubActionsValidationApplicable()
		require.NoError(t, err)
		assert.False(t, githubActionsApplicable)
	})

	t.Run("runs optional validators when their inputs exist", func(t *testing.T) {
		require.NoError(t, os.WriteFile(".editorconfig", []byte("root = true\n"), 0o600))
		require.NoError(t, os.MkdirAll(filepath.Join(".github", "workflows"), 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(".github", "workflows", "validate.yaml"), []byte("name: validate\n"), 0o600))

		editorConfigApplicable, err := editorConfigValidationApplicable()
		require.NoError(t, err)
		assert.True(t, editorConfigApplicable)

		githubActionsApplicable, err := githubActionsValidationApplicable()
		require.NoError(t, err)
		assert.True(t, githubActionsApplicable)
	})
}

func TestValidationTaskErrorsAndApplicabilityFailures(t *testing.T) {
	results, err := runValidationTasks([]validationTask{{
		name: "schema",
		applicable: func() (bool, error) {
			return false, errors.New("cannot inspect schema")
		},
	}, {
		name:       "stacks",
		applicable: func() (bool, error) { return true, nil },
		run:        func() error { return nil },
	}})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrValidationFailed)
	assert.Equal(t, []validationTaskResult{
		{name: "schema", status: validationTaskFailed},
		{name: "stacks", status: validationTaskPassed},
	}, results)

	original := atmosConfig
	t.Cleanup(func() { atmosConfig = original })
	project := t.TempDir()
	t.Chdir(project)
	atmosConfig = schema.AtmosConfiguration{Validate: schema.Validate{EditorConfig: schema.EditorConfig{ConfigFilePaths: []string{"\x00"}}}}
	_, err = editorConfigValidationApplicable()
	require.Error(t, err)

	require.NoError(t, os.WriteFile(".github", []byte("not a directory"), 0o600))
	_, err = githubActionsValidationApplicable()
	require.Error(t, err)
	assert.Contains(t, err.Error(), ".github is not a directory")
}

func TestValidateCommandHelpDescribesAggregateValidation(t *testing.T) {
	_ = NewTestKit(t)

	// Do not call validateCmd.Help() here: the inherited rootHelpFunc treats a
	// help invocation without --help in os.Args as an incorrect usage and
	// exits the process, which would kill the whole test binary. The command's
	// Long text is what the help renderer displays, so assert on it directly.
	assert.NotNil(t, validateCmd.RunE)
	assert.Contains(t, validateCmd.Long, "Without a subcommand")
	assert.Contains(t, validateCmd.Long, "EditorConfig")
	assert.Contains(t, validateCmd.Long, "GitHub Actions workflows")
}

func TestValidateCommandRunsAllApplicableProjectValidators(t *testing.T) {
	_ = NewTestKit(t)

	originalAtmosConfig := atmosConfig
	t.Cleanup(func() {
		atmosConfig = originalAtmosConfig
	})

	projectDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, "stacks"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "atmos.yaml"), []byte("base_path: .\nstacks:\n  base_path: stacks\n  included_paths:\n    - \"**/*\"\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "stacks", "dev.yaml"), []byte("vars:\n  stage: dev\n"), 0o600))
	t.Chdir(projectDir)

	var err error
	atmosConfig, err = cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)
	require.NoError(t, validateCmd.RunE(validateCmd, nil))
}
