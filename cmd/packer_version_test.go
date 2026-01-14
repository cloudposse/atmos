package cmd

import (
	"bytes"
	"os"
	"strings"
	"testing"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/stretchr/testify/assert"
)

func TestPackerVersionCmd(t *testing.T) {
	_ = NewTestKit(t)

	skipIfPackerNotInstalled(t)

	workDir := "../tests/fixtures/scenarios/packer"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", workDir)
	t.Setenv("ATMOS_LOGS_LEVEL", "Warning")

	// Set logger level to match environment variable and capture original settings
	originalLevel := log.GetLevel()
	defer func() {
		log.SetLevel(originalLevel)
		log.SetOutput(os.Stderr)
	}()
	log.SetLevel(log.WarnLevel) // Match ATMOS_LOGS_LEVEL=Warning

	// Capture stdout and logger output
	oldStd := os.Stdout
	r, w, _ := os.Pipe()
	defer func() {
		os.Stdout = oldStd
	}()
	os.Stdout = w
	log.SetOutput(w)

	RootCmd.SetArgs([]string{"packer", "version"})
	err := Execute()
	assert.NoError(t, err, "'TestPackerVersionCmd' should execute without error")

	// Close write end and restore stdout before reading
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
		t.Logf("TestPackerVersionCmd output: %s", output)
		t.Errorf("Output should contain: %s", expected)
	}
}
