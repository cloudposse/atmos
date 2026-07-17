package ci

import (
	"errors"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	cipkg "github.com/cloudposse/atmos/pkg/ci"
	githubprovider "github.com/cloudposse/atmos/pkg/ci/providers/github"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/validation"
)

func TestRunCIValidateRejectsUnknownFormat(t *testing.T) {
	command := &cobra.Command{}
	command.Flags().String("format", "json", "")
	err := runCIValidate(command, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected text, rich, or sarif")
}

func TestNewValidateCommandAdvertisesRichFormat(t *testing.T) {
	command := NewValidateCommand()
	format := command.Flags().Lookup("format")
	require.NotNil(t, format)
	assert.Contains(t, format.Usage, "rich")
}

func TestValidationSuccessMessage(t *testing.T) {
	repositoryMessage := validationSuccessMessage(validation.Report{
		FilesChecked: 2,
	}, false, "")
	assert.Contains(t, repositoryMessage, ".github/workflows")
	assert.NotContains(t, repositoryMessage, "/repo")

	explicitMessage := validationSuccessMessage(validation.Report{FilesChecked: 1}, true, "")
	assert.Equal(t, "Validated **1** GitHub Actions workflow file(s).", explicitMessage)

	workflowPathMessage := validationSuccessMessage(validation.Report{FilesChecked: 1}, false, "tests/fixtures/scenarios/invalid-github-actions-workflows")
	assert.Contains(t, workflowPathMessage, "tests/fixtures/scenarios/invalid-github-actions-workflows")
}

func TestRunCIValidateRejectsWorkflowPathWithWorkflowFiles(t *testing.T) {
	command := NewValidateCommand()
	require.NoError(t, command.Flags().Set("workflow-path", "fixtures/workflows"))

	err := runCIValidate(command, []string{"workflow.yml"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be used with --workflow-path")
}

func TestWorkflowValidationErrorOwnsDiagnostics(t *testing.T) {
	validationErr := workflowValidationError(validation.Report{Diagnostics: []validation.Diagnostic{{
		File:    "invalid.yml",
		Line:    5,
		Column:  5,
		RuleID:  "syntax-check",
		Message: "unexpected key \"branch\"",
	}}, RenderedDiagnostics: "actionlint-style diagnostic"})

	require.Error(t, validationErr)
	assert.ErrorIs(t, validationErr, errWorkflowValidationFailed)
	assert.Equal(t, 1, errUtils.GetExitCode(validationErr))

	rendered := errUtils.Format(validationErr, errUtils.DefaultFormatterConfig())
	assert.Contains(t, rendered, "GitHub Actions workflow validation failed")
	assert.Contains(t, rendered, "actionlint-style diagnostic")
}

func TestActionlintOutputUsesNativeDiagnostics(t *testing.T) {
	report := validation.Report{
		Diagnostics: []validation.Diagnostic{{Message: "fallback"}},
		RenderedDiagnostics: `invalid.yml:5:5: unexpected key "branch" [syntax-check]
  |
5 |     branch: main
  |     ^~~~~~~`,
	}

	assert.Equal(t, report.RenderedDiagnostics, actionlintOutput(report))

	report.RenderedDiagnostics = ""
	assert.Equal(t, ":0: fallback []", actionlintOutput(report))
}

func TestCIValidationAnnotationsEnabled(t *testing.T) {
	restoreRegistry := cipkg.SwapRegistryForTest()
	defer restoreRegistry()
	previousLoader := ciValidationConfigLoader
	defer func() { ciValidationConfigLoader = previousLoader }()

	t.Setenv("GITHUB_ACTIONS", "true")
	cipkg.Register(githubprovider.NewProvider())
	ciValidationConfigLoader = func(*cobra.Command) (*schema.AtmosConfiguration, error) {
		return &schema.AtmosConfiguration{CI: schema.CIConfig{Enabled: true}}, nil
	}
	assert.True(t, ciValidationAnnotationsEnabled(&cobra.Command{}))

	disabled := false
	ciValidationConfigLoader = func(*cobra.Command) (*schema.AtmosConfiguration, error) {
		return &schema.AtmosConfiguration{CI: schema.CIConfig{
			Enabled:     true,
			Annotations: schema.CIAnnotationsConfig{Enabled: &disabled},
		}}, nil
	}
	assert.False(t, ciValidationAnnotationsEnabled(&cobra.Command{}))

	ciValidationConfigLoader = func(*cobra.Command) (*schema.AtmosConfiguration, error) {
		return nil, errors.New("invalid config")
	}
	assert.False(t, ciValidationAnnotationsEnabled(&cobra.Command{}))
}
