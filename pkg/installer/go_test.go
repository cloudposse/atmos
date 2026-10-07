package installer

import (
	"runtime/debug"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGoUpgradeTargetsDetectedBinary(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, goos, executable, target, condition, command string
		env                                                map[string]string
	}{
		{
			name: "later GOPATH entry", goos: "linux", executable: "/custom/bin/atmos",
			env:     map[string]string{"GOPATH": "/other:/custom"},
			command: "GOBIN=/custom/bin go install github.com/cloudposse/atmos@v1.2.4",
		},
		{
			name: "custom GOBIN with spaces", goos: "darwin", executable: "/custom tools/atmos",
			env:     map[string]string{"GOBIN": "/custom tools"},
			command: "GOBIN='/custom tools' go install github.com/cloudposse/atmos@v1.2.4",
		},
		{
			name: "POSIX metacharacters stay literal", goos: "linux", executable: "/tools/it's $(literal)`value`/atmos",
			env:     map[string]string{"GOBIN": "/tools/it's $(literal)`value`"},
			command: "GOBIN='/tools/it'\"'\"'s $(literal)`value`' go install github.com/cloudposse/atmos@v1.2.4",
		},
		{
			name: "symlinked GOBIN", goos: "linux", executable: "/linked/bin/atmos", target: "/actual/bin/atmos",
			env:     map[string]string{"GOBIN": "/linked/bin"},
			command: "GOBIN=/actual/bin go install github.com/cloudposse/atmos@v1.2.4",
		},
		{
			name: "Windows later GOPATH entry", goos: "windows", executable: `D:\Tools\bin\atmos.exe`,
			env: map[string]string{"GOPATH": `C:\Go;D:\Tools`}, condition: "In PowerShell",
			command: `$env:GOBIN = 'D:\Tools\bin'; go install github.com/cloudposse/atmos@v1.2.4`,
		},
		{
			name: "PowerShell metacharacters stay literal", goos: "windows", executable: "C:\\My Tools\\it's $literal`value`\\atmos.exe",
			env: map[string]string{"GOBIN": "C:\\My Tools\\it's $literal`value`"}, condition: "In PowerShell",
			command: "$env:GOBIN = 'C:\\My Tools\\it''s $literal`value`'; go install github.com/cloudposse/atmos@v1.2.4",
		},
		{
			name: "Windows drive root", goos: "windows", executable: `D:\atmos.exe`,
			env: map[string]string{"GOBIN": `D:\`}, condition: "In PowerShell",
			command: `$env:GOBIN = 'D:\'; go install github.com/cloudposse/atmos@v1.2.4`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := newSystem(tt.executable)
			s.goos, s.env = tt.goos, tt.env
			s.info = &debug.BuildInfo{Main: debug.Module{Path: "github.com/cloudposse/atmos", Version: "v1.2.3"}}
			if tt.target != "" {
				s.links[tt.executable] = tt.target
				s.links["/linked/bin"] = "/actual/bin"
			}
			installation := Detect(t.Context(), WithSystem(s))
			require.Equal(t, Go, installation.Kind)
			hint := installation.UpgradeHint("v1.2.4")
			assert.Equal(t, tt.command, hint.Command)
			assert.Equal(t, tt.condition, hint.Condition)
		})
	}
}

func TestGoHintWithoutExecutable(t *testing.T) {
	t.Parallel()
	for _, executable := range []string{"", "atmos"} {
		t.Run(executable, func(t *testing.T) {
			t.Parallel()
			hint := (Installation{Kind: Go, Manager: "go", Executable: executable}).UpgradeHint("1.2.4")
			assert.Equal(t, "go install github.com/cloudposse/atmos@v1.2.4", hint.Command)
			assert.Empty(t, hint.Condition)
		})
	}
}

func TestGoBinaryDirectory(t *testing.T) {
	t.Parallel()
	tests := []struct{ executable, goos, want string }{
		{"/atmos", "linux", "/"},
		{`/tools/back\slash/atmos`, "linux", `/tools/back\slash`},
		{`C:/Tools/bin/atmos.exe`, "windows", "C:/Tools/bin"},
		{`C:/atmos.exe`, "windows", "C:/"},
		{`\\server\share\bin\atmos.exe`, "windows", `\\server\share\bin`},
	}
	for _, tt := range tests {
		t.Run(tt.executable, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, goBinaryDirectory(tt.executable, tt.goos))
		})
	}
}
