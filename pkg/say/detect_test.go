package say

import (
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// withMockExecLookPath replaces execLookPath for the duration of the test.
func withMockExecLookPath(t *testing.T, fn func(string) (string, error)) {
	t.Helper()
	orig := execLookPath
	execLookPath = fn
	t.Cleanup(func() { execLookPath = orig })
}

// withMockRuntimeGOOS replaces runtimeGOOS for the duration of the test.
func withMockRuntimeGOOS(t *testing.T, goos string) {
	t.Helper()
	orig := runtimeGOOS
	runtimeGOOS = goos
	t.Cleanup(func() { runtimeGOOS = orig })
}

// lookPathFor returns an execLookPath stub that succeeds only for the given names.
func lookPathFor(found ...string) func(string) (string, error) {
	set := make(map[string]bool, len(found))
	for _, f := range found {
		set[f] = true
	}
	return func(name string) (string, error) {
		if set[name] {
			return "/usr/bin/" + name, nil
		}
		return "", exec.ErrNotFound
	}
}

func TestDetectSay(t *testing.T) {
	tests := []struct {
		name        string
		goos        string
		found       []string
		wantErr     bool
		wantBackend Backend
		wantPath    string
	}{
		{name: "darwin say", goos: "darwin", found: []string{"say"}, wantBackend: BackendMacSay, wantPath: "/usr/bin/say"},
		{name: "darwin missing", goos: "darwin", found: nil, wantErr: true},
		{name: "linux spd-say preferred", goos: "linux", found: []string{"spd-say", "espeak"}, wantBackend: BackendSpdSay, wantPath: "/usr/bin/spd-say"},
		{name: "linux espeak-ng", goos: "linux", found: []string{"espeak-ng"}, wantBackend: BackendEspeak, wantPath: "/usr/bin/espeak-ng"},
		{name: "linux espeak", goos: "linux", found: []string{"espeak"}, wantBackend: BackendEspeak, wantPath: "/usr/bin/espeak"},
		{name: "linux missing", goos: "linux", found: nil, wantErr: true},
		{name: "windows pwsh preferred", goos: "windows", found: []string{"pwsh", "powershell"}, wantBackend: BackendPowerShell, wantPath: "/usr/bin/pwsh"},
		{name: "windows powershell", goos: "windows", found: []string{"powershell"}, wantBackend: BackendPowerShell, wantPath: "/usr/bin/powershell"},
		{name: "windows missing", goos: "windows", found: nil, wantErr: true},
		{name: "unsupported platform", goos: "plan9", found: []string{"say"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withMockRuntimeGOOS(t, tt.goos)
			withMockExecLookPath(t, lookPathFor(tt.found...))

			info, err := DetectSay()
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, errUtils.ErrSayNotFound)
				assert.Nil(t, info)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, info)
			assert.Equal(t, tt.wantBackend, info.Backend)
			assert.Equal(t, tt.wantPath, info.Path)
		})
	}
}
