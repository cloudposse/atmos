package cmd

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPackerValidateCmd(t *testing.T) {
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

	// Run "packer init" first rather than relying on TestPackerInitCmd having
	// already run and left the "amazon" plugin installed in the shared
	// packer plugin cache: under -shuffle=on, this test can run before that
	// one, and "packer validate" doesn't install missing plugins itself
	// (confirmed in CI: "Missing plugins ... Did you run packer init for
	// this project?"). Making this test install its own prerequisite makes
	// it self-sufficient regardless of run order.
	RootCmd.SetArgs([]string{"packer", "init", "aws/bastion", "-s", "nonprod"})
	require.NoError(t, Execute(), "packer init prerequisite should execute without error")
	// A nil error from Execute() is not proof the prerequisite ran: leaked
	// global state (a stray --version/--help flag, a viper key, dry-run) can
	// make cobra or atmos return nil without ever spawning packer, and the
	// only symptom is validate's "Missing plugins" much further down. Ask
	// packer itself whether the plugin is now present so a silent no-op fails
	// here, on the line that names the real problem.
	requirePackerPluginInstalled(t, "github.com/hashicorp/amazon")

	// Capture stdout and logger output
	oldStd := os.Stdout
	r, w, _ := os.Pipe()
	defer func() {
		os.Stdout = oldStd
	}()
	os.Stdout = w
	log.SetOutput(w)

	RootCmd.SetArgs([]string{"packer", "validate", "aws/bastion", "-s", "nonprod"})
	err := Execute()
	assert.NoError(t, err, "'TestPackerValidateCmd' should execute without error")

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
	expected := "The configuration is valid"

	if !strings.Contains(output, expected) {
		t.Logf("TestPackerValidateCmd output: %s", output)
		t.Errorf("Output should contain: %s", expected)
	}
}

// requirePackerPluginInstalled fails the test unless `packer plugins installed`
// lists the given plugin source, so a prerequisite `atmos packer init` that
// silently did nothing is caught at the prerequisite rather than surfacing as a
// confusing "Missing plugins" from a later validate/build. The command reports
// an OS-native filesystem path (backslash-separated on Windows), while source
// is a forward-slash plugin id, so normalize before comparing.
func requirePackerPluginInstalled(t *testing.T, source string) {
	t.Helper()
	out, err := exec.Command("packer", "plugins", "installed").CombinedOutput()
	require.NoError(t, err, "packer plugins installed: %s", out)
	require.Contains(t, filepath.ToSlash(string(out)), strings.TrimPrefix(source, "github.com/"),
		"prerequisite `atmos packer init` returned nil but did not install %s -- "+
			"Execute() was short-circuited by leaked global state; output:\n%s", source, out)
}
