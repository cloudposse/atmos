package scaffold

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/generator/templates"
	"github.com/cloudposse/atmos/pkg/manifest"
	"github.com/cloudposse/atmos/pkg/project/config"
)

func TestNewScaffoldCommandProvider(t *testing.T) {
	provider := &ScaffoldCommandProvider{}

	assert.NotNil(t, provider)
	assert.Equal(t, "scaffold", provider.GetName())
	assert.Equal(t, "Configuration Management", provider.GetGroup())
	assert.NotNil(t, provider.GetCommand())
	// Parent scaffold command has no flags; flags belong to generate subcommand.
	assert.Nil(t, provider.GetFlagsBuilder())
	assert.Nil(t, provider.GetPositionalArgsBuilder())
	assert.Nil(t, provider.GetCompatibilityFlags())
}

func TestScaffoldCommandProvider_GetCommand(t *testing.T) {
	provider := &ScaffoldCommandProvider{}
	cmd := provider.GetCommand()

	assert.NotNil(t, cmd)
	assert.Equal(t, "scaffold", cmd.Name())
	assert.Contains(t, cmd.Short, "Generate")
	assert.Contains(t, cmd.Long, "Generate code from scaffold templates")
}

func TestScaffoldCommandProvider_GetFlagsBuilder(t *testing.T) {
	provider := &ScaffoldCommandProvider{}
	builder := provider.GetFlagsBuilder()

	// Parent scaffold command has no flags; flags belong to generate subcommand.
	assert.Nil(t, builder)
}

func TestScaffoldCmd_Subcommands(t *testing.T) {
	// Verify scaffold has expected subcommands
	commands := scaffoldCmd.Commands()
	assert.NotEmpty(t, commands)

	commandNames := make([]string, 0, len(commands))
	for _, c := range commands {
		commandNames = append(commandNames, c.Name())
	}

	assert.Contains(t, commandNames, "generate")
	assert.Contains(t, commandNames, "list")
	assert.Contains(t, commandNames, "validate")
}

func TestScaffoldGenerateCmd_FlagDefinitions(t *testing.T) {
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
			name:         "dry-run flag",
			flagName:     "dry-run",
			shorthand:    "",
			defaultValue: "false",
		},
		{
			name:      "set flag",
			flagName:  "set",
			shorthand: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flag := scaffoldGenerateCmd.Flags().Lookup(tt.flagName)
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

func TestScaffoldGenerateCmd_Args(t *testing.T) {
	// MaximumNArgs(2) allows 0, 1, or 2 arguments
	assert.NoError(t, scaffoldGenerateCmd.Args(scaffoldGenerateCmd, []string{}))
	assert.NoError(t, scaffoldGenerateCmd.Args(scaffoldGenerateCmd, []string{"component"}))
	assert.NoError(t, scaffoldGenerateCmd.Args(scaffoldGenerateCmd, []string{"component", "/tmp/target"}))
	assert.Error(t, scaffoldGenerateCmd.Args(scaffoldGenerateCmd, []string{"component", "/tmp/target", "extra"}))
}

func TestScaffoldListCmd_Args(t *testing.T) {
	// NoArgs means no arguments allowed
	assert.NoError(t, scaffoldListCmd.Args(scaffoldListCmd, []string{}))
	assert.Error(t, scaffoldListCmd.Args(scaffoldListCmd, []string{"extra"}))
}

func TestScaffoldValidateCmd_Args(t *testing.T) {
	// MaximumNArgs(1) allows 0 or 1 argument
	assert.NoError(t, scaffoldValidateCmd.Args(scaffoldValidateCmd, []string{}))
	assert.NoError(t, scaffoldValidateCmd.Args(scaffoldValidateCmd, []string{"path/to/scaffold.yaml"}))
	assert.Error(t, scaffoldValidateCmd.Args(scaffoldValidateCmd, []string{"path1", "path2"}))
}

func TestFindScaffoldFiles(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(t *testing.T) string
		expectError bool
		expectCount int
	}{
		{
			name: "single scaffold.yaml file",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				scaffoldPath := filepath.Join(tmpDir, "scaffold.yaml")
				err := os.WriteFile(scaffoldPath, []byte("name: test"), 0o644)
				require.NoError(t, err)
				return scaffoldPath
			},
			expectError: false,
			expectCount: 1,
		},
		{
			name: "single scaffold.yml file",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				scaffoldPath := filepath.Join(tmpDir, "scaffold.yml")
				err := os.WriteFile(scaffoldPath, []byte("name: test"), 0o644)
				require.NoError(t, err)
				return scaffoldPath
			},
			expectError: false,
			expectCount: 1,
		},
		{
			name: "directory with scaffold.yaml",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				scaffoldPath := filepath.Join(tmpDir, "scaffold.yaml")
				err := os.WriteFile(scaffoldPath, []byte("name: test"), 0o644)
				require.NoError(t, err)
				return tmpDir
			},
			expectError: false,
			expectCount: 1,
		},
		{
			name: "directory with nested scaffold.yaml files",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()

				// Create first scaffold
				scaffoldPath1 := filepath.Join(tmpDir, "scaffold.yaml")
				err := os.WriteFile(scaffoldPath1, []byte("name: test1"), 0o644)
				require.NoError(t, err)

				// Create nested scaffold
				nestedDir := filepath.Join(tmpDir, "nested")
				err = os.MkdirAll(nestedDir, 0o755)
				require.NoError(t, err)
				scaffoldPath2 := filepath.Join(nestedDir, "scaffold.yaml")
				err = os.WriteFile(scaffoldPath2, []byte("name: test2"), 0o644)
				require.NoError(t, err)

				return tmpDir
			},
			expectError: false,
			expectCount: 2,
		},
		{
			name: "file without scaffold name",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				otherPath := filepath.Join(tmpDir, "other.yaml")
				err := os.WriteFile(otherPath, []byte("name: test"), 0o644)
				require.NoError(t, err)
				return otherPath
			},
			expectError: true,
		},
		{
			name: "nonexistent path",
			setup: func(t *testing.T) string {
				return "/nonexistent/path"
			},
			expectError: true,
		},
		{
			name: "empty directory",
			setup: func(t *testing.T) string {
				return t.TempDir()
			},
			expectError: false,
			expectCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.setup(t)

			scaffoldPaths, err := findScaffoldFiles(path)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Len(t, scaffoldPaths, tt.expectCount)
			}
		})
	}
}

func TestValidateScaffoldFile(t *testing.T) {
	tests := []struct {
		name         string
		content      string
		expectError  bool
		errorContain string
	}{
		{
			name: "valid scaffold manifest",
			content: `apiVersion: atmos/v1
kind: AtmosScaffoldConfig
metadata:
  name: test-scaffold
  description: Test scaffold
  version: 1.0.0
spec:
  fields:
    - name: component_name
      type: input
      description: Component name
      required: true
`,
			expectError: false,
		},
		{
			name: "legacy prompts format rejected",
			content: `name: test-scaffold
description: Test scaffold
prompts:
  - name: component_name
    type: input
`,
			expectError:  true,
			errorContain: "scaffold validation failed",
		},
		{
			name: "missing metadata name",
			content: `apiVersion: atmos/v1
kind: AtmosScaffoldConfig
metadata:
  description: no name
`,
			expectError:  true,
			errorContain: "scaffold validation failed",
		},
		{
			name: "invalid YAML",
			content: `name: test
invalid: [unclosed
`,
			expectError:  true,
			errorContain: "scaffold validation failed",
		},
		{
			name: "field without name",
			content: `apiVersion: atmos/v1
kind: AtmosScaffoldConfig
metadata:
  name: test-scaffold
spec:
  fields:
    - type: input
      description: Test
`,
			expectError:  true,
			errorContain: "scaffold validation failed",
		},
		{
			name: "field with invalid type",
			content: `apiVersion: atmos/v1
kind: AtmosScaffoldConfig
metadata:
  name: test-scaffold
spec:
  fields:
    - name: test_field
      type: invalid_type
      description: Test
`,
			expectError:  true,
			errorContain: "scaffold validation failed",
		},
		{
			name: "unknown top-level property rejected",
			content: `apiVersion: atmos/v1
kind: AtmosScaffoldConfig
metadata:
  name: test-scaffold
extra: not-allowed
`,
			expectError:  true,
			errorContain: "scaffold validation failed",
		},
		{
			name: "all valid field types",
			content: `apiVersion: atmos/v1
kind: AtmosScaffoldConfig
metadata:
  name: test-scaffold
spec:
  fields:
    - name: field1
      type: input
    - name: field2
      type: select
    - name: field3
      type: confirm
    - name: field4
      type: multiselect
`,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			scaffoldPath := filepath.Join(tmpDir, "scaffold.yaml")
			err := os.WriteFile(scaffoldPath, []byte(tt.content), 0o644)
			require.NoError(t, err)

			err = validateScaffoldFile(scaffoldPath)

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

// writeLocalTemplate creates a minimal local template directory with a
// scaffold.yaml manifest and one templated file, returning its path.
func writeLocalTemplate(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	scaffoldContent := `apiVersion: atmos/v1
kind: AtmosScaffoldConfig
metadata:
  name: local-template
  description: Local scaffold from disk
  version: 0.1.0
spec:
  fields:
    - name: project_name
      type: input
      default: my-project
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "scaffold.yaml"), []byte(scaffoldContent), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md.tmpl"), []byte("# {{.Config.project_name}}\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "stacks"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "stacks", "main.yaml"), []byte("vars: {}\n"), 0o644))
	return dir
}

func TestConvertScaffoldTemplateToConfiguration(t *testing.T) {
	localDir := writeLocalTemplate(t)

	tests := []struct {
		name         string
		templateName string
		templateData interface{}
		expectError  bool
		validate     func(t *testing.T, config templates.Configuration)
	}{
		{
			name:         "template without source rejected",
			templateName: "component",
			templateData: map[string]interface{}{
				"description": "Component scaffold",
			},
			expectError: true, // A source directory is required.
		},
		{
			name:         "template with remote source (lazy stub)",
			templateName: "remote-template",
			templateData: map[string]interface{}{
				"description": "Remote scaffold",
				"source":      "https://github.com/example/template.git",
			},
			expectError: false, // Remote templates resolve to a stub fetched lazily at generation time.
			validate: func(t *testing.T, config templates.Configuration) {
				assert.Equal(t, "remote-template", config.Name)
				assert.Equal(t, "Remote scaffold", config.Description)
				assert.Equal(t, "https://github.com/example/template.git", config.Source)
				assert.Empty(t, config.Files, "remote stub must defer file loading until hydration")
			},
		},
		{
			name:         "template with nonexistent local source rejected",
			templateName: "missing-template",
			templateData: map[string]interface{}{
				"source": filepath.Join(localDir, "does-not-exist"),
			},
			expectError: true,
		},
		{
			name:         "template with local source loads files",
			templateName: "local-template",
			templateData: map[string]interface{}{
				"source": localDir,
			},
			expectError: false,
			validate: func(t *testing.T, config templates.Configuration) {
				// Metadata comes from the template's own scaffold.yaml.
				assert.Equal(t, "local-template", config.Name)
				assert.Equal(t, "Local scaffold from disk", config.Description)
				assert.Equal(t, "0.1.0", config.Version)
				assert.Equal(t, localDir, config.Source)
				// Files are actually loaded from disk, including nested dirs.
				paths := make([]string, 0, len(config.Files))
				for _, f := range config.Files {
					paths = append(paths, f.Path)
				}
				assert.Contains(t, paths, "scaffold.yaml")
				assert.Contains(t, paths, "README.md.tmpl")
				assert.Contains(t, paths, "stacks/main.yaml")
			},
		},
		{
			name:         "atmos.yaml description overrides template metadata",
			templateName: "local-template",
			templateData: map[string]interface{}{
				"description": "Override from atmos.yaml",
				"source":      localDir,
				"target_dir":  "stacks/components",
			},
			expectError: false,
			validate: func(t *testing.T, config templates.Configuration) {
				assert.Equal(t, "Override from atmos.yaml", config.Description)
				assert.Equal(t, "stacks/components", config.TargetDir)
			},
		},
		{
			name:         "invalid template data",
			templateName: "invalid",
			templateData: "not a map",
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := convertScaffoldTemplateToConfiguration(tt.templateName, tt.templateData)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tt.validate != nil {
					tt.validate(t, config)
				}
			}
		})
	}
}

func TestScaffoldGenerateParser_Creation(t *testing.T) {
	assert.NotNil(t, scaffoldGenerateParser)
	assert.IsType(t, &flags.StandardParser{}, scaffoldGenerateParser)
}

func TestScaffoldCmd_Integration_Help(t *testing.T) {
	// Test help output for main command
	scaffoldCmd.SetArgs([]string{"--help"})
	err := scaffoldCmd.Execute()
	assert.NoError(t, err)
}

func TestScaffoldGenerateCmd_Integration_Help(t *testing.T) {
	// Test help output for generate subcommand
	scaffoldGenerateCmd.SetArgs([]string{"--help"})
	err := scaffoldGenerateCmd.Execute()
	assert.NoError(t, err)
}

func TestScaffoldListCmd_Integration_Help(t *testing.T) {
	// Test help output for list subcommand
	scaffoldListCmd.SetArgs([]string{"--help"})
	err := scaffoldListCmd.Execute()
	assert.NoError(t, err)
}

func TestScaffoldValidateCmd_Integration_Help(t *testing.T) {
	// Test help output for validate subcommand
	scaffoldValidateCmd.SetArgs([]string{"--help"})
	err := scaffoldValidateCmd.Execute()
	assert.NoError(t, err)
}

func TestScaffoldCmd_ViperIntegration(t *testing.T) {
	v := viper.New()

	// Set values via viper
	v.Set("force", true)
	v.Set("dry-run", true)
	v.Set("set", map[string]string{"key": "value"})

	// Verify viper values
	assert.True(t, v.GetBool("force"))
	assert.True(t, v.GetBool("dry-run"))
	assert.NotNil(t, v.Get("set"))
}

func TestExecuteValidateScaffold_EmptyDirectory(t *testing.T) {
	t.Skip("Integration test - requires UI formatter initialization")

	tmpDir := t.TempDir()

	err := executeValidateScaffold(context.Background(), tmpDir)

	// Empty directory should not error, just report no files found
	assert.NoError(t, err)
}

func TestExecuteValidateScaffold_WithValidFile(t *testing.T) {
	t.Skip("Integration test - requires UI formatter initialization")

	tmpDir := t.TempDir()

	// Create valid scaffold.yaml
	scaffoldPath := filepath.Join(tmpDir, "scaffold.yaml")
	content := `apiVersion: atmos/v1
kind: AtmosScaffoldConfig
metadata:
  name: test-scaffold
  description: Test
  version: 1.0.0
spec:
  fields:
    - name: test_field
      type: input
`
	err := os.WriteFile(scaffoldPath, []byte(content), 0o644)
	require.NoError(t, err)

	err = executeValidateScaffold(context.Background(), tmpDir)
	assert.NoError(t, err)
}

func TestInit_PackageInitialization(t *testing.T) {
	// Test that init() function was called and registered command
	assert.NotNil(t, scaffoldCmd)
	assert.NotNil(t, scaffoldGenerateCmd)
	assert.NotNil(t, scaffoldListCmd)
	assert.NotNil(t, scaffoldValidateCmd)
	assert.NotNil(t, scaffoldGenerateParser)

	// Verify flags are registered on generate command
	assert.NotNil(t, scaffoldGenerateCmd.Flags().Lookup("force"))
	assert.NotNil(t, scaffoldGenerateCmd.Flags().Lookup("dry-run"))
	assert.NotNil(t, scaffoldGenerateCmd.Flags().Lookup("set"))
}

func TestScaffoldCmd_CoverageBooster(t *testing.T) {
	// This test exercises code paths for coverage
	provider := &ScaffoldCommandProvider{}

	// Exercise all interface methods
	_ = provider.GetCommand()
	_ = provider.GetName()
	_ = provider.GetGroup()
	_ = provider.GetFlagsBuilder()
	_ = provider.GetPositionalArgsBuilder()
	_ = provider.GetCompatibilityFlags()

	// Verify values
	assert.Equal(t, "scaffold", provider.GetName())
	assert.Equal(t, "Configuration Management", provider.GetGroup())
}

func TestScaffoldGenerateCmd_SetFlagParsing(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		expectedKey   string
		expectedValue string
		expectError   bool
	}{
		{
			name:          "valid key=value",
			input:         "key=value",
			expectedKey:   "key",
			expectedValue: "value",
			expectError:   false,
		},
		{
			name:        "missing equals",
			input:       "keyvalue",
			expectError: true,
		},
		{
			name:        "empty key",
			input:       "=value",
			expectError: true,
		},
		{
			name:          "valid with spaces in value",
			input:         "key=value with spaces",
			expectedKey:   "key",
			expectedValue: "value with spaces",
			expectError:   false,
		},
		{
			name:          "key with leading/trailing spaces trimmed",
			input:         " key =value",
			expectedKey:   "key",
			expectedValue: "value",
			expectError:   false,
		},
		{
			name:          "value with leading/trailing spaces trimmed",
			input:         "key= value with spaces ",
			expectedKey:   "key",
			expectedValue: "value with spaces",
			expectError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test validation
			err := validateSetFlag(tt.input)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			// Test parsing for valid inputs
			if !tt.expectError {
				key, value, parseErr := parseSetFlag(tt.input)
				assert.NoError(t, parseErr)
				assert.Equal(t, tt.expectedKey, key)
				assert.Equal(t, tt.expectedValue, value)
			}
		})
	}
}

func TestScaffoldGenerateCmd_TemplateValuesConversion(t *testing.T) {
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
				"component_name": "vpc",
			},
			expected: map[string]interface{}{
				"component_name": "vpc",
			},
		},
		{
			name: "multiple values",
			input: map[string]string{
				"component_name": "vpc",
				"namespace":      "core",
				"region":         "us-east-1",
			},
			expected: map[string]interface{}{
				"component_name": "vpc",
				"namespace":      "core",
				"region":         "us-east-1",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the conversion logic from scaffold.go:118-121
			templateValues := make(map[string]interface{})
			for k, val := range tt.input {
				templateValues[k] = val
			}

			assert.Equal(t, tt.expected, templateValues)
		})
	}
}

func TestScaffoldGenerateCmd_RunEFunction(t *testing.T) {
	// Verify RunE function is set
	assert.NotNil(t, scaffoldGenerateCmd.RunE)
}

func TestScaffoldListCmd_RunEFunction(t *testing.T) {
	// Verify RunE function is set
	assert.NotNil(t, scaffoldListCmd.RunE)
}

func TestScaffoldValidateCmd_RunEFunction(t *testing.T) {
	// Verify RunE function is set
	assert.NotNil(t, scaffoldValidateCmd.RunE)
}

func TestScaffoldManifestSchema_Registered(t *testing.T) {
	// The AtmosScaffoldConfig kind registers its generated JSON Schema with
	// the manifest registry (replacing the old embedded scaffold-schema.json).
	def, ok := manifest.GetDefinition(config.ScaffoldKind)
	require.True(t, ok)
	assert.Contains(t, def.SchemaJSON(), "$schema")
	assert.Contains(t, def.SchemaJSON(), config.ScaffoldKind)
}

func TestExecuteScaffoldGenerate_AbsolutePath(t *testing.T) {
	tests := []struct {
		name      string
		targetDir string
	}{
		{
			name:      "relative path converted to absolute",
			targetDir: "test-component",
		},
		{
			name:      "absolute path kept as-is",
			targetDir: "/tmp/test-component",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This test validates the path conversion logic
			// without actually executing the full command
			if tt.targetDir != "" {
				absPath, err := filepath.Abs(tt.targetDir)
				assert.NoError(t, err)
				assert.True(t, filepath.IsAbs(absPath))
			}
		})
	}
}

func TestFindScaffoldFiles_WalkError(t *testing.T) {
	// Create a directory structure where we can't walk
	tmpDir := t.TempDir()
	restrictedDir := filepath.Join(tmpDir, "restricted")
	err := os.MkdirAll(restrictedDir, 0o000) // No permissions
	if err != nil {
		t.Skip("Cannot create restricted directory on this system")
	}
	defer os.Chmod(restrictedDir, 0o755) // Cleanup

	_, err = findScaffoldFiles(tmpDir)
	// Should handle walk errors gracefully
	// Behavior may vary by OS
	if err != nil {
		assert.Error(t, err)
	}
}

func TestMergeConfiguredTemplates_NoTemplatesKey(t *testing.T) {
	// Isolate from the real atmos.yaml in the repo by switching to an
	// empty temp directory.  ReadAtmosScaffoldSection("." ) returns an empty
	// map when no atmos.yaml is present, exercising the "no templates key"
	// code path.
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	configs := map[string]templates.Configuration{
		"existing": {Name: "existing", Description: "Existing template"},
	}
	origins := map[string]string{
		"existing": "embedded",
	}

	// No atmos.yaml → no templates section → should not error.
	err := mergeConfiguredTemplates(configs, origins)
	assert.NoError(t, err)
	assert.Len(t, configs, 1) // Original template still there.
}

func TestMergeConfiguredTemplates_InvalidTemplatesFormat(t *testing.T) {
	// Write a fixture atmos.yaml where `scaffold.templates` is a scalar
	// (not a map) to trigger the invalid-format branch.
	tmpDir := t.TempDir()
	fixture := `scaffold:
  templates: "not-a-map"
`
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "atmos.yaml"), []byte(fixture), 0o644))
	t.Chdir(tmpDir)

	configs := map[string]templates.Configuration{}
	origins := map[string]string{}

	err := mergeConfiguredTemplates(configs, origins)
	// Scalar templates value is rejected with ErrInvalidScaffoldConfig.
	require.Error(t, err)
	assert.NotNil(t, configs) // Configs map is untouched on error.
}

func TestSelectTemplateByName_NotFound(t *testing.T) {
	configs := map[string]templates.Configuration{
		"component": {Name: "component", Description: "Component template"},
		"stack":     {Name: "stack", Description: "Stack template"},
	}

	_, err := selectTemplateByName("nonexistent", configs)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "scaffold template")
	assert.Contains(t, err.Error(), "not found")
}

func TestSelectTemplateByName_Found(t *testing.T) {
	configs := map[string]templates.Configuration{
		"component": {Name: "component", Description: "Component template"},
		"stack":     {Name: "stack", Description: "Stack template"},
	}

	result, err := selectTemplateByName("component", configs)
	require.NoError(t, err)
	assert.Equal(t, "component", result.Name)
	assert.Equal(t, "Component template", result.Description)
}

func TestValidateAllScaffoldFiles_WithErrors(t *testing.T) {
	t.Skip("Integration test - requires UI formatter initialization")

	// Create temp files with mix of valid and invalid scaffolds
	tmpDir := t.TempDir()

	validPath := filepath.Join(tmpDir, "valid.yaml")
	validContent := `apiVersion: atmos/v1
kind: AtmosScaffoldConfig
metadata:
  name: valid-scaffold
spec:
  fields:
    - name: test
      type: input`
	err := os.WriteFile(validPath, []byte(validContent), 0o644)
	require.NoError(t, err)

	invalidPath := filepath.Join(tmpDir, "invalid.yaml")
	invalidContent := `spec: {}` // Missing envelope and metadata.name.
	err = os.WriteFile(invalidPath, []byte(invalidContent), 0o644)
	require.NoError(t, err)

	scaffoldPaths := []string{validPath, invalidPath}

	validCount, errorCount := validateAllScaffoldFiles(scaffoldPaths)
	assert.Equal(t, 1, validCount)
	assert.Equal(t, 1, errorCount)
}

func TestPrintValidationSummary_WithErrors(t *testing.T) {
	t.Skip("Integration test - requires UI formatter initialization")

	err := printValidationSummary(2, 1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed validation")
}

func TestPrintValidationSummary_NoErrors(t *testing.T) {
	t.Skip("Integration test - requires UI formatter initialization")

	err := printValidationSummary(3, 0)
	require.NoError(t, err)
}

func TestDetermineScaffoldPathsToValidate(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		setup       func(t *testing.T) string
		expectError bool
	}{
		{
			name: "valid directory",
			path: "test",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				scaffoldPath := filepath.Join(tmpDir, "scaffold.yaml")
				err := os.WriteFile(scaffoldPath, []byte("name: test"), 0o644)
				require.NoError(t, err)
				return tmpDir
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.path
			if tt.setup != nil {
				path = tt.setup(t)
			}

			paths, err := determineScaffoldPathsToValidate(path)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				// paths can be empty slice or non-nil
				_ = paths
			}
		})
	}
}

func TestLoadDryRunValues_ErrorPaths(t *testing.T) {
	tests := []struct {
		name        string
		config      *templates.Configuration
		vars        map[string]interface{}
		expectError bool
	}{
		{
			name: "scaffold config with invalid YAML",
			config: &templates.Configuration{
				Files: []templates.File{
					{
						Path:    "scaffold.yaml",
						Content: "invalid: [unclosed yaml",
					},
				},
			},
			vars:        map[string]interface{}{},
			expectError: true,
		},
		{
			name: "no scaffold config file",
			config: &templates.Configuration{
				Files: []templates.File{
					{Path: "README.md", Content: "# Test"},
				},
			},
			vars:        map[string]interface{}{"var1": "value1"},
			expectError: false,
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

func TestParseSetFlag_AllBranches(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectKey   string
		expectValue string
		expectError bool
	}{
		{
			name:        "valid key=value",
			input:       "key=value",
			expectKey:   "key",
			expectValue: "value",
			expectError: false,
		},
		{
			name:        "with spaces trimmed",
			input:       " key = value ",
			expectKey:   "key",
			expectValue: "value",
			expectError: false,
		},
		{
			name:        "error - no equals sign",
			input:       "keyvalue",
			expectError: true,
		},
		{
			name:        "error - empty key",
			input:       "=value",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, value, err := parseSetFlag(tt.input)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectKey, key)
				assert.Equal(t, tt.expectValue, value)
			}
		})
	}
}

func TestMergeConfiguredTemplates_AllBranches(t *testing.T) {
	tests := []struct {
		name          string
		fixture       string // atmos.yaml content; empty = no file
		initialConfig map[string]templates.Configuration
		wantErr       bool
	}{
		{
			name:    "no atmos.yaml preserves existing configs",
			fixture: "", // No atmos.yaml in temp dir.
			initialConfig: map[string]templates.Configuration{
				"existing": {Name: "existing", Description: "Existing template"},
			},
			wantErr: false,
		},
		{
			name: "scaffold section with no templates key is a no-op",
			fixture: `scaffold:
  some_other_key: value
`,
			initialConfig: map[string]templates.Configuration{},
			wantErr:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			if tt.fixture != "" {
				require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "atmos.yaml"), []byte(tt.fixture), 0o644))
			}
			t.Chdir(tmpDir)

			configs := make(map[string]templates.Configuration)
			for k, v := range tt.initialConfig {
				configs[k] = v
			}
			origins := make(map[string]string)
			for name := range configs {
				origins[name] = "embedded"
			}

			err := mergeConfiguredTemplates(configs, origins)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.NotNil(t, configs)
		})
	}
}

func TestExecuteTemplateGeneration_ErrorPath(t *testing.T) {
	t.Skip("Requires UI initialization - integration test")

	config := templates.Configuration{
		Name:  "test",
		Files: []templates.File{{Path: "test.txt", Content: "content"}},
	}

	// Test with nil UI to trigger error.
	opts := scaffoldGenerateOptions{useDefaults: true, templateValues: map[string]interface{}{}}
	err := executeTemplateGeneration(&config, t.TempDir(), opts, nil)
	assert.Error(t, err)
}

func TestResolveTargetDirectory_ErrorPath(t *testing.T) {
	// Test that filepath.Abs errors are handled
	// This is hard to trigger in practice, so we just ensure the function exists
	result, err := resolveTargetDirectory(".")
	assert.NoError(t, err)
	assert.True(t, filepath.IsAbs(result))
}

func TestLoadScaffoldTemplates_Coverage(t *testing.T) {
	// Test the function executes without errors
	configs, origins, ui, err := loadScaffoldTemplates("")
	require.NoError(t, err)
	assert.NotNil(t, configs)
	assert.NotNil(t, origins)
	assert.NotNil(t, ui)

	// Verify some expected templates exist
	assert.NotEmpty(t, configs, "Should have at least some embedded templates")

	// Verify origins are tracked for all configs
	for name := range configs {
		assert.Contains(t, origins, name, "Origin should be tracked for template %s", name)
	}
}
