package shell

import (
	"io"
	"os"
	"path/filepath"
	"time"

	xterm "golang.org/x/term"

	"github.com/cloudposse/atmos/pkg/diagnostics"
	log "github.com/cloudposse/atmos/pkg/logger"
	process "github.com/cloudposse/atmos/pkg/process"
	terminalpkg "github.com/cloudposse/atmos/pkg/terminal"
)

func emitProcessEndDiagnostics(config diagnostics.Config, id string, fallbackStart time.Time, result *process.Result) {
	if result == nil {
		return
	}
	finishedAt := result.FinishedAt
	if finishedAt.IsZero() {
		finishedAt = time.Now()
	}

	startedAt := result.StartedAt
	if startedAt.IsZero() {
		startedAt = fallbackStart
	}

	event := diagnostics.Event{
		Type:       "process.end",
		Time:       finishedAt.UTC(),
		ID:         id,
		Started:    diagnostics.Bool(result.Started),
		Success:    diagnostics.Bool(result.Err == nil),
		Canceled:   diagnostics.Bool(result.Canceled),
		ExitCode:   diagnostics.Int(result.ExitCode),
		DurationMS: diagnostics.Int64(finishedAt.Sub(startedAt).Milliseconds()),
		Signaled:   diagnostics.Bool(result.Signaled),
		Signal:     result.Signal,
		Error:      errorString(result.Err),
	}
	if result.Signaled {
		event.SignalNumber = diagnostics.Int(result.SignalNumber)
	}
	emitDiagnosticsEvent(config, &event)
}

func emitDiagnosticsEvent(config diagnostics.Config, event *diagnostics.Event) {
	if err := diagnostics.EmitWithConfig(config, event); err != nil {
		log.Debug("Failed to write diagnostics event", "error", err)
	}
}

func resolveDiagnosticsCWD(dir string) string {
	if dir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return ""
		}
		return cwd
	}
	if filepath.IsAbs(dir) {
		return filepath.Clean(dir)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return filepath.Clean(dir)
	}
	return filepath.Clean(filepath.Join(cwd, dir))
}

func streamsHaveTTY(streams process.Streams) bool {
	return isTTYReader(streams.Stdin) || terminalpkg.IsTTYWriter(streams.Stdout) || terminalpkg.IsTTYWriter(streams.Stderr)
}

func isTTYReader(r io.Reader) bool {
	file, ok := r.(*os.File)
	if !ok || file == nil {
		return false
	}
	return xterm.IsTerminal(int(file.Fd()))
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
