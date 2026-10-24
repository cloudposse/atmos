package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestImportCommandMerging tests various scenarios of command merging through imports.
// This addresses the issue where imported commands should be merged (not replaced).
func TestImportCommandMerging(t *testing.T) {
	setupTestAdapters()

	tests := []struct {
		name             string
		setupFiles       map[string]string
		expectedCommands []string // Expected command names in order
		description      string
	}{
		{
			name: "explicit_import_merges_commands",
			setupFiles: map[string]string{
				"atmos.yaml": `
base_path: "."
import:
  - "imported.yaml"
commands:
  - name: "main-cmd1"
    description: "Command from main"
    steps:
      - echo "main1"
  - name: "main-cmd2"
    description: "Another command from main"
    steps:
      - echo "main2"
`,
				"imported.yaml": `
commands:
  - name: "imported-cmd1"
    description: "Command from import"
    steps:
      - echo "imported1"
  - name: "imported-cmd2"
    description: "Another command from import"
    steps:
      - echo "imported2"
`,
			},
			expectedCommands: []string{"main-cmd1", "main-cmd2", "imported-cmd1", "imported-cmd2"},
			description:      "Explicit imports should merge commands (main can override imported)",
		},
		{
			name: "nested_imports_merge_all_commands",
			setupFiles: map[string]string{
				"atmos.yaml": `
base_path: "."
import:
  - "level1.yaml"
commands:
  - name: "main-cmd"
    description: "Command from main"
    steps:
      - echo "main"
`,
				"level1.yaml": `
import:
  - "level2.yaml"
commands:
  - name: "level1-cmd"
    description: "Command from level1"
    steps:
      - echo "level1"
`,
				"level2.yaml": `
commands:
  - name: "level2-cmd"
    description: "Command from level2"
    steps:
      - echo "level2"
`,
			},
			expectedCommands: []string{"main-cmd", "level1-cmd", "level2-cmd"},
			description:      "Nested imports should merge all commands (main first, then imports)",
		},
		{
			name: "atmos_d_merges_with_explicit_imports",
			setupFiles: map[string]string{
				"atmos.yaml": `
base_path: "."
import:
  - "imported.yaml"
commands:
  - name: "main-cmd"
    description: "Command from main"
    steps:
      - echo "main"
`,
				"imported.yaml": `
commands:
  - name: "imported-cmd"
    description: "Command from import"
    steps:
      - echo "imported"
`,
				".atmos.d/extra.yaml": `
commands:
  - name: "atmos-d-cmd"
    description: "Command from .atmos.d"
    steps:
      - echo "atmos.d"
`,
			},
			expectedCommands: []string{"main-cmd", "atmos-d-cmd", "imported-cmd"},
			description:      ".atmos.d commands should merge with both imported and main commands (main, .atmos.d, imports)",
		},
		{
			name: "duplicate_command_names_deduplicated",
			setupFiles: map[string]string{
				"atmos.yaml": `
base_path: "."
import:
  - "imported.yaml"
commands:
  - name: "shared-cmd"
    description: "Main version of shared command"
    steps:
      - echo "main version"
  - name: "main-only"
    description: "Only in main"
    steps:
      - echo "main only"
`,
				"imported.yaml": `
commands:
  - name: "shared-cmd"
    description: "Imported version of shared command"
    steps:
      - echo "imported version"
  - name: "imported-only"
    description: "Only in imported"
    steps:
      - echo "imported only"
`,
			},
			expectedCommands: []string{"shared-cmd", "main-only", "imported-only"},
			description:      "Duplicate command names should be deduplicated (main version wins)",
		},
		{
			name: "multiple_imports_merge_all",
			setupFiles: map[string]string{
				"atmos.yaml": `
base_path: "."
import:
  - "import1.yaml"
  - "import2.yaml"
commands:
  - name: "main-cmd"
    description: "Command from main"
    steps:
      - echo "main"
`,
				"import1.yaml": `
commands:
  - name: "import1-cmd"
    description: "Command from import1"
    steps:
      - echo "import1"
`,
				"import2.yaml": `
commands:
  - name: "import2-cmd"
    description: "Command from import2"
    steps:
      - echo "import2"
`,
			},
			expectedCommands: []string{"main-cmd", "import1-cmd", "import2-cmd"},
			description:      "Multiple imports should all merge their commands (main first)",
		},
		{
			name: "cloudposse_style_centralized_config",
			setupFiles: map[string]string{
				"atmos.yaml": `
base_path: "."
import:
  - "github-org-config/atmos-common.yaml"
commands:
  - name: "project-specific"
    description: "Project-specific command"
    steps:
      - echo "project"
`,
				"github-org-config/atmos-common.yaml": `
commands:
  - name: "org-lint"
    description: "Organization-wide lint command"
    steps:
      - echo "Running org linting"
  - name: "org-test"
    description: "Organization-wide test command"
    steps:
      - echo "Running org tests"
  - name: "org-deploy"
    description: "Organization-wide deploy command"
    steps:
      - echo "Running org deployment"
`,
			},
			expectedCommands: []string{"project-specific", "org-lint", "org-test", "org-deploy"},
			description:      "CloudPosse-style: project commands appear first and can override org commands",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a temporary directory for test files.
			tempDir := t.TempDir()

			// Create test files.
			for relativePath, content := range tt.setupFiles {
				fullPath := filepath.Join(tempDir, relativePath)
				dir := filepath.Dir(fullPath)
				err := os.MkdirAll(dir, 0o755)
				require.NoError(t, err)
				err = os.WriteFile(fullPath, []byte(content), 0o644)
				require.NoError(t, err)
			}

			// Change to test directory.
			t.Chdir(tempDir)

			// Load the configuration.
			configInfo := schema.ConfigAndStacksInfo{
				AtmosBasePath:      tempDir,
				AtmosCliConfigPath: filepath.Join(tempDir, "atmos.yaml"),
			}
			cfg, err := InitCliConfig(configInfo, false)
			require.NoError(t, err, tt.description)

			// Verify commands are merged correctly.
			require.NotNil(t, cfg.Commands, "Commands should not be nil for test: %s", tt.name)

			// Extract command names.
			actualCommands := make([]string, len(cfg.Commands))
			for i, cmd := range cfg.Commands {
				actualCommands[i] = cmd.Name
			}

			// Check that all expected commands are present.
			assert.Equal(t, tt.expectedCommands, actualCommands,
				"Test '%s': %s\nExpected commands: %v\nActual commands: %v",
				tt.name, tt.description, tt.expectedCommands, actualCommands)
		})
	}
}

// TestImportCommandMergingEdgeCases tests edge cases in command merging.
func TestImportCommandMergingEdgeCases(t *testing.T) {
	setupTestAdapters()

	tests := []struct {
		name             string
		setupFiles       map[string]string
		expectedCommands []string
		description      string
	}{
		{
			name: "empty_commands_in_import",
			setupFiles: map[string]string{
				"atmos.yaml": `
base_path: "."
import:
  - "imported.yaml"
commands:
  - name: "main-cmd"
    description: "Command from main"
    steps:
      - echo "main"
`,
				"imported.yaml": `
# No commands defined
settings:
  some_setting: true
`,
			},
			expectedCommands: []string{"main-cmd"},
			description:      "Import with no commands should not affect main commands",
		},
		{
			name: "empty_commands_in_main",
			setupFiles: map[string]string{
				"atmos.yaml": `
base_path: "."
import:
  - "imported.yaml"
# No commands in main
`,
				"imported.yaml": `
commands:
  - name: "imported-cmd"
    description: "Command from import"
    steps:
      - echo "imported"
`,
			},
			expectedCommands: []string{"imported-cmd"},
			description:      "Main with no commands should preserve imported commands",
		},
		{
			name: "glob_pattern_imports",
			setupFiles: map[string]string{
				"atmos.yaml": `
base_path: "."
import:
  - "commands/*.yaml"
commands:
  - name: "main-cmd"
    description: "Command from main"
    steps:
      - echo "main"
`,
				"commands/cmd1.yaml": `
commands:
  - name: "cmd1"
    description: "First command"
    steps:
      - echo "cmd1"
`,
				"commands/cmd2.yaml": `
commands:
  - name: "cmd2"
    description: "Second command"
    steps:
      - echo "cmd2"
`,
			},
			expectedCommands: []string{"main-cmd", "cmd1", "cmd2"},
			description:      "Glob pattern imports should merge all matched files' commands (main first)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a temporary directory for test files.
			tempDir := t.TempDir()

			// Create test files.
			for relativePath, content := range tt.setupFiles {
				fullPath := filepath.Join(tempDir, relativePath)
				dir := filepath.Dir(fullPath)
				err := os.MkdirAll(dir, 0o755)
				require.NoError(t, err)
				err = os.WriteFile(fullPath, []byte(content), 0o644)
				require.NoError(t, err)
			}

			// Change to test directory.
			t.Chdir(tempDir)

			// Load the configuration.
			configInfo := schema.ConfigAndStacksInfo{
				AtmosBasePath:      tempDir,
				AtmosCliConfigPath: filepath.Join(tempDir, "atmos.yaml"),
			}
			cfg, err := InitCliConfig(configInfo, false)
			require.NoError(t, err, tt.description)

			// Extract command names.
			actualCommands := make([]string, 0)
			if cfg.Commands != nil {
				for _, cmd := range cfg.Commands {
					actualCommands = append(actualCommands, cmd.Name)
				}
			}

			// Check that all expected commands are present.
			assert.Equal(t, tt.expectedCommands, actualCommands,
				"Test '%s': %s\nExpected commands: %v\nActual commands: %v",
				tt.name, tt.description, tt.expectedCommands, actualCommands)
		})
	}
}

func TestImportCommandMergingNestedCommands(t *testing.T) {
	setupTestAdapters()

	tempDir := t.TempDir()
	files := map[string]string{
		"atmos.yaml": `
base_path: "."
import:
  - "commands/*.yaml"
commands:
  - name: "casts"
    description: "Root command from main"
    commands:
      - name: "setup"
        description: "Prepare fixtures"
        steps:
          - echo setup
`,
		"commands/examples.yaml": `
commands:
  - name: "casts"
    commands:
      - name: "generate"
        commands:
          - name: "examples"
            commands:
              - name: "quick-start-simple"
                commands:
                  - name: "list-and-plan"
                    steps:
                      - echo generate quick-start-simple
`,
		"commands/demo.yaml": `
commands:
  - name: "casts"
    commands:
      - name: "generate"
        commands:
          - name: "demo"
            commands:
              - name: "fixtures"
                commands:
                  - name: "native-terraform"
                    commands:
                      - name: "plan"
                        steps:
                          - echo generate native-terraform plan
      - name: "validate"
        commands:
          - name: "all"
            steps:
              - echo validate all
`,
	}
	for relativePath, content := range files {
		fullPath := filepath.Join(tempDir, relativePath)
		require.NoError(t, os.MkdirAll(filepath.Dir(fullPath), 0o755))
		require.NoError(t, os.WriteFile(fullPath, []byte(content), 0o644))
	}
	t.Chdir(tempDir)

	configInfo := schema.ConfigAndStacksInfo{
		AtmosBasePath:      tempDir,
		AtmosCliConfigPath: filepath.Join(tempDir, "atmos.yaml"),
	}
	cfg, err := InitCliConfig(configInfo, false)
	require.NoError(t, err)
	require.Len(t, cfg.Commands, 1)

	casts := cfg.Commands[0]
	require.Equal(t, "casts", casts.Name)
	assert.Equal(t, "Root command from main", casts.Description)

	setup := findCommand(t, casts.Commands, "setup")
	assert.Equal(t, "Prepare fixtures", setup.Description)

	generate := findCommand(t, casts.Commands, "generate")
	examples := findCommand(t, generate.Commands, "examples")
	quickStartSimple := findCommand(t, examples.Commands, "quick-start-simple")
	findCommand(t, quickStartSimple.Commands, "list-and-plan")

	demo := findCommand(t, generate.Commands, "demo")
	fixtures := findCommand(t, demo.Commands, "fixtures")
	nativeTerraform := findCommand(t, fixtures.Commands, "native-terraform")
	findCommand(t, nativeTerraform.Commands, "plan")

	validate := findCommand(t, casts.Commands, "validate")
	findCommand(t, validate.Commands, "all")
}

func TestImportCommandMergingPathNamesPreserveDeepMerge(t *testing.T) {
	setupTestAdapters()

	tempDir := t.TempDir()
	files := map[string]string{
		"atmos.yaml": `
base_path: "."
import:
  - "base.yaml"
commands:
  - name: "casts generate demo fixtures basic list-stacks"
    description: "Local list stacks"
    steps:
      - echo local list-stacks
  - name: "casts generate demo fixtures native-terraform plan"
    description: "Local native terraform plan"
    steps:
      - echo local native-terraform plan
`,
		"base.yaml": `
commands:
  - name: "casts"
    description: "Base casts root"
    commands:
      - name: "generate"
        description: "Base generate"
        commands:
          - name: "demo"
            commands:
              - name: "fixtures"
                commands:
                  - name: "basic"
                    commands:
                      - name: "list-stacks"
                        description: "Base list stacks"
                        steps:
                          - echo base list-stacks
                      - name: "describe-component"
                        description: "Base describe component"
                        steps:
                          - echo base describe-component
          - name: "examples"
            commands:
              - name: "quick-start-simple"
                steps:
                  - echo base example
      - name: "setup"
        description: "Base setup"
        steps:
          - echo setup
`,
	}
	for relativePath, content := range files {
		fullPath := filepath.Join(tempDir, relativePath)
		require.NoError(t, os.MkdirAll(filepath.Dir(fullPath), 0o755))
		require.NoError(t, os.WriteFile(fullPath, []byte(content), 0o644))
	}
	t.Chdir(tempDir)

	configInfo := schema.ConfigAndStacksInfo{
		AtmosBasePath:      tempDir,
		AtmosCliConfigPath: filepath.Join(tempDir, "atmos.yaml"),
	}
	cfg, err := InitCliConfig(configInfo, false)
	require.NoError(t, err)
	require.Len(t, cfg.Commands, 1)

	casts := cfg.Commands[0]
	require.Equal(t, "casts", casts.Name)
	assert.Equal(t, "Base casts root", casts.Description)
	findCommand(t, casts.Commands, "setup")

	generate := findCommand(t, casts.Commands, "generate")
	assert.Equal(t, "Base generate", generate.Description)
	examples := findCommand(t, generate.Commands, "examples")
	findCommand(t, examples.Commands, "quick-start-simple")

	demo := findCommand(t, generate.Commands, "demo")
	fixtures := findCommand(t, demo.Commands, "fixtures")
	basic := findCommand(t, fixtures.Commands, "basic")
	listStacks := findCommand(t, basic.Commands, "list-stacks")
	describeComponent := findCommand(t, basic.Commands, "describe-component")
	assert.Equal(t, "Local list stacks", listStacks.Description)
	require.Len(t, listStacks.Steps, 1)
	assert.Equal(t, "echo local list-stacks", listStacks.Steps[0].Command)
	assert.Equal(t, "Base describe component", describeComponent.Description)

	nativeTerraform := findCommand(t, fixtures.Commands, "native-terraform")
	plan := findCommand(t, nativeTerraform.Commands, "plan")
	assert.Equal(t, "Local native terraform plan", plan.Description)
	require.Len(t, plan.Steps, 1)
	assert.Equal(t, "echo local native-terraform plan", plan.Steps[0].Command)
}

func findCommand(t *testing.T, commands []schema.Command, name string) schema.Command {
	t.Helper()
	for i := range commands {
		command := commands[i]
		if command.Name == name {
			return command
		}
	}
	t.Fatalf("command %q not found", name)
	return schema.Command{}
}
