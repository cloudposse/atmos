package githubactions

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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
	assert.Contains(t, err.Error(), filepath.Join(".github", "workflows"))
}

func TestRepositoryPath(t *testing.T) {
	assert.Equal(t, ".github/workflows/invalid.yml", repositoryPath(".github\\workflows\\invalid.yml"))
	assert.Equal(t, ".github/workflows/invalid.yml", repositoryPath(".github/workflows/invalid.yml"))
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

func TestValidatorSelfReferences(t *testing.T) {
	root := actionlintTestRepository(t)
	actionDir := filepath.Join(root, ".github", "actions", "example")
	require.NoError(t, os.MkdirAll(actionDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(actionDir, "action.yml"), []byte(`name: Example
description: Example action
inputs:
  message:
    required: true
    description: Message
runs:
  using: composite
  steps:
    - uses: $/.github/actions/nested
`), 0o600))
	workflow := `name: Test
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: "$/.github/actions/example"
        with:
          message: hello
      - run: echo '$/this-is-shell-text'
`
	path := writeWorkflow(t, root, "self.yml", workflow)
	report, err := (Validator{}).Validate(context.Background(), civalidate.Request{Root: root})
	require.NoError(t, err)
	assert.Empty(t, report.Diagnostics)
	unchanged, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, workflow, string(unchanged))

	// Input validation must still run for the referenced action, with source
	// positions and diagnostics referring to the original $/ spelling.
	writeWorkflow(t, root, "self.yml", strings.Replace(workflow, "message: hello", "unknown: hello", 1))
	report, err = (Validator{}).Validate(context.Background(), civalidate.Request{Root: root})
	require.NoError(t, err)
	assert.True(t, report.HasErrors())
	assert.Contains(t, report.RenderedDiagnostics, "unknown")
}

func TestValidatorRejectsInvalidSelfReferences(t *testing.T) {
	for _, reference := range []string{"$/../outside", "$//absolute", "$/.github/actions/example@main"} {
		t.Run(reference, func(t *testing.T) {
			root := actionlintTestRepository(t)
			workflow := strings.Replace(validWorkflow, "run: echo ok", "uses: "+reference, 1)
			writeWorkflow(t, root, "invalid.yml", workflow)
			report, err := (Validator{}).Validate(context.Background(), civalidate.Request{Root: root})
			require.NoError(t, err)
			assert.True(t, report.HasErrors())
		})
	}
}

func TestValidatorSelfReferencedWorkflow(t *testing.T) {
	root := actionlintTestRepository(t)
	writeWorkflow(t, root, "child.yml", `on: workflow_call
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - run: echo ok
`)
	writeWorkflow(t, root, "caller.yml", `on: push
jobs:
  call:
    uses: $/.github/workflows/child.yml
`)
	report, err := (Validator{}).Validate(context.Background(), civalidate.Request{Root: root})
	require.NoError(t, err)
	assert.Empty(t, report.Diagnostics)
}

func TestValidatorCacheModes(t *testing.T) {
	for _, mode := range []string{"read", "write", "write-only", "none"} {
		t.Run(mode, func(t *testing.T) {
			root := actionlintTestRepository(t)
			workflow := strings.Replace(validWorkflow, "on: push", "on: push\ncache-mode: "+mode, 1)
			workflow = strings.Replace(workflow, "    runs-on:", "    cache-mode: read\n    runs-on:", 1)
			writeWorkflow(t, root, "cache.yml", workflow)
			report, err := (Validator{}).Validate(context.Background(), civalidate.Request{Root: root})
			require.NoError(t, err)
			assert.Empty(t, report.Diagnostics)
			assert.Empty(t, report.RenderedDiagnostics)
		})
	}
}

func TestValidatorInvalidCacheModes(t *testing.T) {
	for _, mode := range []string{"read-write", "true", "[read]", "${{ github.ref }}"} {
		t.Run(mode, func(t *testing.T) {
			root := actionlintTestRepository(t)
			writeWorkflow(t, root, "cache.yml", strings.Replace(validWorkflow, "on: push", "on: push\ncache-mode: "+mode, 1))
			report, err := (Validator{}).Validate(context.Background(), civalidate.Request{Root: root})
			require.NoError(t, err)
			require.Len(t, report.Diagnostics, 1)
			assert.Equal(t, 3, report.Diagnostics[0].Line)
			assert.Contains(t, report.RenderedDiagnostics, "cache-mode must be one of")
		})
	}
}

func TestValidatorCacheModePreservesOtherErrors(t *testing.T) {
	root := actionlintTestRepository(t)
	writeWorkflow(t, root, "cache.yml", strings.Replace(invalidWorkflow, "name: Test", "name: Test\ncache-mode: read", 1))
	report, err := (Validator{}).Validate(context.Background(), civalidate.Request{Root: root})
	require.NoError(t, err)
	require.Len(t, report.Diagnostics, 1)
	assert.Contains(t, report.RenderedDiagnostics, `unexpected key "branch"`)
	assert.NotContains(t, report.RenderedDiagnostics, `unexpected key "cache-mode"`)

	// A valid value in the wrong location is still a syntax error.
	writeWorkflow(t, root, "cache.yml", strings.Replace(validWorkflow, "      - run: echo ok", "      - run: echo ok\n        cache-mode: read", 1))
	report, err = (Validator{}).Validate(context.Background(), civalidate.Request{Root: root})
	require.NoError(t, err)
	assert.True(t, report.HasErrors())
}
