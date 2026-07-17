package cmd

import (
	"bytes"
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
}

func TestValidationApplicability(t *testing.T) {
	_ = NewTestKit(t)

	originalWorkingDirectory, err := os.Getwd()
	require.NoError(t, err)
	originalAtmosConfig := atmosConfig
	t.Cleanup(func() {
		require.NoError(t, os.Chdir(originalWorkingDirectory))
		atmosConfig = originalAtmosConfig
	})

	projectDir := t.TempDir()
	require.NoError(t, os.Chdir(projectDir))
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

func TestValidateCommandHelpDescribesAggregateValidation(t *testing.T) {
	_ = NewTestKit(t)

	var output bytes.Buffer
	validateCmd.SetOut(&output)
	validateCmd.SetErr(&output)
	require.NoError(t, validateCmd.Help())

	assert.NotNil(t, validateCmd.RunE)
	assert.Contains(t, output.String(), "Without a subcommand")
	assert.Contains(t, output.String(), "EditorConfig")
	assert.Contains(t, output.String(), "GitHub Actions workflows")
}

func TestValidateCommandRunsAllApplicableProjectValidators(t *testing.T) {
	_ = NewTestKit(t)

	originalWorkingDirectory, err := os.Getwd()
	require.NoError(t, err)
	originalAtmosConfig := atmosConfig
	t.Cleanup(func() {
		require.NoError(t, os.Chdir(originalWorkingDirectory))
		atmosConfig = originalAtmosConfig
	})

	projectDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, "stacks"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "atmos.yaml"), []byte("base_path: .\nstacks:\n  base_path: stacks\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "stacks", "dev.yaml"), []byte("vars:\n  stage: dev\n"), 0o600))
	require.NoError(t, os.Chdir(projectDir))

	atmosConfig, err = cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)
	require.NoError(t, validateCmd.RunE(validateCmd, nil))
}
