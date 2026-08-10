package say

import (
	"fmt"
	"os/exec"
	"runtime"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Backend identifies a platform text-to-speech engine.
type Backend int

const (
	// BackendMacSay is the macOS `say` command.
	BackendMacSay Backend = iota
	// BackendSpdSay is the Linux Speech Dispatcher `spd-say` command.
	BackendSpdSay
	// BackendEspeak is the Linux `espeak`/`espeak-ng` command.
	BackendEspeak
	// BackendPowerShell is the Windows PowerShell System.Speech backend.
	BackendPowerShell
)

// Package-level function variables for testability.
var (
	execLookPath = exec.LookPath
	runtimeGOOS  = runtime.GOOS
)

// SayInfo holds information about a detected text-to-speech backend.
type SayInfo struct {
	// Path is the resolved path to the TTS executable.
	Path string
	// Backend identifies which TTS engine Path refers to.
	Backend Backend
}

// DetectSay finds a text-to-speech backend on the system.
// Returns ErrSayNotFound if no backend is available.
func DetectSay() (*SayInfo, error) {
	defer perf.Track(nil, "say.DetectSay")()

	switch runtimeGOOS {
	case "darwin":
		return detectSayDarwin()
	case "linux":
		return detectSayLinux()
	case "windows":
		return detectSayWindows()
	default:
		return nil, fmt.Errorf("%w: unsupported platform %s", errUtils.ErrSayNotFound, runtimeGOOS)
	}
}

// detectSayDarwin finds the macOS `say` command via PATH.
func detectSayDarwin() (*SayInfo, error) {
	path, err := execLookPath("say")
	if err != nil {
		return nil, fmt.Errorf("%w: `say` not found in PATH", errUtils.ErrSayNotFound)
	}
	log.Debug("Found text-to-speech backend", "backend", "say", "path", path)
	return &SayInfo{Path: path, Backend: BackendMacSay}, nil
}

// detectSayLinux finds a Linux TTS backend, preferring spd-say, then espeak-ng, then espeak.
func detectSayLinux() (*SayInfo, error) {
	candidates := []struct {
		name    string
		backend Backend
	}{
		{"spd-say", BackendSpdSay},
		{"espeak-ng", BackendEspeak},
		{"espeak", BackendEspeak},
	}

	for _, c := range candidates {
		path, err := execLookPath(c.name)
		if err == nil {
			log.Debug("Found text-to-speech backend", "backend", c.name, "path", path)
			return &SayInfo{Path: path, Backend: c.backend}, nil
		}
	}

	return nil, fmt.Errorf("%w: no spd-say/espeak/espeak-ng found in PATH", errUtils.ErrSayNotFound)
}

// detectSayWindows finds PowerShell for the System.Speech backend, preferring pwsh over powershell.
func detectSayWindows() (*SayInfo, error) {
	for _, name := range []string{"pwsh", "powershell"} {
		path, err := execLookPath(name)
		if err == nil {
			log.Debug("Found text-to-speech backend", "backend", name, "path", path)
			return &SayInfo{Path: path, Backend: BackendPowerShell}, nil
		}
	}

	return nil, fmt.Errorf("%w: no pwsh/powershell found in PATH", errUtils.ErrSayNotFound)
}
