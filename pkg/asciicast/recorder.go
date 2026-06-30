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

	"github.com/cloudposse/atmos/pkg/xdg"
)

const (
	DefaultWidth  = 120
	DefaultHeight = 36
)

type Options struct {
	Path       string
	BasePath   string
	Command    []string
	Width      int
	Height     int
	RecordIn   bool
	Explicit   bool
	Env        map[string]string
	Now        func() time.Time
	Executable string
}

type Recorder struct {
	mu        sync.Mutex
	file      *os.File
	writer    *bufio.Writer
	started   time.Time
	closed    bool
	path      string
	recordIn  bool
	width     int
	height    int
	command   string
	eventSink io.Writer
}

type Header struct {
	Version   int               `json:"version"`
	Width     int               `json:"width"`
	Height    int               `json:"height"`
	Timestamp int64             `json:"timestamp"`
	Command   string            `json:"command,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
}

func Start(opts Options) (*Recorder, error) {
	now := time.Now
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
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create cast directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("cast output already exists: %s", path)
		}
		return nil, fmt.Errorf("create cast file: %w", err)
	}

	rec := &Recorder{
		file:     file,
		writer:   bufio.NewWriter(file),
		started:  started,
		path:     path,
		recordIn: opts.RecordIn,
		width:    width,
		height:   height,
		command:  strings.Join(opts.Command, " "),
	}
	header := Header{
		Version:   2,
		Width:     width,
		Height:    height,
		Timestamp: started.Unix(),
		Command:   rec.command,
		Env:       safeEnv(opts.Env),
	}
	if err := rec.writeJSON(header); err != nil {
		_ = rec.Close()
		return nil, err
	}
	return rec, nil
}

func (r *Recorder) Path() string {
	if r == nil {
		return ""
	}
	return r.path
}

func (r *Recorder) Record(stream, content string) {
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

func (r *Recorder) Event(stream, content string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil
	}
	return r.writeJSON([]any{time.Since(r.started).Seconds(), stream, content})
}

func (r *Recorder) Resize(width, height int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil
	}
	return r.writeJSON([]any{time.Since(r.started).Seconds(), "r", fmt.Sprintf("%dx%d", width, height)})
}

func (r *Recorder) Close() error {
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

func ResolvePath(opts Options, started time.Time) (string, error) {
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
		slug = "atmos"
	}
	runID := strings.ToLower(RandomID(6))
	name := fmt.Sprintf("%s-%s-%s.cast", started.Format("150405"), slug, runID)
	return filepath.Join(base, started.Format("2006"), started.Format("01"), started.Format("02"), name), nil
}

func CommandSlug(args []string) string {
	parts := make([]string, 0, len(args))
	for _, arg := range args {
		arg = strings.TrimSpace(arg)
		if arg == "" || strings.HasPrefix(arg, "-") {
			continue
		}
		arg = strings.TrimPrefix(filepath.Base(arg), "atmos")
		if arg == "" {
			continue
		}
		parts = append(parts, arg)
		if len(parts) == 4 {
			break
		}
	}
	slug := strings.ToLower(strings.Join(parts, "-"))
	slug = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")
	if len(slug) > 64 {
		slug = strings.Trim(slug[:64], "-")
	}
	return slug
}

func RandomID(n int) string {
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
