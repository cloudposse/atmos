package driver

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFlociHealthCheck_NativeScript exercises the probe's native-readiness-script
// branch (the minimal GCP/Azure images ship this script but no curl), including a
// native readiness failure that the Bash TCP fallback must not mask. TCP-only
// readiness (no native script) is covered by TestFlociHealthCheck_Readiness in
// health_restart_test.go.
func TestFlociHealthCheck_NativeScript(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the container health check runs in a POSIX shell")
	}

	for _, tc := range []struct {
		name       string
		native     string
		wantOutput string
		wantError  bool
	}{
		{name: "native success", native: "echo native", wantOutput: "native\n"},
		{name: "native failure does not fall back to TCP", native: "echo native; exit 1", wantOutput: "native\n", wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			nativePath := filepath.Join(dir, "healthcheck.sh")
			require.NoError(t, os.WriteFile(nativePath, []byte("#!/bin/sh\n"+tc.native+"\n"), 0o700))
			// Relocate only the image's absolute script path into the fixture.
			probe := strings.ReplaceAll(flociHealthCheck(flociGCPPort).Test[1], "/usr/local/bin/healthcheck.sh", "'"+nativePath+"'")
			cmd := exec.CommandContext(t.Context(), "/bin/sh", "-c", probe)
			output, err := cmd.CombinedOutput()
			if tc.wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err, string(output))
			}
			assert.Equal(t, tc.wantOutput, string(output))
		})
	}
}
