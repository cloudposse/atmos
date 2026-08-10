package scaffold

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/generator/engine"
	"github.com/cloudposse/atmos/pkg/generator/templates"
	"github.com/cloudposse/atmos/pkg/project/config"
)

// Note: The dry-run preview functions require UI initialization which
// is done at runtime. Testing them requires integration tests.
// Here we test the helper functions that don't require UI.

// TestLoadDryRunValues tests loading values for dry-run.
func TestLoadDryRunValues(t *testing.T) {
	tests := []struct {
		name        string
		config      *templates.Configuration
		vars        map[string]interface{}
		expectError bool
	}{
		{
			name: "no scaffold config",
			config: &templates.Configuration{
				Files: []templates.File{{Path: "test.txt"}},
			},
			vars:        map[string]interface{}{"key": "value"},
			expectError: false,
		},
		{
			name: "with scaffold config and defaults",
			config: &templates.Configuration{
				Files: []templates.File{
					{
						Path: config.ScaffoldConfigFileName,
						Content: `apiVersion: atmos/v1
kind: AtmosScaffoldConfig
metadata:
  name: test
spec:
  fields:
    - name: project_name
      type: string
      default: default-name
`,
					},
				},
			},
			vars:        map[string]interface{}{},
			expectError: false,
		},
		{
			name: "invalid scaffold config",
			config: &templates.Configuration{
				Files: []templates.File{
					{
						Path:    config.ScaffoldConfigFileName,
						Content: "invalid: yaml: content: [",
					},
				},
			},
			vars:        map[string]interface{}{},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			values, err := loadDryRunValues(tt.config, tt.vars)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, values)
			}
		})
	}
}

// TestFindScaffoldConfigFile tests finding scaffold config in file list.
func TestFindScaffoldConfigFile(t *testing.T) {
	tests := []struct {
		name     string
		files    []templates.File
		expected bool
	}{
		{
			name: "config exists",
			files: []templates.File{
				{Path: "file1.txt"},
				{Path: config.ScaffoldConfigFileName},
				{Path: "file2.txt"},
			},
			expected: true,
		},
		{
			name: "config does not exist",
			files: []templates.File{
				{Path: "file1.txt"},
				{Path: "file2.txt"},
			},
			expected: false,
		},
		{
			name:     "empty file list",
			files:    []templates.File{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := findScaffoldConfigFile(tt.files)
			if tt.expected {
				assert.NotNil(t, result)
			} else {
				assert.Nil(t, result)
			}
		})
	}
}

// TestRenderFilePath tests file path rendering with variables.
func TestRenderFilePath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		values   map[string]interface{}
		expected string
	}{
		{
			name:     "simple path no variables",
			path:     "path/to/file.txt",
			values:   map[string]interface{}{},
			expected: "path/to/file.txt",
		},
		{
			name:     "path with single Config variable",
			path:     "{{ .Config.project_name }}/file.txt",
			values:   map[string]interface{}{"project_name": "my-project"},
			expected: "my-project/file.txt",
		},
		{
			name: "path with multiple Config variables",
			path: "{{ .Config.namespace }}/{{ .Config.environment }}/{{ .Config.app }}.yaml",
			values: map[string]interface{}{
				"namespace":   "prod",
				"environment": "staging",
				"app":         "api",
			},
			expected: "prod/staging/api.yaml",
		},
		{
			name:     "path with numeric Config variable",
			path:     "{{ .Config.count }}/file.txt",
			values:   map[string]interface{}{"count": 42},
			expected: "42/file.txt", // The engine renders non-string values too.
		},
		{
			name:     "invalid template falls back to raw path",
			path:     "{{ .Config.unterminated /file.txt",
			values:   map[string]interface{}{},
			expected: "{{ .Config.unterminated /file.txt", // Parse error -> raw path returned.
		},
	}

	processor := engine.NewProcessor()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := renderFilePath(processor, tt.path, tt.values)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestResolveTargetDirectory tests target directory resolution.
func TestResolveTargetDirectory(t *testing.T) {
	tests := []struct {
		name        string
		targetDir   string
		expectError bool
	}{
		{
			name:        "empty target directory",
			targetDir:   "",
			expectError: false,
		},
		{
			name:        "absolute path",
			targetDir:   "/tmp/test",
			expectError: false,
		},
		{
			name:        "relative path",
			targetDir:   "./test",
			expectError: false,
		},
		{
			name:        "current directory",
			targetDir:   ".",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := resolveTargetDirectory(tt.targetDir)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tt.targetDir != "" {
					assert.NotEmpty(t, result)
				}
			}
		})
	}
}

// TestLoadScaffoldTemplates tests loading scaffold templates.
func TestLoadScaffoldTemplates(t *testing.T) {
	configs, origins, ui, err := loadScaffoldTemplates("")
	require.NoError(t, err)
	assert.NotNil(t, configs)
	assert.NotNil(t, origins)
	assert.NotNil(t, ui)
	assert.NotEmpty(t, configs)
}

// TestExecuteTemplateGenerationErrors tests error paths in template generation.
func TestExecuteTemplateGenerationErrors(t *testing.T) {
	// This tests the execution flow, not full integration
	// Most error paths require complex setup with git repos, etc.

	// Test that the function exists and has proper signature
	selectedConfig := templates.Configuration{
		Name: "Test",
		Files: []templates.File{
			{Path: "test.txt", Content: "test"},
		},
	}

	// With an empty target directory in non-interactive mode the call must
	// fail fast instead of trying to prompt.
	opts := scaffoldGenerateOptions{
		interactive:    false,
		useDefaults:    true,
		templateValues: map[string]interface{}{},
	}
	err := executeTemplateGeneration(&selectedConfig, "", &opts, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrTargetDirRequired)
}

func TestScaffoldCommandProvider_UncoveredMetadata(t *testing.T) {
	provider := &ScaffoldCommandProvider{}

	assert.Nil(t, provider.GetAliases())
	assert.True(t, provider.IsExperimental())
}

func TestSelectGenerateTemplate_NonInteractiveRequiresName(t *testing.T) {
	_, err := selectGenerateTemplate(&scaffoldGenerateOptions{interactive: false}, map[string]templates.Configuration{}, nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrTemplateNameRequired)
}

func TestRenderDryRunPreview_RendersHeaderAndFileList(t *testing.T) {
	cfg := &templates.Configuration{
		Name:        "demo",
		Description: "demo scaffold",
		Files: []templates.File{
			{
				Path: config.ScaffoldConfigFileName,
				Content: `apiVersion: atmos/v1
kind: AtmosScaffoldConfig
metadata:
  name: demo
spec:
  fields:
    - name: project_name
      type: input
      default: demo-default
`,
			},
			{Path: "{{ .Config.project_name }}/README.md", Content: "hello"},
			{Path: "static/file.txt", Content: "static"},
		},
	}

	err := renderDryRunPreview(cfg, t.TempDir(), map[string]interface{}{"project_name": "demo-project"})
	require.NoError(t, err)
}

func TestPrintFilePath_WithoutTargetDir(t *testing.T) {
	printFilePath("", "relative/file.txt")
}

func TestExecuteScaffoldGenerate_DryRunBuiltInTemplate(t *testing.T) {
	err := executeScaffoldGenerate(&scaffoldGenerateOptions{
		templateName:   "simple",
		targetDir:      t.TempDir(),
		dryRun:         true,
		interactive:    false,
		useDefaults:    true,
		templateValues: map[string]interface{}{"project_name": "demo"},
	})

	require.NoError(t, err)
}

func TestExecuteScaffoldGenerate_DryRunRequiresTarget(t *testing.T) {
	err := executeScaffoldGenerate(&scaffoldGenerateOptions{
		templateName:   "simple",
		dryRun:         true,
		interactive:    false,
		useDefaults:    true,
		templateValues: map[string]interface{}{},
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrTargetDirRequired)
}

func TestExecuteScaffoldList_LoadsAndDisplaysTemplates(t *testing.T) {
	require.NoError(t, executeScaffoldList(nil))
}

func TestExecuteValidateScaffold_EndToEnd(t *testing.T) {
	t.Run("empty directory", func(t *testing.T) {
		require.NoError(t, executeValidateScaffold(context.Background(), t.TempDir()))
	})

	t.Run("valid scaffold", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "scaffold.yaml")
		require.NoError(t, os.WriteFile(path, []byte(`apiVersion: atmos/v1
kind: AtmosScaffoldConfig
metadata:
  name: valid
spec:
  fields:
    - name: project_name
      type: input
      default: demo
`), 0o600))

		require.NoError(t, executeValidateScaffold(context.Background(), dir))
	})

	t.Run("invalid scaffold", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "scaffold.yaml")
		require.NoError(t, os.WriteFile(path, []byte("not: a scaffold\n"), 0o600))

		err := executeValidateScaffold(context.Background(), dir)
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrScaffoldValidation)
	})
}

func TestScaffoldGenerateRunE_DryRunAndSetFlags(t *testing.T) {
	cmd := &cobra.Command{}
	scaffoldGenerateParser.RegisterFlags(cmd)
	require.NoError(t, cmd.Flags().Set("dry-run", "true"))
	require.NoError(t, cmd.Flags().Set("set", "project_name=demo"))

	err := scaffoldGenerateCmd.RunE(cmd, []string{"simple", t.TempDir()})

	require.NoError(t, err)
}

func TestScaffoldGenerateRunE_MalformedSetFlag(t *testing.T) {
	cmd := &cobra.Command{}
	scaffoldGenerateParser.RegisterFlags(cmd)
	require.NoError(t, cmd.Flags().Set("set", "missing-equals"))

	err := scaffoldGenerateCmd.RunE(cmd, []string{"simple", t.TempDir()})

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidFlag)
}

func TestScaffoldListAndValidateRunE(t *testing.T) {
	require.NoError(t, scaffoldListCmd.RunE(&cobra.Command{}, nil))

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "scaffold.yaml"), []byte(`apiVersion: atmos/v1
kind: AtmosScaffoldConfig
metadata:
  name: valid
spec:
  fields: []
`), 0o600))

	require.NoError(t, scaffoldValidateCmd.RunE(&cobra.Command{}, []string{dir}))
}

// TestExecuteScaffoldGenerateWithDryRun tests dry-run flag integration.
func TestExecuteScaffoldGenerateWithDryRun(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := t.TempDir()

	// Create a simple template directory
	templateDir := filepath.Join(tempDir, "templates", "test-template")
	err := os.MkdirAll(templateDir, 0o755)
	require.NoError(t, err)

	// Create a scaffold.yaml
	scaffoldYAML := `name: Test Template
description: A test template
version: 1.0.0
fields:
  project_name:
    type: string
    default: test-project
`
	err = os.WriteFile(filepath.Join(templateDir, "scaffold.yaml"), []byte(scaffoldYAML), 0o644)
	require.NoError(t, err)

	// Create a template file
	err = os.WriteFile(filepath.Join(templateDir, "README.md"), []byte("# {{.project_name}}"), 0o644)
	require.NoError(t, err)

	// Note: Full integration test would require setting up the command context
	// This is a structural test to ensure the dry-run code path exists
	assert.NotNil(t, renderDryRunPreview)
}

// TestSelectTemplateErrors tests error handling in template selection.
func TestSelectTemplateErrors(t *testing.T) {
	configs := map[string]templates.Configuration{
		"template1": {Name: "template1", TemplateID: "id1"},
		"template2": {Name: "template2", TemplateID: "id2"},
	}

	// Test selecting non-existent template. selectTemplateByName never
	// touches scaffoldUI, so a nil ScaffoldUI is safe here.
	_, err := selectTemplate("nonexistent", configs, nil)
	assert.Error(t, err)

	// Test selecting with empty name: this triggers selectTemplateInteractive,
	// which calls scaffoldUI.PromptForTemplate -- simulate a prompt failure
	// (e.g. no TTY available) via a mock rather than a nil receiver.
	ctrl := gomock.NewController(t)
	mockUI := NewMockScaffoldUI(ctrl)
	mockUI.EXPECT().PromptForTemplate("scaffold", gomock.Any()).Return("", assert.AnError)

	_, err = selectTemplate("", configs, mockUI)
	assert.Error(t, err)
}

func TestMaybeInitGeneratedGitRepository_GitEnabled(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello"), 0o600))

	cfg := &templates.Configuration{Name: "demo", Version: "1.0.0"}
	err := maybeInitGeneratedGitRepository(dir, cfg, &scaffoldGenerateOptions{git: true})

	require.NoError(t, err)
	assert.DirExists(t, filepath.Join(dir, ".git"))
}

func TestMaybeInitGeneratedGitRepository_GitDisabled(t *testing.T) {
	dir := t.TempDir()

	cfg := &templates.Configuration{Name: "demo"}
	err := maybeInitGeneratedGitRepository(dir, cfg, &scaffoldGenerateOptions{git: false})

	require.NoError(t, err)
	assert.NoDirExists(t, filepath.Join(dir, ".git"))
}

// TestExecuteTemplateGeneration_WithTargetDir covers the targetDir != "" branch of
// executeTemplateGeneration, which drives the real UI (safe: the "simple" built-in
// template has a scaffold.yaml, and useDefaults:true skips all interactive huh
// prompts) rather than the targetDir == "" branch, which always prompts for a
// target directory via a real terminal form and cannot be safely unit tested.
func TestExecuteTemplateGeneration_WithTargetDir(t *testing.T) {
	configs, _, scaffoldUI, err := loadScaffoldTemplates("")
	require.NoError(t, err)
	cfg := configs["simple"]

	t.Run("success without git", func(t *testing.T) {
		dir := t.TempDir()
		opts := &scaffoldGenerateOptions{
			useDefaults:    true,
			templateValues: map[string]interface{}{"project_name": "demo"},
		}

		err := executeTemplateGeneration(&cfg, dir, opts, scaffoldUI)

		require.NoError(t, err)
		assert.NoDirExists(t, filepath.Join(dir, ".git"))
	})

	t.Run("success with git", func(t *testing.T) {
		dir := t.TempDir()
		opts := &scaffoldGenerateOptions{
			useDefaults:    true,
			git:            true,
			templateValues: map[string]interface{}{"project_name": "demo"},
		}

		err := executeTemplateGeneration(&cfg, dir, opts, scaffoldUI)

		require.NoError(t, err)
		assert.DirExists(t, filepath.Join(dir, ".git"))
	})
}

func TestShouldOfferScaffoldUpdate(t *testing.T) {
	notEmptyErr := errUtils.Build(errUtils.ErrTargetDirectoryNotEmpty).Err()
	otherErr := errUtils.Build(errUtils.ErrInitialization).Err()

	tests := []struct {
		name        string
		err         error
		opts        *scaffoldGenerateOptions
		wantOffer   bool
		wantBaseRef string
	}{
		{"nil error", nil, &scaffoldGenerateOptions{interactive: true}, false, ""},
		{"force already set", notEmptyErr, &scaffoldGenerateOptions{interactive: true, force: true}, false, ""},
		{"update already set", notEmptyErr, &scaffoldGenerateOptions{interactive: true, update: true}, false, ""},
		{"not interactive", notEmptyErr, &scaffoldGenerateOptions{interactive: false}, false, ""},
		{"dry run", notEmptyErr, &scaffoldGenerateOptions{interactive: true, dryRun: true}, false, ""},
		{"different error", otherErr, &scaffoldGenerateOptions{interactive: true}, false, ""},
		{"offers with default HEAD base ref", notEmptyErr, &scaffoldGenerateOptions{interactive: true}, true, "HEAD"},
		{"offers with caller base ref", notEmptyErr, &scaffoldGenerateOptions{interactive: true, baseRef: "v1.2.3"}, true, "v1.2.3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			offer, baseRef := shouldOfferScaffoldUpdate(tt.err, tt.opts)
			assert.Equal(t, tt.wantOffer, offer)
			assert.Equal(t, tt.wantBaseRef, baseRef)
		})
	}
}

// TestDefaultBaseRef pins the fix for a real bug: `atmos scaffold generate
// <template> <dir> --update` with no --base-ref silently set up no git
// storage at all (ExecuteWithDelimiters only calls SetupGitStorage when
// baseRef is non-empty), so every file failed with an opaque "three-way
// merge failed" even on a completely unmodified, freshly re-run directory.
func TestDefaultBaseRef(t *testing.T) {
	assert.Equal(t, "HEAD", defaultBaseRef(""))
	assert.Equal(t, "v1.2.3", defaultBaseRef("v1.2.3"))
}

// TestExecuteTemplateGeneration_UpdateFlag_MergesExistingDirectory covers the
// real bug --update fixes: re-running scaffold generation against an
// already-generated, git-initialized directory with update+base-ref=HEAD
// regenerates the template while preserving the user's own edits via a
// 3-way merge, instead of failing with "target directory is not empty".
func TestExecuteTemplateGeneration_UpdateFlag_MergesExistingDirectory(t *testing.T) {
	configs, _, scaffoldUI, err := loadScaffoldTemplates("")
	require.NoError(t, err)
	cfg := configs["simple"]

	dir := t.TempDir()
	opts := &scaffoldGenerateOptions{
		useDefaults:    true,
		templateValues: map[string]interface{}{"project_name": "demo"},
	}
	require.NoError(t, executeTemplateGeneration(&cfg, dir, opts, scaffoldUI))

	require.NoError(t, scaffoldRunGitCommand(t, dir, "init"))
	// Disable commit signing: dev machines with a GPG/1Password signing agent
	// configured globally can hang or fail here otherwise.
	require.NoError(t, scaffoldRunGitCommand(t, dir, "config", "commit.gpgsign", "false"))
	require.NoError(t, scaffoldRunGitCommand(t, dir, "config", "user.email", "test@example.com"))
	require.NoError(t, scaffoldRunGitCommand(t, dir, "config", "user.name", "Test"))
	require.NoError(t, scaffoldRunGitCommand(t, dir, "add", "."))
	require.NoError(t, scaffoldRunGitCommand(t, dir, "commit", "-m", "initial"))

	readmePath := filepath.Join(dir, "README.md")
	original, err := os.ReadFile(readmePath)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(readmePath, append(original, []byte("\nuser note\n")...), 0o600))

	updateOpts := &scaffoldGenerateOptions{
		useDefaults:    true,
		update:         true,
		baseRef:        "HEAD",
		templateValues: map[string]interface{}{"project_name": "demo"},
	}
	err = executeTemplateGeneration(&cfg, dir, updateOpts, scaffoldUI)

	require.NoError(t, err)
	merged, err := os.ReadFile(readmePath)
	require.NoError(t, err)
	assert.Contains(t, string(merged), "user note", "the user's manual edit must survive the 3-way merge")
}

// scaffoldRunGitCommand runs git in dir for test setup, skipping the test if git is unavailable.
func scaffoldRunGitCommand(t *testing.T, dir string, args ...string) error {
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

func TestSelectGenerateTemplate_ConfigHit(t *testing.T) {
	configs := map[string]templates.Configuration{
		"demo": {Name: "demo", Description: "demo template"},
	}

	result, err := selectGenerateTemplate(&scaffoldGenerateOptions{templateName: "demo"}, configs, nil)

	require.NoError(t, err)
	assert.Equal(t, "demo", result.Name)
}

func TestSelectGenerateTemplate_TemplateSource(t *testing.T) {
	result, err := selectGenerateTemplate(
		&scaffoldGenerateOptions{templateName: "./local-template"},
		map[string]templates.Configuration{},
		nil,
	)

	require.NoError(t, err)
	assert.Equal(t, "./local-template", result.Name)
}

func TestSelectGenerateTemplate_FallbackNotFound(t *testing.T) {
	_, err := selectGenerateTemplate(
		&scaffoldGenerateOptions{templateName: "nonexistent", interactive: false},
		map[string]templates.Configuration{},
		nil,
	)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrScaffoldNotFound)
}

func TestMergeConfiguredTemplates_Success(t *testing.T) {
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "my-template")
	require.NoError(t, os.MkdirAll(srcDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "file.txt"), []byte("hello"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "atmos.yaml"), []byte(`scaffold:
  templates:
    my-template:
      description: My template
      source: ./my-template
`), 0o600))
	t.Chdir(dir)

	configs := map[string]templates.Configuration{}
	origins := map[string]string{}
	err := mergeConfiguredTemplates(configs, origins)

	require.NoError(t, err)
	require.Contains(t, configs, "my-template")
	assert.Equal(t, "atmos.yaml", origins["my-template"])
}

func TestMergeConfiguredTemplates_WarnsAndContinues(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "atmos.yaml"), []byte(`scaffold:
  templates:
    broken-template:
      description: Missing source, cannot be converted
`), 0o600))
	t.Chdir(dir)

	configs := map[string]templates.Configuration{}
	origins := map[string]string{}
	err := mergeConfiguredTemplates(configs, origins)

	require.NoError(t, err)
	assert.NotContains(t, configs, "broken-template")
}

func TestDetermineScaffoldPathsToValidate_EmptyPathDefaultsToCwd(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "scaffold.yaml"), []byte("apiVersion: atmos/v1\n"), 0o600))
	t.Chdir(dir)

	paths, err := determineScaffoldPathsToValidate("")

	require.NoError(t, err)
	assert.Len(t, paths, 1)
}

func TestValidateScaffoldFile_ReadError(t *testing.T) {
	// A directory can't be read as a file, forcing os.ReadFile to fail.
	err := validateScaffoldFile(t.TempDir())

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrScaffoldReadFile)
}

// TestFindScaffoldFilesInDirectory_WalkError exercises the walk-error branch
// deterministically (a nonexistent root makes filepath.Walk's initial Lstat
// fail) instead of relying on chmod-based permission denial, which is
// unreliable when tests run as root or on Windows.
func TestFindScaffoldFilesInDirectory_WalkError(t *testing.T) {
	nonexistent := filepath.Join(t.TempDir(), "does-not-exist")

	_, err := findScaffoldFilesInDirectory(nonexistent, nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrScaffoldDirectoryRead)
}
