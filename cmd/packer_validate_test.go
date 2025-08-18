package cmd

import (
	"bytes"
	"os"
	"strings"
	"testing"

	log "github.com/charmbracelet/log"
	"github.com/stretchr/testify/assert"
)

func TestPackerValidateCmd(t *testing.T) {
	workDir := "../tests/fixtures/scenarios/packer"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", workDir)
	t.Setenv("ATMOS_BASE_PATH", workDir)
	t.Setenv("ATMOS_LOGS_LEVEL", "Info")
	log.SetLevel(log.InfoLevel)

	oldStd := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	log.SetOutput(w)

	RootCmd.SetArgs([]string{"packer", "validate", "aws/bastion", "-s", "nonprod"})
	err := Execute()
	assert.NoError(t, err, "'TestPackerValidateCmd' should execute without error")

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
		t.Logf("TestPackerValidateCmd output: %s", output)
		t.Errorf("Output should contain: %s", expected)
	}
}
