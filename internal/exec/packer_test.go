package exec

import (
	"bytes"
	"os"
	"strings"
	"testing"

	log "github.com/charmbracelet/log"
	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestExecutePacker_Validate(t *testing.T) {
	workDir := "../../tests/fixtures/scenarios/packer"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", workDir)
	t.Setenv("ATMOS_BASE_PATH", workDir)
	t.Setenv("ATMOS_LOGS_LEVEL", "Info")
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
	expected := "The configuration is valid"

	if !strings.Contains(output, expected) {
		t.Logf("TestExecutePacker_Validate output: %s", output)
		t.Errorf("Output should contain: %s", expected)
	}
}

func TestExecutePacker_Inspect(t *testing.T) {
	workDir := "../../tests/fixtures/scenarios/packer"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", workDir)
	t.Setenv("ATMOS_BASE_PATH", workDir)
	t.Setenv("ATMOS_LOGS_LEVEL", "Info")
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
	workDir := "../../tests/fixtures/scenarios/packer"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", workDir)
	t.Setenv("ATMOS_BASE_PATH", workDir)
	t.Setenv("ATMOS_LOGS_LEVEL", "Info")
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
	workDir := "../../tests/fixtures/scenarios/packer"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", workDir)
	t.Setenv("ATMOS_BASE_PATH", workDir)
	t.Setenv("ATMOS_LOGS_LEVEL", "Info")
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
