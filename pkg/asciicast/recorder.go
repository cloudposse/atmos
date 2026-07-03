package asciicast

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/xdg"
)

const (
	// DefaultWidth is the fallback terminal width for cast recordings.
	DefaultWidth = 120
	// DefaultHeight is the fallback terminal height for cast recordings.
	DefaultHeight = 36

	castDirPerm    = 0o755
	castFilePerm   = 0o644
	defaultIDLen   = 6
	slugMaxLen     = 64
	slugSeparator  = "-"
	defaultCastCmd = "atmos"
)

// ErrCastOutputExists indicates that a requested cast output path already exists.
var ErrCastOutputExists = errUtils.ErrCastOutputExists

// Options configures an asciicast recorder.
type Options struct {
	Path       string
	BasePath   string
	Name       string
	Title      string
	Command    []string
	Width      int
	Height     int
	RecordIn   bool
	Explicit   bool
	Env        map[string]string
	Now        func() time.Time
	Executable string
	OutputRate time.Duration
}

// Recorder writes asciicast v3 header and event records to a file.
type Recorder struct {
	mu            sync.Mutex
	file          *os.File
	writer        *bufio.Writer
	started       time.Time
	closed        bool
	path          string
	recordIn      bool
	width         int
	height        int
	title         string
	command       string
	outputRate    time.Duration
	lastEventTime time.Duration
}

// Term describes the recorded terminal in asciicast v3 headers.
type Term struct {
	Cols int    `json:"cols"`
	Rows int    `json:"rows"`
	Type string `json:"type,omitempty"`
}

// Header is the asciicast header written as the first line of a recording.
// Atmos writes v3 headers, but keeps the v2 width/height fields for legacy reads.
type Header struct {
	Version   int               `json:"version"`
	Width     int               `json:"width,omitempty"`
	Height    int               `json:"height,omitempty"`
	Term      *Term             `json:"term,omitempty"`
	Timestamp int64             `json:"timestamp,omitempty"`
	Title     string            `json:"title,omitempty"`
	Command   string            `json:"command,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
}

var writeRecorderHeader = (*Recorder).writeJSON

// Start creates a new recorder and writes its asciicast header.
func Start(opts *Options) (*Recorder, error) {
	defer perf.Track(nil, "asciicast.Start")()

	now := time.Now
	if opts == nil {
		opts = &Options{}
	}
	if opts.Now != nil {
		now = opts.Now
	}
	started := now()
	width := opts.Width
	if width <= 0 {
		width = DefaultWidth
	}
	height := opts.Height
	if height <= 0 {
		height = DefaultHeight
	}

	path, err := ResolvePath(opts, started)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), castDirPerm); err != nil {
		return nil, fmt.Errorf("create cast directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, castFilePerm)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("%w: %s", ErrCastOutputExists, path)
		}
		return nil, fmt.Errorf("create cast file: %w", err)
	}

	rec := &Recorder{
		file:       file,
		writer:     bufio.NewWriter(file),
		started:    started,
		path:       path,
		recordIn:   opts.RecordIn,
		width:      width,
		height:     height,
		title:      strings.TrimSpace(opts.Title),
		command:    strings.Join(opts.Command, " "),
		outputRate: opts.OutputRate,
	}
	if err := writeRecorderHeader(rec, newRecorderHeader(rec, opts, started)); err != nil {
		_ = rec.Close()
		_ = os.Remove(path)
		return nil, err
	}
	return rec, nil
}

// newRecorderHeader builds the asciicast v3 header for a new recording.
func newRecorderHeader(rec *Recorder, opts *Options, started time.Time) Header {
	return Header{
		Version: 3,
		Term: &Term{
			Cols: rec.width,
			Rows: rec.height,
			Type: terminalType(opts.Env),
		},
		Timestamp: started.Unix(),
		Title:     rec.title,
		Command:   rec.command,
		Env:       safeEnvV3(opts.Env),
	}
}

// Path returns the cast file path used by the recorder.
func (r *Recorder) Path() string {
	defer perf.Track(nil, "asciicast.Recorder.Path")()

	if r == nil {
		return ""
	}
	return r.path
}

// Record writes stream content as an asciicast event, applying input-recording rules.
func (r *Recorder) Record(stream, content string) {
	defer perf.Track(nil, "asciicast.Recorder.Record")()

	if r == nil || content == "" {
		return
	}
	if stream == "i" && !r.recordIn {
		return
	}
	if stream != "i" && stream != "o" && stream != "e" {
		stream = "o"
	}
	_ = r.Event(stream, content)
}

// Event writes a single asciicast event to the recording.
func (r *Recorder) Event(stream, content string) error {
	defer perf.Track(nil, "asciicast.Recorder.Event")()

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil
	}
	return r.writeEventLocked(stream, content)
}

// Resize records a terminal resize event.
func (r *Recorder) Resize(width, height int) error {
	defer perf.Track(nil, "asciicast.Recorder.Resize")()

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil
	}
	return r.writeEventLocked("r", fmt.Sprintf("%dx%d", width, height))
}

// Close flushes and closes the underlying cast file.
func (r *Recorder) Close() error {
	defer perf.Track(nil, "asciicast.Recorder.Close")()

	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil
	}
	r.closed = true
	var err error
	if r.writer != nil {
		err = r.writer.Flush()
	}
	if closeErr := r.file.Close(); err == nil {
		err = closeErr
	}
	return err
}

func (r *Recorder) writeJSON(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if _, err := r.writer.Write(b); err != nil {
		return err
	}
	if err := r.writer.WriteByte('\n'); err != nil {
		return err
	}
	return nil
}

func (r *Recorder) writeEventLocked(stream, content string) error {
	now := time.Since(r.started)
	if r.outputRate <= 0 || stream == "i" || stream == "r" {
		eventTime := maxDuration(now, r.lastEventTime)
		return r.writeRelativeEvent(eventTime, stream, content)
	}

	eventTime := maxDuration(now, r.lastEventTime)
	for _, chunk := range splitTerminalLines(content) {
		if err := r.writeRelativeEvent(eventTime, stream, chunk); err != nil {
			return err
		}
		if strings.HasSuffix(chunk, "\n") {
			eventTime += r.outputRate
		}
	}
	return nil
}

func (r *Recorder) writeRelativeEvent(eventTime time.Duration, stream, content string) error {
	delta := eventTime - r.lastEventTime
	if delta < 0 {
		delta = 0
	}
	if err := r.writeJSON([]any{delta.Seconds(), stream, content}); err != nil {
		return err
	}
	r.lastEventTime = eventTime
	return nil
}

func splitTerminalLines(content string) []string {
	if !strings.Contains(content, "\n") {
		return []string{content}
	}
	chunks := make([]string, 0, strings.Count(content, "\n")+1)
	start := 0
	for index, r := range content {
		if r != '\n' {
			continue
		}
		chunks = append(chunks, content[start:index+1])
		start = index + 1
	}
	if start < len(content) {
		chunks = append(chunks, content[start:])
	}
	return chunks
}

func maxDuration(a, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
}

// ResolvePath returns the cast output path for the supplied options and start time.
func ResolvePath(opts *Options, started time.Time) (string, error) {
	defer perf.Track(nil, "asciicast.ResolvePath")()

	if opts == nil {
		opts = &Options{}
	}
	if opts.Path != "" {
		return filepath.Clean(opts.Path), nil
	}
	base := opts.BasePath
	if base == "" {
		var err error
		base, err = xdg.GetXDGCacheDir("casts", xdg.DefaultCacheDirPerm)
		if err != nil {
			return "", err
		}
	}
	slug := CommandSlug(opts.Command)
	if slug == "" {
		slug = CommandSlug(strings.Fields(opts.Title))
	}
	if slug == "" {
		slug = CommandSlug([]string{opts.Name})
	}
	if slug == "" {
		slug = defaultCastCmd
	}
	runID := strings.ToLower(RandomID(defaultIDLen))
	name := fmt.Sprintf("%s-%s-%s.cast", started.Format("150405"), slug, runID)
	return filepath.Join(base, started.Format("2006"), started.Format("01"), started.Format("02"), name), nil
}

// CommandSlug converts command arguments into a filesystem-safe cast filename slug.
func CommandSlug(args []string) string {
	defer perf.Track(nil, "asciicast.CommandSlug")()

	parts := make([]string, 0, len(args))
	for _, arg := range args {
		arg = strings.TrimSpace(arg)
		if arg == "" || strings.HasPrefix(arg, "-") {
			continue
		}
		base := filepath.Base(arg)
		if strings.TrimSuffix(base, ".exe") == defaultCastCmd {
			continue
		}
		parts = append(parts, base)
		if len(parts) == 4 {
			break
		}
	}
	slug := strings.ToLower(strings.Join(parts, slugSeparator))
	slug = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(slug, slugSeparator)
	slug = strings.Trim(slug, slugSeparator)
	if len(slug) > slugMaxLen {
		slug = strings.Trim(slug[:slugMaxLen], slugSeparator)
	}
	return slug
}

// RandomID returns a short lowercase hexadecimal identifier.
func RandomID(n int) string {
	defer perf.Track(nil, "asciicast.RandomID")()

	const letters = "0123456789abcdef"
	b := make([]byte, n)
	f, err := os.Open("/dev/urandom")
	if err == nil {
		defer func() { _ = f.Close() }()
		if _, err := io.ReadFull(f, b); err == nil {
			for i := range b {
				b[i] = letters[int(b[i])%len(letters)]
			}
			return string(b)
		}
	}
	t := time.Now().UnixNano()
	for i := range b {
		b[i] = letters[int(t>>uint(i*4))%len(letters)]
	}
	return string(b)
}

func safeEnv(env map[string]string) map[string]string {
	result := map[string]string{}
	for _, key := range []string{"SHELL", "TERM", "COLORTERM"} {
		if value := env[key]; value != "" {
			result[key] = value
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func safeEnvV3(env map[string]string) map[string]string {
	result := map[string]string{}
	for _, key := range []string{"SHELL", "COLORTERM"} {
		if value := env[key]; value != "" {
			result[key] = value
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func terminalType(env map[string]string) string {
	if env == nil {
		return ""
	}
	return env["TERM"]
}
