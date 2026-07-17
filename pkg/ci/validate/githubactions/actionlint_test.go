package githubactions

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	civalidate "github.com/cloudposse/atmos/pkg/ci/validate"
)

const validWorkflow = `name: Test
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - run: echo ok
`

const invalidWorkflow = `name: Test
on:
  push:
    branch: main
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - run: echo ok
`

func TestValidatorValidateRepository(t *testing.T) {
	root := actionlintTestRepository(t)
	writeWorkflow(t, root, "valid.yml", validWorkflow)
	writeWorkflow(t, root, "invalid.yml", invalidWorkflow)

	report, err := (Validator{}).Validate(context.Background(), civalidate.Request{Root: root})
	require.NoError(t, err)
	require.Len(t, report.Diagnostics, 1)
	assert.Equal(t, 2, report.FilesChecked)
	assert.Equal(t, filepath.Join(root, ".github", "workflows"), report.Target)
	diagnostic := report.Diagnostics[0]
	assert.Equal(t, ValidatorName, diagnostic.Source)
	assert.Equal(t, "syntax-check", diagnostic.RuleID)
	assert.Equal(t, ".github/workflows/invalid.yml", diagnostic.File)
	assert.Equal(t, 4, diagnostic.Line)
	assert.Positive(t, diagnostic.Column)
	assert.True(t, report.HasErrors())
	assert.Contains(t, report.RenderedDiagnostics, "unexpected key \"branch\"")
	assert.Contains(t, report.RenderedDiagnostics, "[syntax-check]")
}

func TestValidatorValidateExplicitFiles(t *testing.T) {
	root := actionlintTestRepository(t)
	validPath := writeWorkflow(t, root, "valid.yml", validWorkflow)
	invalidPath := writeWorkflow(t, root, "invalid.yml", invalidWorkflow)

	report, err := (Validator{}).Validate(context.Background(), civalidate.Request{
		Root:  root,
		Paths: []string{validPath},
	})
	require.NoError(t, err)
	assert.Empty(t, report.Diagnostics)
	assert.Equal(t, 1, report.FilesChecked)

	report, err = (Validator{}).Validate(context.Background(), civalidate.Request{
		Root:  root,
		Paths: []string{invalidPath},
	})
	require.NoError(t, err)
	require.Len(t, report.Diagnostics, 1)
}

func TestValidatorValidateWorkflowPath(t *testing.T) {
	root := actionlintTestRepository(t)
	workflowPath := filepath.Join(root, "fixtures", "invalid-workflows")
	require.NoError(t, os.MkdirAll(workflowPath, 0o755))
	writeWorkflowInDirectory(t, workflowPath, "invalid.yml", invalidWorkflow)

	report, err := (Validator{}).Validate(context.Background(), civalidate.Request{
		Root:         root,
		WorkflowPath: workflowPath,
	})
	require.NoError(t, err)
	assert.Equal(t, workflowPath, report.Target)
	assert.Equal(t, 1, report.FilesChecked)
	require.Len(t, report.Diagnostics, 1)
	assert.Equal(t, "syntax-check", report.Diagnostics[0].RuleID)
}

func TestValidatorRespectsRepositoryConfig(t *testing.T) {
	root := actionlintTestRepository(t)
	writeWorkflow(t, root, "ignored.yml", invalidWorkflow)
	require.NoError(t, os.WriteFile(filepath.Join(root, ".github", "actionlint.yaml"), []byte(`paths:
  .github/workflows/ignored.yml:
    ignore:
      - 'unexpected key "branch"'
`), 0o600))

	report, err := (Validator{}).Validate(context.Background(), civalidate.Request{Root: root})
	require.NoError(t, err)
	assert.Empty(t, report.Diagnostics)
}

func TestValidatorRequiresWorkflowDirectory(t *testing.T) {
	root := t.TempDir()

	_, err := (Validator{}).Validate(context.Background(), civalidate.Request{Root: root})
	require.Error(t, err)
	assert.Contains(t, err.Error(), ".github/workflows")
}

func actionlintTestRepository(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(root, ".git"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".github", "workflows"), 0o755))
	return root
}

func writeWorkflow(t *testing.T, root, name, workflow string) string {
	t.Helper()
	return writeWorkflowInDirectory(t, filepath.Join(root, ".github", "workflows"), name, workflow)
}

func writeWorkflowInDirectory(t *testing.T, directory, name, workflow string) string {
	t.Helper()
	path := filepath.Join(directory, name)
	require.NoError(t, os.WriteFile(path, []byte(workflow), 0o600))
	return path
}
