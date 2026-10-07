//nolint:lintroller // Thin OS adapter methods; detection measures the complete operation.
package installer

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"time"
)

// System supplies read-only operating system operations for installation detection.
// Output implementations must honor context cancellation.
type System interface {
	GOOS() string
	Getenv(string) string
	Executable() (string, error)
	EvalSymlinks(string) (string, error)
	Abs(string) (string, error)
	UserHomeDir() (string, error)
	LookPath(string) (string, error)
	Output(context.Context, string, ...string) ([]byte, error)
	BuildInfo() (*debug.BuildInfo, bool)
}

const probeWaitDelay = 50 * time.Millisecond

type osSystem struct{}

func (osSystem) GOOS() string { return runtime.GOOS }

//nolint:forbidigo // Third-party manager environment is OS evidence, not Atmos configuration.
func (osSystem) Getenv(key string) string              { return os.Getenv(key) }
func (osSystem) Executable() (string, error)           { return os.Executable() }
func (osSystem) EvalSymlinks(p string) (string, error) { return filepath.EvalSymlinks(p) }
func (osSystem) Abs(p string) (string, error)          { return filepath.Abs(p) }
func (osSystem) LookPath(name string) (string, error)  { return exec.LookPath(name) }
func (osSystem) BuildInfo() (*debug.BuildInfo, bool)   { return debug.ReadBuildInfo() }

//nolint:forbidigo // Detection needs the actual user's installation roots, not Atmos configuration.
func (osSystem) UserHomeDir() (string, error) { return os.UserHomeDir() }

func (osSystem) Output(ctx context.Context, executable string, args ...string) ([]byte, error) {
	// Executable comes from LookPath; arguments are fixed, read-only probes.
	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.Env = append(os.Environ(), "LC_ALL=C", "LANG=C")
	cmd.WaitDelay = probeWaitDelay
	return cmd.Output()
}
