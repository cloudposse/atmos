package ci

import (
	"bytes"
	"errors"
	stdio "io"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	cipkg "github.com/cloudposse/atmos/pkg/ci"
	githubprovider "github.com/cloudposse/atmos/pkg/ci/providers/github"
	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/validation"
)

type ciValidationTestStreams struct{ output *bytes.Buffer }

func (s ciValidationTestStreams) Input() stdio.Reader     { return bytes.NewReader(nil) }
func (s ciValidationTestStreams) Output() stdio.Writer    { return s.output }
func (s ciValidationTestStreams) Error() stdio.Writer     { return s.output }
func (s ciValidationTestStreams) RawOutput() stdio.Writer { return s.output }
func (s ciValidationTestStreams) RawError() stdio.Writer  { return s.output }

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

func TestWorkflowFilesExcluding(t *testing.T) {
	root := t.TempDir()
	workflowDir := filepath.Join(root, ".github", "workflows")
	require.NoError(t, os.MkdirAll(workflowDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(workflowDir, "keep.yml"), []byte("name: keep\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(workflowDir, "skip.yaml"), []byte("name: skip\n"), 0o600))

	files, err := workflowFilesExcluding(root, workflowDir, []string{".github/workflows/skip.yaml"})
	require.NoError(t, err)
	assert.Equal(t, []string{".github/workflows/keep.yml"}, files)

	t.Chdir(root)
	files, err = workflowFilesExcluding(root, filepath.Join(".github", "workflows"), []string{".github/workflows/skip.yaml"})
	require.NoError(t, err)
	assert.Equal(t, []string{".github/workflows/keep.yml"}, files)
}

func TestWriteCIValidationOutputFormats(t *testing.T) {
	command := NewValidateCommand()
	var richOutput bytes.Buffer
	command.SetOut(&richOutput)
	require.NoError(t, writeCIValidationOutput(command, validateFormatRich, validation.Report{FilesChecked: 2}, t.TempDir(), false, ""))
	assert.Contains(t, richOutput.String(), "No GitHub Actions workflow validation findings")

	richOutput.Reset()
	require.NoError(t, writeCIValidationOutput(command, validateFormatRich, validation.Report{Diagnostics: []validation.Diagnostic{{
		Source: "github-actions", RuleID: "syntax", Severity: validation.SeverityError, Message: "invalid workflow",
	}}}, t.TempDir(), false, ""))
	assert.Contains(t, richOutput.String(), "invalid workflow")

	// Text diagnostics are returned by workflowValidationError, so the output
	// phase intentionally has no direct side effect when findings exist.
	require.NoError(t, writeCIValidationOutput(command, validateFormatText, validation.Report{Diagnostics: []validation.Diagnostic{{Message: "invalid"}}}, "", false, ""))

	dataOutput := &bytes.Buffer{}
	context, err := iolib.NewContext(iolib.WithStreams(ciValidationTestStreams{output: dataOutput}))
	require.NoError(t, err)
	data.InitWriter(context)
	t.Cleanup(data.Reset)
	require.NoError(t, writeCIValidationOutput(command, validateFormatSARIF, validation.Report{Diagnostics: []validation.Diagnostic{{
		Source: "github-actions", RuleID: "syntax", Severity: validation.SeverityError, Message: "invalid workflow",
	}}}, "", false, ""))
	assert.Contains(t, dataOutput.String(), `"version": "2.1.0"`)
}

func TestRunCIValidateDefaultWorkflowDirectory(t *testing.T) {
	project := t.TempDir()
	workflows := filepath.Join(project, ".github", "workflows")
	require.NoError(t, os.MkdirAll(workflows, 0o700))
	t.Chdir(project)

	command := NewValidateCommand()
	require.NoError(t, command.Flags().Set("format", validateFormatText))
	require.NoError(t, os.WriteFile(filepath.Join(workflows, "valid.yaml"), []byte("name: valid\non: push\njobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo valid\n"), 0o600))
	require.NoError(t, runCIValidate(command, nil))

	require.NoError(t, os.WriteFile(filepath.Join(workflows, "invalid.yaml"), []byte("name: invalid\non: push\njobs:\n  test:\n    runs-on: 42\n"), 0o600))
	err := runCIValidate(command, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, errWorkflowValidationFailed)
	assert.Equal(t, 1, errUtils.GetExitCode(err))
}
