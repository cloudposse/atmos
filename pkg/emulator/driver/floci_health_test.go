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

// Exercise the probe's shell behavior with image executables represented by
// scripts, including a native readiness failure that curl must not mask.
func TestFlociHealthCheck_Readiness(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the container health check runs in a POSIX shell")
	}

	for _, tc := range []struct {
		name       string
		native     string
		curl       string
		wantOutput string
		wantError  bool
	}{
		{name: "native without curl", native: "echo native", wantOutput: "native\n"},
		{name: "native failure does not fall back", native: "echo native; exit 1", curl: "echo curl", wantOutput: "native\n", wantError: true},
		{name: "legacy image", curl: "echo curl", wantOutput: "curl\n"},
		{name: "legacy connection failure", curl: "echo curl; exit 7", wantOutput: "curl\n", wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			nativePath := filepath.Join(dir, "healthcheck.sh")
			for name, body := range map[string]string{"healthcheck.sh": tc.native, "curl": tc.curl} {
				if body != "" {
					require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"+body+"\n"), 0o700))
				}
			}
			// Relocate only the image's absolute script path into the fixture.
			probe := strings.ReplaceAll(flociHealthCheck(flociGCPPort).Test[1], "/usr/local/bin/healthcheck.sh", "'"+nativePath+"'")
			cmd := exec.CommandContext(t.Context(), "/bin/sh", "-c", probe)
			cmd.Env = []string{"PATH=" + dir}
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
