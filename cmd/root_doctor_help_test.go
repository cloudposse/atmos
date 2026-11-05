package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	doctorcmd "github.com/cloudposse/atmos/cmd/doctor"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/telemetry"
)

func TestDoctorIsAnnotatedConfigFree(t *testing.T) {
	_ = NewTestKit(t)

	doctor, _, err := RootCmd.Find([]string{"doctor"})
	require.NoError(t, err)
	require.Equal(t, "doctor", doctor.Name())
	assert.True(t, doctorcmd.IsConfigFree(doctor))

	version, _, err := RootCmd.Find([]string{"version"})
	require.NoError(t, err)
	assert.False(t, doctorcmd.IsConfigFree(version))
	assert.False(t, doctorcmd.IsConfigFree(RootCmd))
}

// TestDoctorHelpDoesNotLoadConfiguration guards "atmos doctor --help" on a malformed atmos.yaml: the
// telemetry disclosure and update check that run with help read the configuration, which must not
// surface a "Could not load config" warning for a command that never loads it.
func TestDoctorHelpDoesNotLoadConfiguration(t *testing.T) {
	_ = NewTestKit(t)
	// The telemetry disclosure is skipped on CI, which would make this test vacuous there.
	defer telemetry.RestoreCIEnvVars(telemetry.PreserveCIEnvVars())

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "atmos.yaml"), []byte("a: [\n"), 0o600))
	t.Chdir(dir)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	var logs bytes.Buffer
	level := log.GetLevel()
	log.SetOutput(&logs)
	t.Cleanup(func() {
		log.SetOutput(os.Stderr)
		log.SetLevel(level)
	})
	log.SetLevel(log.WarnLevel)

	originalArgs := os.Args
	t.Cleanup(func() { os.Args = originalArgs })
	os.Args = []string{"atmos", "doctor", "--help"}

	doctor, _, err := RootCmd.Find([]string{"doctor"})
	require.NoError(t, err)
	rootHelpFunc(doctor, []string{"--help"})

	assert.NotContains(t, logs.String(), "Could not load config")
}
