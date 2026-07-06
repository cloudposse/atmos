package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeCommandArrayPathNames(t *testing.T) {
	commands := []interface{}{
		map[string]interface{}{
			"name":        "casts generate demo",
			"description": "Demo command",
		},
	}

	normalized := normalizeCommandArray(commands)
	require.Len(t, normalized, 1)

	casts := requireCommandMap(t, normalized, "casts")
	generate := requireCommandMap(t, casts[commandsKey], "generate")
	demo := requireCommandMap(t, generate[commandsKey], "demo")
	assert.Equal(t, "Demo command", demo["description"])
}

func TestNormalizeCommandArrayPreservesNestedCommands(t *testing.T) {
	commands := []interface{}{
		map[string]interface{}{
			"name":        "casts",
			"description": "Casts root",
			commandsKey: []interface{}{
				map[string]interface{}{
					"name":        "generate",
					"description": "Generate casts",
				},
			},
		},
	}

	normalized := normalizeCommandArray(commands)
	require.Len(t, normalized, 1)

	casts := requireCommandMap(t, normalized, "casts")
	assert.Equal(t, "Casts root", casts["description"])
	generate := requireCommandMap(t, casts[commandsKey], "generate")
	assert.Equal(t, "Generate casts", generate["description"])
}

func TestNormalizeCommandArrayPathWithNestedChildren(t *testing.T) {
	commands := []interface{}{
		map[string]interface{}{
			"name": "casts generate",
			commandsKey: []interface{}{
				map[string]interface{}{
					"name":        "demo",
					"description": "Demo child",
				},
			},
		},
	}

	normalized := normalizeCommandArray(commands)
	require.Len(t, normalized, 1)

	casts := requireCommandMap(t, normalized, "casts")
	generate := requireCommandMap(t, casts[commandsKey], "generate")
	demo := requireCommandMap(t, generate[commandsKey], "demo")
	assert.Equal(t, "Demo child", demo["description"])
}

func TestMergeCommandArraysPathNamesSharePrefix(t *testing.T) {
	commands := []interface{}{
		map[string]interface{}{
			"name":        "casts generate demo",
			"description": "Demo command",
		},
		map[string]interface{}{
			"name":        "casts generate examples",
			"description": "Examples command",
		},
	}

	merged := mergeCommandArrays(nil, commands)
	require.Len(t, merged, 1)

	casts := requireCommandMap(t, merged, "casts")
	generate := requireCommandMap(t, casts[commandsKey], "generate")
	demo := requireCommandMap(t, generate[commandsKey], "demo")
	examples := requireCommandMap(t, generate[commandsKey], "examples")
	assert.Equal(t, "Demo command", demo["description"])
	assert.Equal(t, "Examples command", examples["description"])
}

func TestMergeConfigFilePathCommandsAcrossFiles(t *testing.T) {
	tempDir := t.TempDir()
	files := map[string]string{
		"basic.yaml": `
commands:
  - name: casts generate demo fixtures basic list-stacks
    description: List stacks
`,
		"native.yaml": `
commands:
  - name: casts generate demo fixtures native-terraform plan
    description: Terraform plan
`,
	}
	for name, content := range files {
		require.NoError(t, os.WriteFile(filepath.Join(tempDir, name), []byte(content), 0o644))
	}

	v := viper.New()
	v.SetConfigType(yamlType)
	require.NoError(t, mergeConfigFile(filepath.Join(tempDir, "basic.yaml"), v))
	require.NoError(t, mergeConfigFile(filepath.Join(tempDir, "native.yaml"), v))

	casts := requireCommandMap(t, v.Get(commandsKey), "casts")
	generate := requireCommandMap(t, casts[commandsKey], "generate")
	demo := requireCommandMap(t, generate[commandsKey], "demo")
	fixtures := requireCommandMap(t, demo[commandsKey], "fixtures")
	basic := requireCommandMap(t, fixtures[commandsKey], "basic")
	listStacks := requireCommandMap(t, basic[commandsKey], "list-stacks")
	nativeTerraform := requireCommandMap(t, fixtures[commandsKey], "native-terraform")
	plan := requireCommandMap(t, nativeTerraform[commandsKey], "plan")
	assert.Equal(t, "List stacks", listStacks["description"])
	assert.Equal(t, "Terraform plan", plan["description"])
}

func TestLoadAtmosDPathCommandsPreservesSiblings(t *testing.T) {
	tempDir := t.TempDir()
	atmosD := filepath.Join(tempDir, "atmos.d")
	files := map[string]string{
		"all.yaml": `
commands:
  - name: casts
    description: Root command
    commands:
      - name: generate
        commands:
          - name: cli
            description: CLI casts
`,
		"examples/quick-start-simple.yaml": `
commands:
  - name: casts generate examples quick-start-simple list-and-plan
    description: Example cast
`,
		"demo/fixtures/basic.yaml": `
commands:
  - name: casts generate demo fixtures basic list-stacks
    description: Fixture cast
`,
	}
	for name, content := range files {
		path := filepath.Join(atmosD, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}

	v := viper.New()
	v.SetConfigType(yamlType)

	absAtmosD, err := filepath.Abs(atmosD)
	require.NoError(t, err)
	pattern := filepath.Join(absAtmosD, "**", "*")
	require.NoError(t, loadAtmosConfigsFromDirectory(pattern, v, "test atmos.d"))

	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("commands: %#v", v.Get(commandsKey))
		}
	})
	casts := requireCommandMap(t, v.Get(commandsKey), "casts")
	generate := requireCommandMap(t, casts[commandsKey], "generate")
	requireCommandMap(t, generate[commandsKey], "cli")
	requireCommandMap(t, generate[commandsKey], "examples")
	demo := requireCommandMap(t, generate[commandsKey], "demo")
	fixtures := requireCommandMap(t, demo[commandsKey], "fixtures")
	basic := requireCommandMap(t, fixtures[commandsKey], "basic")
	requireCommandMap(t, basic[commandsKey], "list-stacks")
}

func TestMergeCommandArraysPathLeafOverridesNestedAndPreservesSiblings(t *testing.T) {
	base := []interface{}{
		map[string]interface{}{
			"name": "casts",
			commandsKey: []interface{}{
				map[string]interface{}{
					"name": "generate",
					commandsKey: []interface{}{
						map[string]interface{}{
							"name":        "demo",
							"description": "Base demo",
						},
						map[string]interface{}{
							"name":        "examples",
							"description": "Base examples",
						},
					},
				},
				map[string]interface{}{
					"name":        "setup",
					"description": "Base setup",
				},
			},
		},
	}
	local := []interface{}{
		map[string]interface{}{
			"name":        "casts generate demo",
			"description": "Local demo",
		},
	}

	merged := mergeCommandArrays(base, local)
	require.Len(t, merged, 1)

	casts := requireCommandMap(t, merged, "casts")
	findCommandMap(t, casts[commandsKey], "setup")
	generate := requireCommandMap(t, casts[commandsKey], "generate")
	demo := requireCommandMap(t, generate[commandsKey], "demo")
	examples := requireCommandMap(t, generate[commandsKey], "examples")
	assert.Equal(t, "Local demo", demo["description"])
	assert.Equal(t, "Base examples", examples["description"])
}

func requireCommandMap(t *testing.T, commands interface{}, name string) map[string]interface{} {
	t.Helper()
	command := findCommandMap(t, commands, name)
	require.NotNil(t, command, "command %q not found", name)
	return command
}

func findCommandMap(t *testing.T, commands interface{}, name string) map[string]interface{} {
	t.Helper()
	cmdSlice, ok := commands.([]interface{})
	require.True(t, ok, "commands should be []interface{}")
	for _, cmd := range cmdSlice {
		cmdMap, ok := cmd.(map[string]interface{})
		if !ok {
			continue
		}
		if cmdMap["name"] == name {
			return cmdMap
		}
	}
	return nil
}

// TestCommandMergeCore validates the core command merging functionality,
// ensuring that commands from imported configurations are properly merged
// with local commands, and that local commands can override imported ones.
func TestCommandMergeCore(t *testing.T) {
	setupTestAdapters()

	tests := []struct {
		name        string
		setupFiles  map[string]string
		verifyFunc  func(t *testing.T, commands []schema.Command)
		description string
	}{
		{
			name: "basic_import_merging",
			setupFiles: map[string]string{
				"atmos.yaml": `
base_path: "."
import:
  - "import.yaml"
commands:
  - name: "local-cmd"
    description: "Local command"
`,
				"import.yaml": `
commands:
  - name: "imported-cmd"
    description: "Imported command"
`,
			},
			verifyFunc: func(t *testing.T, commands []schema.Command) {
				assert.Len(t, commands, 2, "Should have both imported and local commands")

				names := make(map[string]string)
				for _, cmd := range commands {
					names[cmd.Name] = cmd.Description
				}

				assert.Equal(t, "Local command", names["local-cmd"])
				assert.Equal(t, "Imported command", names["imported-cmd"])
			},
			description: "Basic import: imports are merged with local",
		},
		{
			name: "local_overrides_imported",
			setupFiles: map[string]string{
				"atmos.yaml": `
base_path: "."
import:
  - "import.yaml"
commands:
  - name: "shared"
    description: "Local wins"
  - name: "local-only"
    description: "Local only"
`,
				"import.yaml": `
commands:
  - name: "shared"
    description: "Import loses"
  - name: "import-only"
    description: "Import only"
`,
			},
			verifyFunc: func(t *testing.T, commands []schema.Command) {
				assert.Len(t, commands, 3, "Should have 3 unique commands")

				names := make(map[string]string)
				for _, cmd := range commands {
					names[cmd.Name] = cmd.Description
				}

				// Local should win for duplicate
				assert.Equal(t, "Local wins", names["shared"], "Local command should override imported")
				assert.Equal(t, "Local only", names["local-only"])
				assert.Equal(t, "Import only", names["import-only"])
			},
			description: "Override behavior: local overrides imported on name conflict",
		},
		{
			name: "deep_nesting_works",
			setupFiles: map[string]string{
				"atmos.yaml": `
base_path: "."
import:
  - "l1.yaml"
commands:
  - name: "main"
    description: "Main"
`,
				"l1.yaml": `
import:
  - "l2.yaml"
commands:
  - name: "l1"
    description: "Level 1"
`,
				"l2.yaml": `
import:
  - "l3.yaml"
commands:
  - name: "l2"
    description: "Level 2"
`,
				"l3.yaml": `
commands:
  - name: "l3"
    description: "Level 3"
`,
			},
			verifyFunc: func(t *testing.T, commands []schema.Command) {
				assert.Len(t, commands, 4, "Should have all 4 commands from all levels")

				names := make(map[string]bool)
				t.Cleanup(func() {
					if t.Failed() {
						for _, cmd := range commands {
							t.Logf("  Found: %s - %s", cmd.Name, cmd.Description)
						}
					}
				})
				for _, cmd := range commands {
					names[cmd.Name] = true
				}

				assert.True(t, names["main"], "Main command present")
				assert.True(t, names["l1"], "Level 1 command present")
				assert.True(t, names["l2"], "Level 2 command present")
				assert.True(t, names["l3"], "Level 3 command present")
			},
			description: "Deep nesting: commands from all import levels are included",
		},
		{
			name: "ten_plus_one_equals_eleven",
			setupFiles: map[string]string{
				"atmos.yaml": `
base_path: "."
import:
  - "upstream.yaml"
commands:
  - name: "my-local-cmd"
    description: "My local command"
`,
				"upstream.yaml": `
commands:
  - name: "upstream-1"
    description: "Upstream command 1"
  - name: "upstream-2"
    description: "Upstream command 2"
  - name: "upstream-3"
    description: "Upstream command 3"
  - name: "upstream-4"
    description: "Upstream command 4"
  - name: "upstream-5"
    description: "Upstream command 5"
  - name: "upstream-6"
    description: "Upstream command 6"
  - name: "upstream-7"
    description: "Upstream command 7"
  - name: "upstream-8"
    description: "Upstream command 8"
  - name: "upstream-9"
    description: "Upstream command 9"
  - name: "upstream-10"
    description: "Upstream command 10"
`,
			},
			verifyFunc: func(t *testing.T, commands []schema.Command) {
				assert.Len(t, commands, 11, "10 upstream + 1 local = 11 total")

				foundLocal := false
				upstreamCount := 0
				for _, cmd := range commands {
					if cmd.Name == "my-local-cmd" {
						foundLocal = true
					} else if strings.HasPrefix(cmd.Name, "upstream") {
						upstreamCount++
					}
				}

				assert.True(t, foundLocal, "Local command should be present")
				assert.Equal(t, 10, upstreamCount, "All 10 upstream commands should be present")
			},
			description: "Real-world scenario: upstream has 10, local adds 1 = 11 total",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			tempDir := t.TempDir()

			for path, content := range tt.setupFiles {
				fullPath := filepath.Join(tempDir, path)
				err := os.WriteFile(fullPath, []byte(content), 0o644)
				require.NoError(t, err)
			}

			t.Chdir(tempDir)

			// Load config
			configInfo := schema.ConfigAndStacksInfo{
				AtmosBasePath:      tempDir,
				AtmosCliConfigPath: filepath.Join(tempDir, "atmos.yaml"),
			}
			cfg, err := InitCliConfig(configInfo, false)
			require.NoError(t, err, tt.description)

			// Verify
			t.Cleanup(func() {
				if t.Failed() {
					t.Logf("\n%s: %s", tt.name, tt.description)
				}
			})
			tt.verifyFunc(t, cfg.Commands)
		})
	}
}
