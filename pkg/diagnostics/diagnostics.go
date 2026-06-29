package diagnostics

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	// SinkFile is the first supported diagnostics sink.
	SinkFile = "file"

	// LevelDebug is the default diagnostics event level for process lifecycle events.
	LevelDebug = "debug"

	levelOff     = "off"
	dirMode      = 0o755
	eventLogMode = 0o600
)

var (
	configMu      sync.RWMutex
	defaultConfig Config
	fileMu        sync.Mutex
	idCounter     uint64
)

// Config describes the active diagnostics sink configuration.
type Config struct {
	File  string
	Level string
	Sink  string
	URL   string
}

// Event is a machine-readable diagnostics event. Events are intentionally flat
// JSON objects so JSONL artifacts are easy to inspect with jq and grep.
type Event struct {
	Type           string    `json:"type"`
	Time           time.Time `json:"time"`
	Level          string    `json:"level,omitempty"`
	ID             string    `json:"id,omitempty"`
	Command        string    `json:"command,omitempty"`
	Args           []string  `json:"args,omitempty"`
	CWD            string    `json:"cwd,omitempty"`
	DryRun         *bool     `json:"dry_run,omitempty"`
	TTY            *bool     `json:"tty,omitempty"`
	StdinTTY       *bool     `json:"stdin_tty,omitempty"`
	StdoutTTY      *bool     `json:"stdout_tty,omitempty"`
	StderrTTY      *bool     `json:"stderr_tty,omitempty"`
	RedirectStderr string    `json:"redirect_stderr,omitempty"`
	Started        *bool     `json:"started,omitempty"`
	Success        *bool     `json:"success,omitempty"`
	Canceled       *bool     `json:"canceled,omitempty"`
	ExitCode       *int      `json:"exit_code,omitempty"`
	DurationMS     *int64    `json:"duration_ms,omitempty"`
	Signaled       *bool     `json:"signaled,omitempty"`
	Signal         string    `json:"signal,omitempty"`
	SignalNumber   *int      `json:"signal_number,omitempty"`
	Error          string    `json:"error,omitempty"`
}

// FromSchema converts Atmos configuration into a diagnostics package config.
func FromSchema(config schema.Diagnostics) Config {
	defer perf.Track(nil, "diagnostics.FromSchema")()

	return Config{
		File:  config.File,
		Level: config.Level,
		Sink:  config.Sink,
		URL:   config.URL,
	}
}

// Configure sets the package-level diagnostics configuration.
func Configure(config schema.Diagnostics) {
	defer perf.Track(nil, "diagnostics.Configure")()

	SetConfig(FromSchema(config))
}

// SetConfig sets the package-level diagnostics configuration.
func SetConfig(config Config) {
	defer perf.Track(nil, "diagnostics.SetConfig")()

	configMu.Lock()
	defer configMu.Unlock()
	defaultConfig = config
}

// GetConfig returns the package-level diagnostics configuration.
func GetConfig() Config {
	defer perf.Track(nil, "diagnostics.GetConfig")()

	configMu.RLock()
	defer configMu.RUnlock()
	return defaultConfig
}

// NewID returns a process-local monotonically unique diagnostics event id.
func NewID(prefix string) string {
	defer perf.Track(nil, "diagnostics.NewID")()

	if prefix == "" {
		prefix = "event"
	}
	return fmt.Sprintf("%s-%d-%d-%d", prefix, os.Getpid(), time.Now().UnixNano(), atomic.AddUint64(&idCounter, 1))
}

// Enabled reports whether diagnostics should emit events for the current config.
func Enabled(config Config) bool {
	defer perf.Track(nil, "diagnostics.Enabled")()

	if strings.EqualFold(strings.TrimSpace(config.Level), levelOff) {
		return false
	}
	if strings.TrimSpace(config.File) == "" {
		return false
	}
	sink := strings.TrimSpace(config.Sink)
	return sink == "" || strings.EqualFold(sink, SinkFile)
}

// Emit appends one event to the package-level diagnostics sink.
func Emit(event *Event) error {
	defer perf.Track(nil, "diagnostics.Emit")()
	return EmitWithConfig(GetConfig(), event)
}

// EmitWithConfig appends one event to the provided diagnostics sink.
func EmitWithConfig(config Config, event *Event) error {
	defer perf.Track(nil, "diagnostics.EmitWithConfig")()

	if !Enabled(config) {
		return nil
	}
	if event == nil {
		return nil
	}
	if event.Time.IsZero() {
		event.Time = time.Now().UTC()
	}
	if event.Level == "" {
		event.Level = LevelDebug
	}

	redactEvent(event)

	fileMu.Lock()
	defer fileMu.Unlock()

	if dir := filepath.Dir(config.File); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, dirMode); err != nil {
			return err
		}
	}
	f, err := os.OpenFile(config.File, os.O_CREATE|os.O_WRONLY|os.O_APPEND, eventLogMode)
	if err != nil {
		return err
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetEscapeHTML(false)
	return enc.Encode(event)
}

func redactEvent(event *Event) {
	event.Command = iolib.MaskString(event.Command)
	event.CWD = iolib.MaskString(event.CWD)
	event.RedirectStderr = iolib.MaskString(event.RedirectStderr)
	event.Signal = iolib.MaskString(event.Signal)
	event.Error = iolib.MaskString(event.Error)
	for i, arg := range event.Args {
		event.Args[i] = iolib.MaskString(arg)
	}
}

// Bool returns a pointer for optional bool event fields.
func Bool(value bool) *bool {
	defer perf.Track(nil, "diagnostics.Bool")()

	return &value
}

// Int returns a pointer for optional int event fields.
func Int(value int) *int {
	defer perf.Track(nil, "diagnostics.Int")()

	return &value
}

// Int64 returns a pointer for optional int64 event fields.
func Int64(value int64) *int64 {
	defer perf.Track(nil, "diagnostics.Int64")()

	return &value
}
