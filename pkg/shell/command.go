package shell

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	xterm "golang.org/x/term"

	cfg "github.com/cloudposse/atmos/pkg/config"
	envpkg "github.com/cloudposse/atmos/pkg/env"
	ioLayer "github.com/cloudposse/atmos/pkg/io"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	terminalpkg "github.com/cloudposse/atmos/pkg/terminal"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// resolveMaskingDisabled decides whether output masking should be disabled.
// The --mask/ATMOS_MASK flag/env override wins when set, otherwise
// settings.terminal.mask.enabled (from atmos.yaml) applies. The presence
// check (IsSet) and value read (GetBool) run together under one
// cfg.GlobalViper().View call so a concurrent LoadConfig Set() between the
// two can never combine one snapshot's presence result with a different
// snapshot's value -- masking is security-sensitive, so this decision must
// never be made from a torn read.
func resolveMaskingDisabled(atmosConfig *schema.AtmosConfiguration) bool {
	disableMasking := false
	cfg.GlobalViper().View(func(v cfg.ViperReader) {
		if v.IsSet("mask") {
			disableMasking = !v.GetBool("mask")
		} else if v.IsSet("settings.terminal.mask.enabled") {
			disableMasking = !atmosConfig.Settings.Terminal.Mask.Enabled
		}
	})
	return disableMasking
}

// CommandRequest describes a subprocess invocation. Environment entries are
// overlaid on the environment supplied by WithEnvironment, or the current process.
type CommandRequest struct {
	AtmosConfig    schema.AtmosConfiguration
	Command        string
	Args           []string
	Dir            string
	Env            []string
	DryRun         bool
	RedirectStderr string
}

// ExecuteCommand prints and executes a subprocess with the supplied options.
func ExecuteCommand(request *CommandRequest, opts ...ShellCommandOption) error {
	defer perf.Track(&request.AtmosConfig, "shell.ExecuteCommand")()

	requestSnapshot := *request
	request = &requestSnapshot

	ioLayer.ApplyMaskingConfig(&ioLayer.Config{
		DisableMasking: resolveMaskingDisabled(&request.AtmosConfig),
		AtmosConfig:    request.AtmosConfig,
	})
	var config CommandConfig
	for _, opt := range opts {
		opt(&config)
	}
	env, err := commandEnvironment(request, &config)
	if err != nil {
		return err
	}
	execution := newCommandExecution(request, &config)
	if err := execution.prepareOutput(); err != nil {
		return err
	}
	defer execution.closeRedirectFile()
	execution.emitStart()
	if request.DryRun {
		return execution.finishDryRun()
	}
	result := execution.run(env)
	return execution.finish(&result)
}

// commandEnvironment merges process, global and command-specific environment entries.
func commandEnvironment(request *CommandRequest, config *CommandConfig) ([]string, error) {
	newShellLevel, err := u.GetNextShellLevel()
	if err != nil {
		return nil, err
	}

	// When auth has sanitized the environment, ProcessEnv is used instead of
	// os.Environ() to avoid reintroducing problematic vars (e.g., IRSA credentials).
	baseEnv := os.Environ()
	if config.ProcessEnv != nil {
		baseEnv = config.ProcessEnv
	}
	cmdEnv := envpkg.MergeGlobalEnv(baseEnv, request.AtmosConfig.Env)
	cmdEnv = append(cmdEnv, request.Env...)
	cmdEnv = append(cmdEnv, fmt.Sprintf("ATMOS_SHLVL=%d", newShellLevel))

	// Propagate TTY state to subprocess. MaskWriter wraps stderr as a pipe, so
	// subprocess TTY detection (e.g., for SSO device auth) otherwise sees a pipe
	// even when the user is interactive. Never override an explicit setting.
	if xterm.IsTerminal(int(os.Stderr.Fd())) && !envKeyIsSet(cmdEnv, "ATMOS_FORCE_TTY") {
		cmdEnv = append(cmdEnv, "ATMOS_FORCE_TTY=true")
	}
	return cmdEnv, nil
}

// flushStreamMaskers releases the tail held by each streaming masker. Maskers are flushed in
// reverse creation order because a later masker (stderr merged into stdout) can feed an earlier one.
func flushStreamMaskers(maskers []*ioLayer.StreamingMaskWriter) {
	for i := len(maskers) - 1; i >= 0; i-- {
		if err := maskers[i].Flush(); err != nil {
			log.Debug("Failed to flush masked output", "error", err)
		}
	}
}

func maybePaceTerminalWriter(w io.Writer, speed float64, closers *[]io.Closer) io.Writer {
	if !terminalpkg.IsSpeedLimited(speed) || !terminalpkg.IsTTYWriter(w) {
		return w
	}

	pacedWriter := terminalpkg.NewPacingWriter(w, speed)
	*closers = append(*closers, pacedWriter)
	return pacedWriter
}

func closePacedWriters(closers []io.Closer) error {
	var closeErr error
	for _, closer := range closers {
		if err := closer.Close(); err != nil && closeErr == nil {
			closeErr = err
		}
	}
	return closeErr
}

type synchronizedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (w *synchronizedWriter) Write(p []byte) (int, error) {
	defer perf.Track(nil, "shell.Write")()

	w.mu.Lock()
	defer w.mu.Unlock()
	return w.w.Write(p)
}

// envKeyIsSet returns true if any entry in env starts with "KEY=".
func envKeyIsSet(env []string, key string) bool {
	prefix := key + "="
	for _, e := range env {
		if strings.HasPrefix(e, prefix) {
			return true
		}
	}
	return false
}

// EnvVarFromList returns the value of the last "KEY=value" entry in env, or ""
// if the key is not present.  The last entry wins, matching how exec.Cmd.Env
// and os.Environ() resolve duplicates.
func EnvVarFromList(env []string, key string) string {
	defer perf.Track(nil, "shell.EnvVarFromList")()

	prefix := key + "="
	result := ""
	for _, e := range env {
		if strings.HasPrefix(e, prefix) {
			result = e[len(prefix):]
		}
	}
	return result
}
