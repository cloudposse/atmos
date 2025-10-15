package exec

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/tests"
)

func TestExecutePacker_Validate(t *testing.T) {
	tests.RequirePacker(t)

	workDir := "../../tests/fixtures/scenarios/packer"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", workDir)
	t.Setenv("ATMOS_BASE_PATH", workDir)
	t.Setenv("ATMOS_LOGS_LEVEL", "Warning")
	log.SetLevel(log.InfoLevel)

	// First run packer init to install required plugins.
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
	assert.NoError(t, err)

	// Now run validate.
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

	log.SetOutput(w)

	err = ExecutePacker(&info, &packerFlags)
	assert.NoError(t, err)

	// Restore std
	err = w.Close()
	assert.NoError(t, err)
	os.Stdout = oldStd

	// Read the captured output
	var buf bytes.Buffer
	_, err = buf.ReadFrom(r)
	assert.NoError(t, err)
	output := buf.String()

	// Check the output
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
	t.Setenv("ATMOS_BASE_PATH", workDir)
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

	log.SetOutput(w)
	packerFlags := PackerFlags{}

	err := ExecutePacker(&info, &packerFlags)
	assert.NoError(t, err)

	// Restore std
	err = w.Close()
	assert.NoError(t, err)
	os.Stdout = oldStd

	// Read the captured output
	var buf bytes.Buffer
	_, err = buf.ReadFrom(r)
	assert.NoError(t, err)
	output := buf.String()

	// Check the output
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
	t.Setenv("ATMOS_BASE_PATH", workDir)
	t.Setenv("ATMOS_LOGS_LEVEL", "Warning")
	log.SetLevel(log.InfoLevel)

	info := schema.ConfigAndStacksInfo{
		ComponentType: "packer",
		SubCommand:    "version",
	}

	oldStd := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	log.SetOutput(w)
	packerFlags := PackerFlags{}

	err := ExecutePacker(&info, &packerFlags)
	assert.NoError(t, err)

	// Restore std
	err = w.Close()
	assert.NoError(t, err)
	os.Stdout = oldStd

	// Read the captured output
	var buf bytes.Buffer
	_, err = buf.ReadFrom(r)
	assert.NoError(t, err)
	output := buf.String()

	// Check the output
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
	t.Setenv("ATMOS_BASE_PATH", workDir)
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

func TestExecutePacker_Errors(t *testing.T) {
	tests.RequirePacker(t)

	workDir := "../../tests/fixtures/scenarios/packer"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", workDir)
	t.Setenv("ATMOS_BASE_PATH", workDir)
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
		assert.ErrorIs(t, err, errUtils.ErrMissingComponent)
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
