package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// toolchainChildEnv gates the re-executed test binary into a fake toolchain executable.
// The child is a "tofu" when its file name starts with toolchainTofuPrefix and an
// ordinary HCL tool otherwise, so one environment can reach both kinds of child.
const (
	toolchainChildEnv = "ATMOS_TERRAGRUNT_TOOLCHAIN_CHILD"
	toolchainCallLog  = "ATMOS_TERRAGRUNT_TOOLCHAIN_CALL_LOG"
	toolchainTofuName = "fixture-tofu"
	toolchainToolName = "fixture-tool"
)

// toolchainReport is what every fake executable observed about its own invocation.
type toolchainReport struct {
	Executable string            `json:"executable"`
	Directory  string            `json:"directory"`
	Arguments  []string          `json:"arguments"`
	Path       string            `json:"path"`
	Profile    string            `json:"profile"`
	Custom     string            `json:"custom"`
	Token      string            `json:"token"`
	Ambient    string            `json:"ambient"`
	Environ    map[string]string `json:"environ,omitempty"`
}

func currentToolchainReport() toolchainReport {
	executable, _ := os.Executable()
	directory, _ := os.Getwd()
	return toolchainReport{
		Executable: executable, Directory: directory, Arguments: os.Args[1:], Path: os.Getenv("PATH"),
		Profile: os.Getenv("AWS_PROFILE"), Custom: os.Getenv("CUSTOM_VALUE"), Token: os.Getenv("AWS_WEB_IDENTITY_TOKEN_FILE"),
		Ambient: os.Getenv("ATMOS_TERRAGRUNT_TOOLCHAIN_AMBIENT"),
	}
}

// runToolchainChild implements the fake executables. It reports whether this process is one.
func runToolchainChild() bool {
	if os.Getenv(toolchainChildEnv) != "1" {
		return false
	}
	report := currentToolchainReport()
	if path := os.Getenv(toolchainCallLog); path != "" {
		if file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600); err == nil {
			_ = json.NewEncoder(file).Encode(report)
			_ = file.Close()
		}
	}
	if !strings.HasPrefix(filepath.Base(os.Args[0]), toolchainTofuName) {
		_ = json.NewEncoder(os.Stdout).Encode(report)
		return true
	}
	if len(os.Args) > 1 && os.Args[1] == "output" {
		encoded, _ := json.Marshal(report)
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"report": map[string]any{"value": string(encoded), "type": "string", "sensitive": false}})
	}
	if len(os.Args) > 1 && os.Args[1] == "version" {
		_, _ = os.Stdout.WriteString(`{"terraform_version":"1.9.0"}`)
	}
	return true
}

// installFixtureExecutable places a hard link (or copy) of the test binary at dir/name.
func installFixtureExecutable(t *testing.T, dir, name string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	require.NoError(t, os.MkdirAll(dir, 0o755))
	target := filepath.Join(dir, name)
	executable, err := os.Executable()
	require.NoError(t, err)
	if err := os.Link(executable, target); err == nil {
		return target
	}
	content, err := os.ReadFile(executable)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(target, content, 0o755))
	return target
}

// physicalPath removes platform temp-directory symlinks so equal directories compare equal.
func physicalPath(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	require.NoError(t, err)
	return filepath.Clean(resolved)
}

// executableSuffix is the file name suffix of an executable on this platform.
func executableSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}
