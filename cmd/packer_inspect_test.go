package cmd

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"

	log "github.com/charmbracelet/log"
	"github.com/stretchr/testify/assert"
)

func TestPackerInspectCmd(t *testing.T) {
	// Skip test if packer binary is not available
	if _, err := exec.LookPath("packer"); err != nil {
		t.Skipf("Skipping test because packer binary is not found in PATH: %v", err)
	}

	workDir := "../tests/fixtures/scenarios/packer"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", workDir)
	t.Setenv("ATMOS_BASE_PATH", workDir)
	t.Setenv("ATMOS_LOGS_LEVEL", "Warning")
	log.SetLevel(log.WarnLevel)

	oldStd := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	log.SetOutput(w)

	// Ensure cleanup happens before any reads
	defer func() {
		os.Stdout = oldStd
		log.SetOutput(os.Stderr)
	}()

	RootCmd.SetArgs([]string{"packer", "inspect", "aws/bastion", "-s", "nonprod"})
	err := Execute()
	assert.NoError(t, err, "'TestPackerInspectCmd' should execute without error")

	// Close write end after Execute
	err = w.Close()
	assert.NoError(t, err)

	// Read the captured output
	var buf bytes.Buffer
	_, err = buf.ReadFrom(r)
	assert.NoError(t, err)
	output := buf.String()

	// Check the output
	expected := "var.source_ami: \"ami-0013ceeff668b979b\""

	if !strings.Contains(output, expected) {
		t.Logf("TestPackerInspectCmd output: %s", output)
		t.Errorf("Output should contain: %s", expected)
	}
}
