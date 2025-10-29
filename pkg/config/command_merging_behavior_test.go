package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCommandMergingBehavior tests the exact command merging behavior.
// This test clarifies:
// 1. Remote/imported configs provide a set of commands.
// 2. Local config can add more commands.
// 3. If local has a duplicate name, local should win.
func TestCommandMergingBehavior(t *testing.T) {
	tests := []struct {
		name          string
		setupFiles    map[string]string
		expectedCount int
		checkCommands func(t *testing.T, commands []schema.Command)
	}{
		{
			name: "remote_has_10_local_has_1_equals_11",
			setupFiles: map[string]string{
				"atmos.yaml": `
base_path: "."
import:
  - "remote.yaml"
commands:
  - name: "local-cmd"
    description: "My local command"
    steps:
      - echo "local"
`,
				"remote.yaml": `
commands:
  - name: "remote-cmd1"
    description: "Remote command 1"
    steps:
      - echo "remote1"
  - name: "remote-cmd2"
    description: "Remote command 2"
    steps:
      - echo "remote2"
  - name: "remote-cmd3"
    description: "Remote command 3"
    steps:
      - echo "remote3"
  - name: "remote-cmd4"
    description: "Remote command 4"
    steps:
      - echo "remote4"
  - name: "remote-cmd5"
    description: "Remote command 5"
    steps:
      - echo "remote5"
  - name: "remote-cmd6"
    description: "Remote command 6"
    steps:
      - echo "remote6"
  - name: "remote-cmd7"
    description: "Remote command 7"
    steps:
      - echo "remote7"
  - name: "remote-cmd8"
    description: "Remote command 8"
    steps:
      - echo "remote8"
  - name: "remote-cmd9"
    description: "Remote command 9"
    steps:
      - echo "remote9"
  - name: "remote-cmd10"
    description: "Remote command 10"
    steps:
      - echo "remote10"
`,
			},
			expectedCount: 11, // 10 remote + 1 local = 11 total.
			checkCommands: func(t *testing.T, commands []schema.Command) {
				// All 11 commands should be present.
				names := make(map[string]bool)
				for _, cmd := range commands {
					names[cmd.Name] = true
				}

				// Check all remote commands are present.
				for i := 1; i <= 10; i++ {
					cmdName := fmt.Sprintf("remote-cmd%d", i)
					assert.True(t, names[cmdName], "Remote command %s should be present", cmdName)
				}

				// Check local command is present.
				assert.True(t, names["local-cmd"], "Local command should be present")
			},
		},
		{
			name: "local_overrides_duplicate_remote",
			setupFiles: map[string]string{
				"atmos.yaml": `
base_path: "."
import:
  - "remote.yaml"
commands:
  - name: "shared-cmd"
    description: "LOCAL version of shared command"
    steps:
      - echo "LOCAL VERSION"
  - name: "local-only"
    description: "Local only command"
    steps:
      - echo "local only"
`,
				"remote.yaml": `
commands:
  - name: "shared-cmd"
    description: "REMOTE version of shared command"
    steps:
      - echo "REMOTE VERSION"
  - name: "remote-only"
    description: "Remote only command"
    steps:
      - echo "remote only"
`,
			},
			expectedCount: 3, // shared-cmd (local wins), local-only, remote-only.
			checkCommands: func(t *testing.T, commands []schema.Command) {
				// Find the shared command and verify it's the LOCAL version.
				var sharedCmd *schema.Command
				cmdMap := make(map[string]*schema.Command)

				for i := range commands {
					cmdMap[commands[i].Name] = &commands[i]
					if commands[i].Name == "shared-cmd" {
						sharedCmd = &commands[i]
					}
				}

				require.NotNil(t, sharedCmd, "shared-cmd should exist")

				// The local version should win (check description to verify).
				assert.Equal(t, "LOCAL version of shared command", sharedCmd.Description,
					"Local command should override remote command with same name")

				// Verify we have all expected commands.
				assert.NotNil(t, cmdMap["local-only"], "local-only command should exist")
				assert.NotNil(t, cmdMap["remote-only"], "remote-only command should exist")

				// Capture command details for debugging on failure.
				t.Cleanup(func() {
					if t.Failed() {
						t.Log("\nActual commands:")
						for _, cmd := range commands {
							t.Logf("  - %s: %s", cmd.Name, cmd.Description)
						}
					}
				})
			},
		},
		{
			name: "deep_import_chain_4_levels",
			setupFiles: map[string]string{
				"atmos.yaml": `
base_path: "."
import:
  - "level1.yaml"
commands:
  - name: "main-cmd"
    description: "Main level command"
    steps:
      - echo "main"
`,
				"level1.yaml": `
import:
  - "level2.yaml"
commands:
  - name: "level1-cmd"
    description: "Level 1 command"
    steps:
      - echo "level1"
`,
				"level2.yaml": `
import:
  - "level3.yaml"
commands:
  - name: "level2-cmd"
    description: "Level 2 command"
    steps:
      - echo "level2"
`,
				"level3.yaml": `
import:
  - "level4.yaml"
commands:
  - name: "level3-cmd"
    description: "Level 3 command"
    steps:
      - echo "level3"
`,
				"level4.yaml": `
commands:
  - name: "level4-cmd"
    description: "Level 4 command (deepest)"
    steps:
      - echo "level4"
`,
			},
			expectedCount: 5, // One command from each level.
			checkCommands: func(t *testing.T, commands []schema.Command) {
				// All 5 commands from all levels should be present.
				cmdMap := make(map[string]bool)
				for _, cmd := range commands {
					cmdMap[cmd.Name] = true
				}

				assert.True(t, cmdMap["main-cmd"], "Main command should be present")
				assert.True(t, cmdMap["level1-cmd"], "Level 1 command should be present")
				assert.True(t, cmdMap["level2-cmd"], "Level 2 command should be present")
				assert.True(t, cmdMap["level3-cmd"], "Level 3 command should be present")
				assert.True(t, cmdMap["level4-cmd"], "Level 4 command should be present")

				t.Cleanup(func() {
					if t.Failed() {
						t.Log("\nCommands from 4-level deep import:")
						for _, cmd := range commands {
							t.Logf("  - %s: %s", cmd.Name, cmd.Description)
						}
					}
				})
			},
		},
		{
			name: "complex_nested_command_structure",
			setupFiles: map[string]string{
				"atmos.yaml": `
base_path: "."
import:
  - "base.yaml"
commands:
  - name: "complex-cmd"
    description: "A complex command with nested structure"
    steps:
      - "echo 'Step 1'"
      - "echo 'Step 2'"
    env:
      - KEY1: "value1"
      - KEY2: "value2"
    arguments:
      - name: "arg1"
        description: "First argument"
    flags:
      - name: "verbose"
        shorthand: "v"
        description: "Verbose output"
`,
				"base.yaml": `
commands:
  - name: "base-complex-cmd"
    description: "Base complex command"
    steps:
      - "terraform plan"
      - "terraform apply"
    verbose: true
`,
			},
			expectedCount: 2,
			checkCommands: func(t *testing.T, commands []schema.Command) {
				assert.Len(t, commands, 2, "Should have both commands")

				// Verify complex nested structures are preserved.
				t.Cleanup(func() {
					if t.Failed() {
						for _, cmd := range commands {
							t.Logf("\nCommand '%s' structure:", cmd.Name)
							t.Logf("  Description: %s", cmd.Description)
							if len(cmd.Steps) > 0 {
								t.Logf("  Steps: %d", len(cmd.Steps))
							}
							if len(cmd.Env) > 0 {
								t.Logf("  Env vars: %d", len(cmd.Env))
							}
							if len(cmd.Arguments) > 0 {
								t.Logf("  Arguments: %d", len(cmd.Arguments))
							}
							if len(cmd.Flags) > 0 {
								t.Logf("  Flags: %d", len(cmd.Flags))
							}
						}
					}
				})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp directory.
			tempDir := t.TempDir()

			// Create test files.
			for path, content := range tt.setupFiles {
				fullPath := filepath.Join(tempDir, path)
				err := os.WriteFile(fullPath, []byte(content), 0o644)
				require.NoError(t, err)
			}

			// Change to test directory.
			t.Chdir(tempDir)

			// Load configuration.
			configInfo := schema.ConfigAndStacksInfo{
				AtmosBasePath:      tempDir,
				AtmosCliConfigPath: filepath.Join(tempDir, "atmos.yaml"),
			}
			cfg, err := InitCliConfig(configInfo, false)
			require.NoError(t, err)

			// Check results.
			assert.Len(t, cfg.Commands, tt.expectedCount,
				"Test %s: Should have %d commands", tt.name, tt.expectedCount)

			if tt.checkCommands != nil {
				tt.checkCommands(t, cfg.Commands)
			}
		})
	}
}

// TestCommandStructurePreservation verifies that complex command structures are preserved through merging.
func TestCommandStructurePreservation(t *testing.T) {
	// Create temp directory.
	tempDir := t.TempDir()

	// Create a file with a complex command structure.
	complexYAML := `
commands:
  - name: "terraform-plan"
    description: "Run terraform plan with all the bells and whistles"
    steps:
      - "terraform init"
      - "terraform validate"
      - "terraform plan -out=tfplan"
    env:
      - TF_VAR_region: "us-east-1"
      - TF_VAR_env: "dev"
    arguments:
      - name: "component"
        description: "Component to plan"
        required: true
      - name: "stack"
        description: "Stack to use"
        required: true
    flags:
      - name: "dry-run"
        shorthand: "d"
        description: "Dry run mode"
        type: "bool"
        value: "false"
    component_config:
      component_path: "components/terraform"
    verbose: true
`

	err := os.WriteFile(filepath.Join(tempDir, "atmos.yaml"), []byte(complexYAML), 0o644)
	require.NoError(t, err)

	// Load and verify.
	t.Chdir(tempDir)

	configInfo := schema.ConfigAndStacksInfo{
		AtmosBasePath:      tempDir,
		AtmosCliConfigPath: filepath.Join(tempDir, "atmos.yaml"),
	}
	cfg, err := InitCliConfig(configInfo, false)
	require.NoError(t, err)

	// Verify the complex structure is preserved.
	require.Len(t, cfg.Commands, 1)
	cmd := cfg.Commands[0]

	assert.Equal(t, "terraform-plan", cmd.Name)
	assert.Equal(t, "Run terraform plan with all the bells and whistles", cmd.Description)
	assert.Len(t, cmd.Steps, 3, "Should have 3 steps")
	assert.Len(t, cmd.Env, 2, "Should have 2 env vars")
	assert.Len(t, cmd.Arguments, 2, "Should have 2 arguments")
	assert.Len(t, cmd.Flags, 1, "Should have 1 flag")
	assert.True(t, cmd.Verbose)
}
