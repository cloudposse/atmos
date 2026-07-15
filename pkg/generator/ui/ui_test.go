package ui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/generator/engine"
	"github.com/cloudposse/atmos/pkg/generator/templates"
	iolib "github.com/cloudposse/atmos/pkg/io"
	runnerstep "github.com/cloudposse/atmos/pkg/runner/step"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/terminal"
	atmosui "github.com/cloudposse/atmos/pkg/ui"
)

// createTestUI creates a UI instance with I/O for testing.
func createTestUI(t *testing.T) *InitUI {
	t.Helper()

	// Create I/O context with default settings (stdout/stderr)
	ioCtx, err := iolib.NewContext()
	if err != nil {
		t.Fatalf("Failed to create I/O context: %v", err)
	}

	// Initialize UI formatter with I/O context
	atmosui.InitFormatter(ioCtx)

	// Create terminal with I/O
	termWriter := iolib.NewTerminalWriter(ioCtx)
	term := terminal.New(terminal.WithIO(termWriter))

	return NewInitUI(ioCtx, term)
}

func TestNewInitUI(t *testing.T) {
	ui := createTestUI(t)

	if ui.checkmark != "✓" {
		t.Errorf("Expected checkmark to be ✓, got %s", ui.checkmark)
	}

	if ui.xMark != "✗" {
		t.Errorf("Expected xMark to be ✗, got %s", ui.xMark)
	}

	// maxChanges field has been removed - threshold is now handled by the templating processor
}

func TestProcessFile_NewFile(t *testing.T) {
	ui := createTestUI(t)
	tempDir := t.TempDir()

	file := templates.File{
		Path:        "test.txt",
		Content:     "Hello World!",
		IsTemplate:  false,
		Permissions: 0o644,
	}

	// Convert templates.File to engine.File and use the processor
	templatingFile := engine.File{
		Path:        file.Path,
		Content:     file.Content,
		IsTemplate:  file.IsTemplate,
		Permissions: file.Permissions,
	}

	err := ui.processor.ProcessFile(templatingFile, tempDir, false, false, nil, nil)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	filePath := filepath.Join(tempDir, "test.txt")
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Error("Expected file to be created")
	}
}

func TestProcessFile_ExistingFile_NoFlags(t *testing.T) {
	ui := createTestUI(t)
	tempDir := t.TempDir()

	// Create existing file
	filePath := filepath.Join(tempDir, "test.txt")
	err := os.WriteFile(filePath, []byte("existing content"), 0o644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	file := templates.File{
		Path:        "test.txt",
		Content:     "new content",
		IsTemplate:  false,
		Permissions: 0o644,
	}

	// Convert templates.File to engine.File and use the processor
	templatingFile := engine.File{
		Path:        file.Path,
		Content:     file.Content,
		IsTemplate:  file.IsTemplate,
		Permissions: file.Permissions,
	}

	err = ui.processor.ProcessFile(templatingFile, tempDir, false, false, nil, nil)
	if err == nil {
		t.Fatal("Expected error for existing file")
	}

	if !strings.Contains(err.Error(), "file already exists") {
		t.Errorf("Expected error about existing file, got: %v", err)
	}
}

// TestFileExistsAt verifies the dry-run create/update status helper: the
// single authoritative signal executeWithSetup uses to label a file
// "(would create)" vs "(would update)" in dry-run preview output.
func TestFileExistsAt(t *testing.T) {
	tempDir := t.TempDir()

	if fileExistsAt(tempDir, "missing.txt") {
		t.Error("expected fileExistsAt to report false for a file that doesn't exist")
	}

	if err := os.WriteFile(filepath.Join(tempDir, "present.txt"), []byte("content"), 0o644); err != nil {
		t.Fatalf("failed to seed file: %v", err)
	}
	if !fileExistsAt(tempDir, "present.txt") {
		t.Error("expected fileExistsAt to report true for an existing file")
	}

	if err := os.MkdirAll(filepath.Join(tempDir, "adir"), 0o755); err != nil {
		t.Fatalf("failed to seed directory: %v", err)
	}
	if fileExistsAt(tempDir, "adir") {
		t.Error("expected fileExistsAt to report false for a directory (only files count)")
	}
}

// TestExecuteWithCommandValues_CustomDelimitersReachFileContent verifies that
// custom delimiters resolved in executeWithCommandValues actually reach file
// rendering (previously nil was passed as scaffoldConfig, so ProcessFile
// always fell back to the default "{{"/"}}" delimiters and left custom-
// delimited templates unrendered).
func TestExecuteWithCommandValues_CustomDelimitersReachFileContent(t *testing.T) {
	ui := createTestUI(t)
	tempDir := t.TempDir()

	embedsConfig := &templates.Configuration{
		Name: "test-template",
		Files: []templates.File{
			{
				Path:        "greeting.txt",
				Content:     "Hello, [[ .Config.name ]]!",
				IsTemplate:  true,
				Permissions: 0o644,
			},
		},
	}

	err := ui.executeWithCommandValues(embedsConfig, tempDir, false, false,
		map[string]interface{}{"name": "widget"}, []string{"[[", "]]"})
	if err != nil {
		t.Fatalf("executeWithCommandValues failed: %v", err)
	}

	content, readErr := os.ReadFile(filepath.Join(tempDir, "greeting.txt"))
	if readErr != nil {
		t.Fatalf("failed to read generated file: %v", readErr)
	}

	if !strings.Contains(string(content), "Hello, widget!") {
		t.Errorf("expected custom-delimited template to render, got: %q", string(content))
	}
	if strings.Contains(string(content), "[[") {
		t.Errorf("expected no unresolved custom delimiters in output, got: %q", string(content))
	}
}

// TestExecuteWithSetup_FilesWhenGating verifies spec.files[].when: gates
// generation of specific files based on the collected multiselect answer,
// independent of the pre-existing path-templating sentinel-skip mechanism.
func TestExecuteWithSetup_FilesWhenGating(t *testing.T) {
	ui := createTestUI(t)
	tempDir := t.TempDir()

	scaffoldYAML := `apiVersion: atmos/v1
kind: AtmosScaffoldConfig
metadata:
  name: test-template
spec:
  fields:
    - name: environments
      type: multiselect
      options: [dev, staging, prod]
  files:
    - path: dev.yaml
      when: "'dev' in answers.environments"
    - path: staging.yaml
      when: "'staging' in answers.environments"
    - path: prod.yaml
      when: "'prod' in answers.environments"
`

	embedsConfig := &templates.Configuration{
		Name: "test-template",
		Files: []templates.File{
			{Path: "scaffold.yaml", Content: scaffoldYAML, Permissions: 0o644},
			{Path: "dev.yaml", Content: "env: dev", Permissions: 0o644},
			{Path: "staging.yaml", Content: "env: staging", Permissions: 0o644},
			{Path: "prod.yaml", Content: "env: prod", Permissions: 0o644},
			{Path: "README.md", Content: "unconditional file", Permissions: 0o644},
		},
	}

	cmdTemplateValues := map[string]interface{}{"environments": []string{"dev", "staging"}}
	err := ui.executeWithSetup(embedsConfig, tempDir, false, false, true, "", cmdTemplateValues, []string{"{{", "}}"})
	if err != nil {
		t.Fatalf("executeWithSetup failed: %v", err)
	}

	for _, want := range []string{"dev.yaml", "staging.yaml", "README.md"} {
		if _, statErr := os.Stat(filepath.Join(tempDir, want)); statErr != nil {
			t.Errorf("expected %s to be generated, got: %v", want, statErr)
		}
	}
	if _, statErr := os.Stat(filepath.Join(tempDir, "prod.yaml")); !os.IsNotExist(statErr) {
		t.Errorf("expected prod.yaml to be skipped (not in selected environments), stat err: %v", statErr)
	}
}

func TestExecuteWithSetupRejectsInvalidBooleanCommandValue(t *testing.T) {
	ui := createTestUI(t)
	embedsConfig := &templates.Configuration{
		Name: "test-template",
		Files: []templates.File{
			{Path: "scaffold.yaml", Content: `apiVersion: atmos/v1
kind: AtmosScaffoldConfig
metadata:
  name: test-template
spec:
  fields:
    - name: enable_feature
      type: confirm
      default: false
`, Permissions: 0o644},
			{Path: "README.md", Content: "test", Permissions: 0o644},
		},
	}

	err := ui.executeWithSetup(
		embedsConfig,
		t.TempDir(),
		false,
		false,
		true,
		"",
		map[string]interface{}{"enable_feature": "not-a-bool"},
		[]string{"{{", "}}"},
	)
	require.Error(t, err)
	assert.ErrorContains(t, err, "enable_feature")
}

func TestScaffoldingExampleGeneratesPinnedVendorManifest(t *testing.T) {
	ui := createTestUI(t)
	exampleDir := filepath.Join("..", "..", "..", "examples", "scaffolding")
	embedsConfig, err := templates.LoadConfigurationFromDir("example", exampleDir)
	require.NoError(t, err)

	targetDir := t.TempDir()
	err = ui.executeWithSetup(
		embedsConfig,
		targetDir,
		false,
		false,
		true,
		"",
		map[string]interface{}{
			"enable_vendoring": "true",
			"vendor_version":   "1.536.0",
		},
		[]string{"{{", "}}"},
	)
	require.NoError(t, err)

	vendorManifest, err := os.ReadFile(filepath.Join(targetDir, "vendor.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(vendorManifest), "modules/s3-bucket?ref=1.536.0")
	assert.Contains(t, string(vendorManifest), "version: \"1.536.0\"")
	assert.Contains(t, string(vendorManifest), "components/terraform/s3-bucket")
}

// hookMarkerHandler records the resolved content of every step it executes,
// so tests can assert whether a scaffold hook actually ran.
type hookMarkerHandler struct {
	runnerstep.BaseHandler
	calls *[]string
}

func (h *hookMarkerHandler) Validate(*schema.WorkflowStep) error { return nil }

func (h *hookMarkerHandler) Execute(_ context.Context, step *schema.WorkflowStep, vars *runnerstep.Variables) (*runnerstep.StepResult, error) {
	resolved, err := vars.Resolve(step.Content)
	if err != nil {
		return nil, err
	}
	*h.calls = append(*h.calls, resolved)
	return runnerstep.NewStepResult(resolved), nil
}

// TestExecuteWithSetup_HooksRunAndCanBeSkipped verifies the post-generate
// hook runs after files are generated, and that SetSkipHooks bypasses a
// named hook without touching file generation itself.
func TestExecuteWithSetup_HooksRunAndCanBeSkipped(t *testing.T) {
	scaffoldYAML := `apiVersion: atmos/v1
kind: AtmosScaffoldConfig
metadata:
  name: test-template
spec:
  hooks:
    marker:
      events:
        - after.scaffold.generate
      kind: step
      type: hook-marker-test
      with:
        content: "post-generate ran"
`
	embedsConfig := &templates.Configuration{
		Name: "test-template",
		Files: []templates.File{
			{Path: "scaffold.yaml", Content: scaffoldYAML, Permissions: 0o644},
			{Path: "README.md", Content: "hello", Permissions: 0o644},
		},
	}

	t.Run("hook runs by default", func(t *testing.T) {
		calls := &[]string{}
		runnerstep.Register(&hookMarkerHandler{
			BaseHandler: runnerstep.NewBaseHandler("hook-marker-test", runnerstep.CategoryOutput, false),
			calls:       calls,
		})

		ui := createTestUI(t)
		tempDir := t.TempDir()
		err := ui.executeWithSetup(embedsConfig, tempDir, false, false, true, "", map[string]interface{}{}, []string{"{{", "}}"})
		if err != nil {
			t.Fatalf("executeWithSetup failed: %v", err)
		}
		if got := *calls; len(got) != 1 || got[0] != "post-generate ran" {
			t.Errorf("expected the post-generate hook to run once, got: %v", got)
		}
	})

	t.Run("hook is bypassed via SetSkipHooks", func(t *testing.T) {
		calls := &[]string{}
		runnerstep.Register(&hookMarkerHandler{
			BaseHandler: runnerstep.NewBaseHandler("hook-marker-test-skip", runnerstep.CategoryOutput, false),
			calls:       calls,
		})
		skippedYAML := strings.Replace(scaffoldYAML, "hook-marker-test", "hook-marker-test-skip", 1)
		skippedConfig := &templates.Configuration{
			Name: "test-template",
			Files: []templates.File{
				{Path: "scaffold.yaml", Content: skippedYAML, Permissions: 0o644},
				{Path: "README.md", Content: "hello", Permissions: 0o644},
			},
		}

		ui := createTestUI(t)
		ui.SetSkipHooks(func(name string) bool { return name == "marker" })
		tempDir := t.TempDir()
		err := ui.executeWithSetup(skippedConfig, tempDir, false, false, true, "", map[string]interface{}{}, []string{"{{", "}}"})
		if err != nil {
			t.Fatalf("executeWithSetup failed: %v", err)
		}
		if got := *calls; len(got) != 0 {
			t.Errorf("expected the post-generate hook to be skipped, got: %v", got)
		}
	})
}

// TestExecuteWithSetup_BasicTemplate_EnvironmentsGating exercises the real,
// embedded "basic" init template end-to-end through executeWithSetup (not a
// hand-built fixture) to verify its multiselect "environments" field and
// spec.files[].when: overlay actually gate which stack files get generated.
func TestExecuteWithSetup_BasicTemplate_EnvironmentsGating(t *testing.T) {
	configs, err := templates.GetAvailableConfigurations()
	if err != nil {
		t.Fatalf("failed to load embedded configurations: %v", err)
	}
	basic, ok := configs["basic"]
	if !ok {
		t.Fatal("basic template not found in embedded configurations")
	}

	ui := createTestUI(t)
	tempDir := t.TempDir()
	values := map[string]interface{}{
		"project_name": "test-proj",
		"environments": []string{"dev", "staging"},
	}
	if err := ui.executeWithSetup(&basic, tempDir, false, false, true, "", values, []string{"{{", "}}"}); err != nil {
		t.Fatalf("executeWithSetup failed: %v", err)
	}

	for _, want := range []string{"stacks/dev.yaml", "stacks/staging.yaml"} {
		if _, statErr := os.Stat(filepath.Join(tempDir, filepath.FromSlash(want))); statErr != nil {
			t.Errorf("expected %s to be generated, got: %v", want, statErr)
		}
	}
	if _, statErr := os.Stat(filepath.Join(tempDir, "stacks", "prod.yaml")); !os.IsNotExist(statErr) {
		t.Errorf("expected stacks/prod.yaml to be skipped (not in selected environments), stat err: %v", statErr)
	}

	readme, err := os.ReadFile(filepath.Join(tempDir, "README.md"))
	if err != nil {
		t.Fatalf("failed to read README.md: %v", err)
	}
	if !strings.Contains(string(readme), "`stacks/dev.yaml`") || !strings.Contains(string(readme), "`stacks/staging.yaml`") {
		t.Errorf("expected README to list only the selected stacks, got: %s", readme)
	}
	if strings.Contains(string(readme), "`stacks/prod.yaml`") {
		t.Errorf("expected README not to list the unselected prod stack, got: %s", readme)
	}
}
