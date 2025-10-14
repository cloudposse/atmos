package cmd

import (
	"bytes"
	"os"
	"strings"
	"testing"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/stretchr/testify/assert"
)

func TestPackerOutputCmd(t *testing.T) {
	skipIfPackerNotInstalled(t)

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

	RootCmd.SetArgs([]string{"packer", "output", "aws/bastion", "-s", "nonprod", "-q", ".builds[0].artifact_id | split(\":\")[1]"})
	err := Execute()
	assert.NoError(t, err, "'TestPackerOutputCmd' should execute without error")

	// Close write end after Execute
	err = w.Close()
	assert.NoError(t, err)

	// Read the captured output
	var buf bytes.Buffer
	_, err = buf.ReadFrom(r)
	assert.NoError(t, err)
	output := buf.String()

	// Check the output
	expected := "ami-0c2ca16b7fcac7529"

	if !strings.Contains(output, expected) {
		t.Logf("TestPackerOutputCmd output: %s", output)
		t.Errorf("Output should contain: %s", expected)
	}
}
