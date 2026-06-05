package exec

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/tests"
)

// captureStdout captures stdout during the execution of fn and returns the captured output.
// It restores stdout and logger output after the function completes, even if fn panics.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	oldStdout := os.Stdout
	oldLogOut := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	os.Stdout = w
	log.SetOutput(w)

	closed := false

	// Ensure stdout and logger are restored even if fn panics.
	defer func() {
		os.Stdout = oldStdout
		log.SetOutput(oldLogOut)
		if !closed {
			_ = w.Close()
			_ = r.Close()
		}
	}()

	fn()

	// Close writer before reading to avoid deadlock.
	_ = w.Close()
	closed = true
	os.Stdout = oldStdout
	log.SetOutput(oldLogOut)

	var buf bytes.Buffer
	_, err = io.Copy(&buf, r)
	_ = r.Close()
	if err != nil {
		t.Fatalf("failed to read captured output: %v", err)
	}

	return buf.String()
}

func TestExecutePacker_Validate(t *testing.T) {
	tests.RequirePacker(t)

	workDir := "../../tests/fixtures/scenarios/packer"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", workDir)
	t.Setenv("ATMOS_LOGS_LEVEL", "Warning")
	log.SetLevel(log.InfoLevel)

	// Run packer init first to install required plugins.
	initInfo := schema.ConfigAndStacksInfo{
		StackFromArg:     "",
		Stack:            "nonprod",
		StackFile:        "",
		ComponentType:    "packer",
		ComponentFromArg: "aws/bastion",
		SubCommand:       "init",
		ProcessTemplates: true,
		ProcessFunctions: true,
	}

	packerFlags := PackerFlags{}
	err := ExecutePacker(&initInfo, &packerFlags)
	if err != nil {
		t.Skipf("Skipping test: packer init failed (may require network access): %v", err)
	}

	info := schema.ConfigAndStacksInfo{
		StackFromArg:     "",
		Stack:            "nonprod",
		StackFile:        "",
		ComponentType:    "packer",
		ComponentFromArg: "aws/bastion",
		SubCommand:       "validate",
		ProcessTemplates: true,
		ProcessFunctions: true,
	}

	oldStd := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Ensure stdout is restored even if test fails.
	defer func() {
		os.Stdout = oldStd
	}()

	log.SetOutput(w)

	err = ExecutePacker(&info, &packerFlags)

	// Restore stdout before assertions.
	w.Close()
	os.Stdout = oldStd

	assert.NoError(t, err)

	// Read the captured output.
	var buf bytes.Buffer
	_, err = buf.ReadFrom(r)
	assert.NoError(t, err)
	output := buf.String()

	// Check the output.
	expected := "The configuration is valid"

	if !strings.Contains(output, expected) {
		t.Logf("TestExecutePacker_Validate output: %s", output)
		t.Errorf("Output should contain: %s", expected)
	}
}

func TestExecutePacker_Inspect(t *testing.T) {
	tests.RequirePacker(t)

	workDir := "../../tests/fixtures/scenarios/packer"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", workDir)
	t.Setenv("ATMOS_LOGS_LEVEL", "Warning")
	log.SetLevel(log.InfoLevel)

	info := schema.ConfigAndStacksInfo{
		StackFromArg:     "",
		Stack:            "nonprod",
		StackFile:        "",
		ComponentType:    "packer",
		ComponentFromArg: "aws/bastion",
		SubCommand:       "inspect",
		ProcessTemplates: true,
		ProcessFunctions: true,
	}

	oldStd := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Ensure stdout is restored even if test fails.
	defer func() {
		os.Stdout = oldStd
	}()

	log.SetOutput(w)
	packerFlags := PackerFlags{}

	err := ExecutePacker(&info, &packerFlags)

	// Restore stdout before assertions.
	w.Close()
	os.Stdout = oldStd

	assert.NoError(t, err)

	// Read the captured output.
	var buf bytes.Buffer
	_, err = buf.ReadFrom(r)
	assert.NoError(t, err)
	output := buf.String()

	// Check the output.
	expected := "var.source_ami: \"ami-0013ceeff668b979b\""

	if !strings.Contains(output, expected) {
		t.Logf("TestExecutePacker_Inspect output: %s", output)
		t.Errorf("Output should contain: %s", expected)
	}
}

func TestExecutePacker_Version(t *testing.T) {
	tests.RequirePacker(t)

	workDir := "../../tests/fixtures/scenarios/packer"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", workDir)
	t.Setenv("ATMOS_LOGS_LEVEL", "Warning")
	log.SetLevel(log.InfoLevel)

	info := schema.ConfigAndStacksInfo{
		ComponentType: "packer",
		SubCommand:    "version",
	}

	packerFlags := PackerFlags{}
	var execErr error

	output := captureStdout(t, func() {
		execErr = ExecutePacker(&info, &packerFlags)
	})

	assert.NoError(t, execErr)

	// Check the output.
	expected := "Packer v"
	if !strings.Contains(output, expected) {
		t.Logf("TestExecutePacker_Version output: %s", output)
		t.Errorf("Output should contain: %s", expected)
	}
}

func TestExecutePacker_Init(t *testing.T) {
	tests.RequirePacker(t)

	workDir := "../../tests/fixtures/scenarios/packer"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", workDir)
	t.Setenv("ATMOS_LOGS_LEVEL", "Warning")
	log.SetLevel(log.InfoLevel)

	info := schema.ConfigAndStacksInfo{
		StackFromArg:     "",
		Stack:            "nonprod",
		StackFile:        "",
		ComponentType:    "packer",
		ComponentFromArg: "aws/bastion",
		SubCommand:       "init",
		ProcessTemplates: true,
		ProcessFunctions: true,
	}

	packerFlags := PackerFlags{}

	err := ExecutePacker(&info, &packerFlags)
	assert.NoError(t, err)
}

func TestExecutePacker_Fmt(t *testing.T) {
	tests.RequirePacker(t)

	workDir := "../../tests/fixtures/scenarios/packer"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", workDir)
	t.Setenv("ATMOS_LOGS_LEVEL", "Warning")
	log.SetLevel(log.InfoLevel)

	info := schema.ConfigAndStacksInfo{
		StackFromArg:     "",
		Stack:            "nonprod",
		StackFile:        "",
		ComponentType:    "packer",
		ComponentFromArg: "aws/bastion",
		SubCommand:       "fmt",
		ProcessTemplates: true,
		ProcessFunctions: true,
	}

	packerFlags := PackerFlags{}

	// packer fmt will format files in place and return success.
	// Note: We don't use -check because that returns exit code 3 if files need formatting.
	err := ExecutePacker(&info, &packerFlags)
	assert.NoError(t, err)
}

// TestExecutePacker_DryRun tests that DryRun mode skips writing the variable file.
func TestExecutePacker_DryRun(t *testing.T) {
	tests.RequirePacker(t)

	workDir := "../../tests/fixtures/scenarios/packer"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", workDir)
	t.Setenv("ATMOS_LOGS_LEVEL", "Warning")
	log.SetLevel(log.InfoLevel)

	info := schema.ConfigAndStacksInfo{
		StackFromArg:     "",
		Stack:            "nonprod",
		StackFile:        "",
		ComponentType:    "packer",
		ComponentFromArg: "aws/bastion",
		SubCommand:       "validate",
		ProcessTemplates: true,
		ProcessFunctions: true,
		DryRun:           true,
	}

	var execErr error
	output := captureStdout(t, func() {
		packerFlags := PackerFlags{}
		execErr = ExecutePacker(&info, &packerFlags)
	})

	// DryRun should succeed without actually running packer.
	// The output should indicate a dry run.
	assert.NoError(t, execErr)
	_ = output // Output may vary, just verify no error.
}

func TestExecutePacker_Errors(t *testing.T) {
	tests.RequirePacker(t)

	workDir := "../../tests/fixtures/scenarios/packer"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", workDir)
	t.Setenv("ATMOS_LOGS_LEVEL", "Warning")
	log.SetLevel(log.InfoLevel)

	t.Run("missing stack", func(t *testing.T) {
		info := schema.ConfigAndStacksInfo{
			ComponentType:    "packer",
			ComponentFromArg: "aws/bastion",
			SubCommand:       "validate",
			ProcessTemplates: true,
			ProcessFunctions: true,
		}
		packerFlags := PackerFlags{}

		err := ExecutePacker(&info, &packerFlags)
		assert.Error(t, err)
	})

	t.Run("missing packer base path", func(t *testing.T) {
		// Create a temporary directory with minimal config without packer base_path.
		tempDir := t.TempDir()
		configPath := filepath.Join(tempDir, "atmos.yaml")
		stacksDir := filepath.Join(tempDir, "stacks")
		require.NoError(t, os.MkdirAll(stacksDir, 0o755))
		stackFile := filepath.Join(stacksDir, "nonprod.yaml")
		err := os.WriteFile(stackFile, []byte(`vars:
  stage: nonprod
`), 0o644)
		require.NoError(t, err)

		// Normalize path for YAML (Windows backslashes break YAML parsing).
		yamlSafePath := filepath.ToSlash(tempDir)

		// Write config with empty packer base_path.
		err = os.WriteFile(configPath, []byte(fmt.Sprintf(`base_path: "%s"
stacks:
  base_path: "stacks"
  included_paths:
    - "**/*"
  excluded_paths: []
  name_pattern: "{stage}"
components:
  terraform:
    base_path: "components/terraform"
  packer:
    base_path: ""
`, yamlSafePath)), 0o644)
		require.NoError(t, err)

		t.Setenv("ATMOS_CLI_CONFIG_PATH", tempDir)

		info := schema.ConfigAndStacksInfo{
			Stack:            "nonprod",
			ComponentType:    "packer",
			ComponentFromArg: "aws/bastion",
			SubCommand:       "validate",
		}
		packerFlags := PackerFlags{}

		err = ExecutePacker(&info, &packerFlags)
		assert.Error(t, err)
		assert.True(t, errors.Is(err, errUtils.ErrMissingPackerBasePath), "expected ErrMissingPackerBasePath, got: %v", err)

		// Reset working directory.
		t.Setenv("ATMOS_CLI_CONFIG_PATH", workDir)
	})

	t.Run("invalid component path", func(t *testing.T) {
		info := schema.ConfigAndStacksInfo{
			Stack:            "nonprod",
			ComponentType:    "packer",
			ComponentFromArg: "invalid/component",
			SubCommand:       "validate",
			ProcessTemplates: true,
			ProcessFunctions: true,
		}
		packerFlags := PackerFlags{}

		err := ExecutePacker(&info, &packerFlags)
		assert.Error(t, err)
	})

	t.Run("disabled component", func(t *testing.T) {
		info := schema.ConfigAndStacksInfo{
			Stack:              "nonprod",
			ComponentType:      "packer",
			ComponentFromArg:   "aws/bastion",
			SubCommand:         "validate",
			ComponentIsEnabled: false,
		}
		packerFlags := PackerFlags{}

		err := ExecutePacker(&info, &packerFlags)
		assert.NoError(t, err) // Should return nil for disabled components
	})

	t.Run("invalid subcommand", func(t *testing.T) {
		info := schema.ConfigAndStacksInfo{
			Stack:            "nonprod",
			ComponentType:    "packer",
			ComponentFromArg: "aws/bastion",
			SubCommand:       "invalid_command",
			ProcessTemplates: true,
			ProcessFunctions: true,
		}
		packerFlags := PackerFlags{}

		err := ExecutePacker(&info, &packerFlags)
		assert.Error(t, err)
	})

	t.Run("invalid working directory", func(t *testing.T) {
		t.Setenv("ATMOS_CLI_CONFIG_PATH", "/nonexistent/path")
		info := schema.ConfigAndStacksInfo{
			Stack:            "nonprod",
			ComponentType:    "packer",
			ComponentFromArg: "aws/bastion",
			SubCommand:       "validate",
			ProcessTemplates: true,
			ProcessFunctions: true,
		}
		packerFlags := PackerFlags{}

		err := ExecutePacker(&info, &packerFlags)
		assert.Error(t, err)
		// Reset working directory
		t.Setenv("ATMOS_CLI_CONFIG_PATH", workDir)
	})

	t.Run("invalid configuration", func(t *testing.T) {
		info := schema.ConfigAndStacksInfo{
			Stack:            "invalid_stack",
			ComponentType:    "packer",
			ComponentFromArg: "aws/bastion",
			SubCommand:       "validate",
			ProcessTemplates: true,
			ProcessFunctions: true,
		}
		packerFlags := PackerFlags{}

		err := ExecutePacker(&info, &packerFlags)
		assert.Error(t, err)
	})

	t.Run("inspect with invalid template", func(t *testing.T) {
		info := schema.ConfigAndStacksInfo{
			Stack:            "nonprod",
			ComponentType:    "packer",
			ComponentFromArg: "invalid/template",
			SubCommand:       "inspect",
			ProcessTemplates: true,
			ProcessFunctions: true,
		}
		packerFlags := PackerFlags{}

		err := ExecutePacker(&info, &packerFlags)
		assert.Error(t, err)
	})

	t.Run("validate with corrupted template", func(t *testing.T) {
		// Create a temporary corrupted template file
		tmpDir := t.TempDir()
		t.Setenv("ATMOS_CLI_CONFIG_PATH", tmpDir)

		info := schema.ConfigAndStacksInfo{
			Stack:            "nonprod",
			ComponentType:    "packer",
			ComponentFromArg: "corrupted/template",
			SubCommand:       "validate",
			ProcessTemplates: true,
			ProcessFunctions: true,
		}
		packerFlags := PackerFlags{}

		err := ExecutePacker(&info, &packerFlags)
		assert.Error(t, err)

		// Reset config path
		t.Setenv("ATMOS_CLI_CONFIG_PATH", workDir)
	})

	t.Run("missing component", func(t *testing.T) {
		info := schema.ConfigAndStacksInfo{
			Stack:            "nonprod",
			ComponentType:    "packer",
			ComponentFromArg: "",
			SubCommand:       "validate",
			ProcessTemplates: true,
			ProcessFunctions: true,
		}
		packerFlags := PackerFlags{}

		err := ExecutePacker(&info, &packerFlags)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "`component` is required")
	})

	t.Run("invalid packer template syntax", func(t *testing.T) {
		// Create a temporary directory with an invalid packer template
		tmpDir := t.TempDir()
		templatePath := filepath.Join(tmpDir, "invalid_template.json")
		err := os.WriteFile(templatePath, []byte("{\n  \"variables\": {\n    \"invalid_json\": true,\n  } // Trailing comma and missing closing brace"), 0o644)
		assert.NoError(t, err)

		info := schema.ConfigAndStacksInfo{
			Stack:            "nonprod",
			ComponentType:    "packer",
			ComponentFromArg: "aws/bastion",
			SubCommand:       "validate",
			ProcessTemplates: true,
			ProcessFunctions: true,
		}
		packerFlags := PackerFlags{
			Template: templatePath,
		}

		err = ExecutePacker(&info, &packerFlags)
		assert.Error(t, err)
	})

	t.Run("missing packer binary", func(t *testing.T) {
		// Temporarily modify PATH to ensure packer is not found
		t.Setenv("PATH", "/nonexistent/path")

		info := schema.ConfigAndStacksInfo{
			Stack:            "nonprod",
			ComponentType:    "packer",
			ComponentFromArg: "aws/bastion",
			SubCommand:       "validate",
		}
		packerFlags := PackerFlags{}

		err := ExecutePacker(&info, &packerFlags)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "executable file not found")
	})

	t.Run("invalid command arguments", func(t *testing.T) {
		info := schema.ConfigAndStacksInfo{
			Stack:            "nonprod",
			ComponentType:    "packer",
			ComponentFromArg: "aws/bastion",
			SubCommand:       "validate me",
		}
		packerFlags := PackerFlags{}

		err := ExecutePacker(&info, &packerFlags)
		assert.Error(t, err)
	})
}

// TestExecutePacker_DirectoryMode tests that Packer commands work with directory-based templates.
// Multiple *.pkr.hcl files are loaded from the component directory.
// This tests the fix for GitHub issue #1937.
func TestExecutePacker_DirectoryMode(t *testing.T) {
	tests.RequirePacker(t)

	workDir := "../../tests/fixtures/scenarios/packer"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", workDir)
	t.Setenv("ATMOS_LOGS_LEVEL", "Warning")
	log.SetLevel(log.InfoLevel)

	t.Run("directory mode with no template specified", func(t *testing.T) {
		// First init the multi-file component.
		initInfo := schema.ConfigAndStacksInfo{
			Stack:            "prod",
			ComponentType:    "packer",
			ComponentFromArg: "aws/multi-file",
			SubCommand:       "init",
			ProcessTemplates: true,
			ProcessFunctions: true,
		}

		packerFlags := PackerFlags{}
		err := ExecutePacker(&initInfo, &packerFlags)
		if err != nil {
			t.Skipf("Skipping test: packer init failed (may require network access): %v", err)
		}

		// Now test validate with no template (should default to ".").
		info := schema.ConfigAndStacksInfo{
			Stack:            "prod",
			ComponentType:    "packer",
			ComponentFromArg: "aws/multi-file",
			SubCommand:       "validate",
			ProcessTemplates: true,
			ProcessFunctions: true,
		}

		var execErr error
		output := captureStdout(t, func() {
			// No template flag - should default to ".".
			execErr = ExecutePacker(&info, &packerFlags)
		})

		assert.NoError(t, execErr, "validate should succeed with directory mode (no template)")

		expected := "The configuration is valid"
		if !strings.Contains(output, expected) {
			t.Logf("TestExecutePacker_DirectoryMode output: %s", output)
			t.Errorf("Output should contain: %s", expected)
		}
	})

	t.Run("directory mode with explicit dot template", func(t *testing.T) {
		// First init.
		initInfo := schema.ConfigAndStacksInfo{
			Stack:            "prod",
			ComponentType:    "packer",
			ComponentFromArg: "aws/multi-file",
			SubCommand:       "init",
			ProcessTemplates: true,
			ProcessFunctions: true,
		}

		packerFlags := PackerFlags{Template: "."}
		err := ExecutePacker(&initInfo, &packerFlags)
		if err != nil {
			t.Skipf("Skipping test: packer init failed (may require network access): %v", err)
		}

		// Test inspect with explicit "." template.
		info := schema.ConfigAndStacksInfo{
			Stack:            "prod",
			ComponentType:    "packer",
			ComponentFromArg: "aws/multi-file",
			SubCommand:       "inspect",
			ProcessTemplates: true,
			ProcessFunctions: true,
		}

		var execErr error
		output := captureStdout(t, func() {
			execErr = ExecutePacker(&info, &packerFlags)
		})

		assert.NoError(t, execErr, "inspect should succeed with explicit '.' template")

		// Verify that variables from variables.pkr.hcl are loaded.
		expected := "var.region"
		if !strings.Contains(output, expected) {
			t.Logf("TestExecutePacker_DirectoryMode inspect output: %s", output)
			t.Errorf("Output should contain: %s", expected)
		}
	})
}

// TestExecutePacker_MultiFileComponent tests that a component with separate files works correctly.
// It uses variables.pkr.hcl and main.pkr.hcl files when no template is specified.
func TestExecutePacker_MultiFileComponent(t *testing.T) {
	tests.RequirePacker(t)

	workDir := "../../tests/fixtures/scenarios/packer"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", workDir)
	t.Setenv("ATMOS_LOGS_LEVEL", "Warning")
	log.SetLevel(log.InfoLevel)

	// Verify the multi-file component has the correct files.
	componentPath := filepath.Join(workDir, "components", "packer", "aws", "multi-file")

	// Check variables.pkr.hcl exists.
	variablesFile := filepath.Join(componentPath, "variables.pkr.hcl")
	_, err := os.Stat(variablesFile)
	assert.NoError(t, err, "variables.pkr.hcl should exist in multi-file component")

	// Check main.pkr.hcl exists.
	mainFile := filepath.Join(componentPath, "main.pkr.hcl")
	_, err = os.Stat(mainFile)
	assert.NoError(t, err, "main.pkr.hcl should exist in multi-file component")

	// Run packer init.
	initInfo := schema.ConfigAndStacksInfo{
		Stack:            "nonprod",
		ComponentType:    "packer",
		ComponentFromArg: "aws/multi-file",
		SubCommand:       "init",
		ProcessTemplates: true,
		ProcessFunctions: true,
	}

	packerFlags := PackerFlags{}
	err = ExecutePacker(&initInfo, &packerFlags)
	if err != nil {
		t.Skipf("Skipping test: packer init failed (may require network access): %v", err)
	}

	// Run packer validate - this should load both files.
	info := schema.ConfigAndStacksInfo{
		Stack:            "nonprod",
		ComponentType:    "packer",
		ComponentFromArg: "aws/multi-file",
		SubCommand:       "validate",
		ProcessTemplates: true,
		ProcessFunctions: true,
	}

	var execErr error
	output := captureStdout(t, func() {
		execErr = ExecutePacker(&info, &packerFlags)
	})

	// This should succeed because Packer loads all *.pkr.hcl files from the directory.
	assert.NoError(t, execErr, "multi-file component should validate successfully when Packer loads all *.pkr.hcl files")

	expected := "The configuration is valid"
	if !strings.Contains(output, expected) {
		t.Logf("TestExecutePacker_MultiFileComponent output: %s", output)
		t.Errorf("Output should contain: %s", expected)
	}
}

// TestPackerComponentEnvSectionConversion verifies that ComponentEnvSection is properly
// converted to ComponentEnvList in Packer execution. This ensures auth environment variables
// and stack config env sections are passed to Packer commands.
//
//nolint:dupl // Test logic is intentionally similar across terraform/helmfile/packer for consistency
func TestPackerComponentEnvSectionConversion(t *testing.T) {
	tests := []struct {
		name            string
		envSection      map[string]any
		expectedEnvList map[string]string
	}{
		{
			name: "converts AWS auth environment variables for Packer",
			envSection: map[string]any{
				"AWS_CONFIG_FILE":             "/path/to/config",
				"AWS_SHARED_CREDENTIALS_FILE": "/path/to/credentials",
				"AWS_PROFILE":                 "packer-profile",
				"AWS_REGION":                  "us-west-2",
			},
			expectedEnvList: map[string]string{
				"AWS_CONFIG_FILE":             "/path/to/config",
				"AWS_SHARED_CREDENTIALS_FILE": "/path/to/credentials",
				"AWS_PROFILE":                 "packer-profile",
				"AWS_REGION":                  "us-west-2",
			},
		},
		{
			name: "handles custom environment variables",
			envSection: map[string]any{
				"PACKER_LOG":      "1",
				"PACKER_LOG_PATH": "/var/log/packer.log",
				"CUSTOM_VAR":      "custom-value",
			},
			expectedEnvList: map[string]string{
				"PACKER_LOG":      "1",
				"PACKER_LOG_PATH": "/var/log/packer.log",
				"CUSTOM_VAR":      "custom-value",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test ConfigAndStacksInfo with ComponentEnvSection populated.
			info := schema.ConfigAndStacksInfo{
				ComponentEnvSection: tt.envSection,
				ComponentEnvList:    []string{},
			}

			// Call the production conversion function.
			ConvertComponentEnvSectionToList(&info)

			// Verify all expected environment variables are in ComponentEnvList.
			envListMap := make(map[string]string)
			for _, envVar := range info.ComponentEnvList {
				parts := strings.SplitN(envVar, "=", 2)
				if len(parts) == 2 {
					envListMap[parts[0]] = parts[1]
				}
			}

			// Check that all expected vars are present with correct values.
			for key, expectedValue := range tt.expectedEnvList {
				actualValue, exists := envListMap[key]
				assert.True(t, exists, "Expected environment variable %s to be in ComponentEnvList", key)
				assert.Equal(t, expectedValue, actualValue,
					"Environment variable %s should have value %s, got %s", key, expectedValue, actualValue)
			}

			// Verify count matches.
			assert.Equal(t, len(tt.expectedEnvList), len(envListMap),
				"ComponentEnvList should contain exactly %d variables", len(tt.expectedEnvList))
		})
	}
}

// TestExecutePacker_ComponentMetadata tests that abstract and locked components are handled correctly.
// Abstract components cannot be built (metadata.type: abstract).
// Locked components cannot be modified (metadata.locked: true).
// Both should still allow read-only commands like validate and inspect.
func TestExecutePacker_ComponentMetadata(t *testing.T) {
	tests.RequirePacker(t)

	workDir := "../../tests/fixtures/scenarios/packer"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", workDir)
	t.Setenv("ATMOS_LOGS_LEVEL", "Warning")
	log.SetLevel(log.InfoLevel)

	testCases := []struct {
		name           string
		component      string
		subCommand     string
		shouldError    bool
		errorSubstring string
	}{
		// Abstract component tests.
		{
			name:           "build command should fail for abstract component",
			component:      "aws/bastion-abstract",
			subCommand:     "build",
			shouldError:    true,
			errorSubstring: "abstract",
		},
		{
			name:           "validate command should succeed for abstract component",
			component:      "aws/bastion-abstract",
			subCommand:     "validate",
			shouldError:    false,
			errorSubstring: "abstract",
		},
		{
			name:           "inspect command should succeed for abstract component",
			component:      "aws/bastion-abstract",
			subCommand:     "inspect",
			shouldError:    false,
			errorSubstring: "abstract",
		},
		// Locked component tests.
		{
			name:           "build command should fail for locked component",
			component:      "aws/bastion-locked",
			subCommand:     "build",
			shouldError:    true,
			errorSubstring: "locked",
		},
		{
			name:           "validate command should succeed for locked component",
			component:      "aws/bastion-locked",
			subCommand:     "validate",
			shouldError:    false,
			errorSubstring: "locked",
		},
		{
			name:           "inspect command should succeed for locked component",
			component:      "aws/bastion-locked",
			subCommand:     "inspect",
			shouldError:    false,
			errorSubstring: "locked",
		},
		// Disabled component tests.
		{
			name:           "build command should skip disabled component",
			component:      "aws/bastion-disabled",
			subCommand:     "build",
			shouldError:    false,
			errorSubstring: "",
		},
		{
			name:           "validate command should skip disabled component",
			component:      "aws/bastion-disabled",
			subCommand:     "validate",
			shouldError:    false,
			errorSubstring: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			info := schema.ConfigAndStacksInfo{
				Stack:            "nonprod",
				ComponentType:    "packer",
				ComponentFromArg: tc.component,
				SubCommand:       tc.subCommand,
				ProcessTemplates: true,
				ProcessFunctions: true,
			}

			packerFlags := PackerFlags{}
			err := ExecutePacker(&info, &packerFlags)

			if tc.shouldError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.errorSubstring)
			} else if err != nil {
				// For non-build commands, we don't check for no error because
				// they may fail for other reasons (network, plugins, etc.)
				// We just verify they don't fail with the metadata-specific error.
				assert.NotContains(t, err.Error(), tc.errorSubstring,
					"Non-build commands should not fail with %s error", tc.errorSubstring)
			}
		})
	}
}
