package asciicast

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

var (
	// ErrEmptyCastFile indicates that a cast file had no header line.
	ErrEmptyCastFile = errUtils.ErrEmptyCastFile
	// ErrRenderOutputExists indicates that a render target already exists.
	ErrRenderOutputExists = errUtils.ErrRenderOutputExists
	// ErrMissingAgg indicates that the agg renderer executable was not found.
	ErrMissingAgg = errUtils.ErrMissingAgg
	// ErrMissingFFmpeg indicates that the ffmpeg executable was not found.
	ErrMissingFFmpeg = errUtils.ErrMissingFFmpeg
)

const (
	initialScanBuffer = 64 * 1024
	// MaxEventTokenSize bounds a single cast event line (16 MiB).
	maxEventTokenSize = 16 * 1024 * 1024
)

// Event is one asciicast v2 event entry.
type Event struct {
	Time   float64
	Stream string
	Data   string
}

// ReadEvents reads an asciicast file header and event stream.
func ReadEvents(path string) (Header, []Event, error) {
	defer perf.Track(nil, "asciicast.ReadEvents")()

	file, err := os.Open(path)
	if err != nil {
		return Header{}, nil, err
	}
	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)
	// A single event can carry large payloads (e.g. a shell completion script
	// in one write); the default 64 KiB token limit is too small.
	scanner.Buffer(make([]byte, 0, initialScanBuffer), maxEventTokenSize)
	if !scanner.Scan() {
		return Header{}, nil, fmt.Errorf("%w: %s", ErrEmptyCastFile, path)
	}
	var header Header
	if err := json.Unmarshal(scanner.Bytes(), &header); err != nil {
		return Header{}, nil, fmt.Errorf("decode cast header: %w", err)
	}
	var events []Event
	var absoluteTime float64
	for scanner.Scan() {
		event, ok, err := parseEventLine(scanner.Bytes())
		if err != nil {
			return Header{}, nil, err
		}
		if !ok {
			continue
		}
		// Unknown streams are excluded from the result but still advance the
		// v3 relative-time accumulator so later event times stay correct.
		if header.Version == 3 {
			absoluteTime += event.Time
			event.Time = absoluteTime
		}
		if isKnownStream(event.Stream) {
			events = append(events, event)
		}
	}
	if err := scanner.Err(); err != nil {
		return Header{}, nil, err
	}
	return header, events, nil
}

// parseEventLine decodes one asciicast event line. It returns ok=false for
// comments and structurally malformed records.
func parseEventLine(line []byte) (Event, bool, error) {
	if len(line) > 0 && line[0] == '#' {
		return Event{}, false, nil
	}
	var raw []any
	if err := json.Unmarshal(line, &raw); err != nil {
		return Event{}, false, fmt.Errorf("decode cast event: %w", err)
	}
	if len(raw) != 3 {
		return Event{}, false, nil
	}
	t, _ := raw[0].(float64)
	stream, _ := raw[1].(string)
	data, _ := raw[2].(string)
	return Event{Time: t, Stream: stream, Data: data}, true, nil
}

func isKnownStream(stream string) bool {
	switch stream {
	case "o", "i", "e", "r", "m":
		return true
	default:
		return false
	}
}

// Play replays an asciicast file to the provided writer.
func Play(path string, out io.Writer) error {
	defer perf.Track(nil, "asciicast.Play")()

	_, events, err := ReadEvents(path)
	if err != nil {
		return err
	}
	var previous float64
	for _, event := range events {
		if event.Stream != "o" && event.Stream != "e" {
			continue
		}
		if event.Time > previous {
			time.Sleep(time.Duration((event.Time - previous) * float64(time.Second)))
		}
		if _, err := io.WriteString(out, event.Data); err != nil {
			return err
		}
		previous = event.Time
	}
	return nil
}

// RenderOptions selects the render outputs to generate from a cast file.
type RenderOptions struct {
	GIF   string
	MP4   string
	HTML  string
	ASCII string
	PNG   string
	JPEG  string
}

// Render generates requested media outputs from an asciicast file.
func Render(input string, opts *RenderOptions) error {
	defer perf.Track(nil, "asciicast.Render")()

	if opts == nil {
		return nil
	}
	targets := renderTargets(opts)
	for _, target := range targets {
		if err := prepareRenderOutput(target.output); err != nil {
			return err
		}
	}
	for _, target := range targets {
		if err := target.render(input, target.output); err != nil {
			return err
		}
	}
	return nil
}

type renderTarget struct {
	output string
	render func(input, output string) error
}

func renderTargets(opts *RenderOptions) []renderTarget {
	specs := []struct {
		output string
		render func(input, output string) error
	}{
		{opts.GIF, renderWithAgg},
		{opts.MP4, renderMP4},
		{opts.HTML, RenderHTML},
		{opts.ASCII, RenderASCII},
		{opts.PNG, RenderPNG},
		{opts.JPEG, RenderJPEG},
	}
	targets := make([]renderTarget, 0, len(specs))
	for _, spec := range specs {
		if spec.output != "" {
			targets = append(targets, renderTarget{output: spec.output, render: spec.render})
		}
	}
	return targets
}

func prepareRenderOutput(output string) error {
	if _, err := os.Stat(output); err == nil {
		return fmt.Errorf("%w: %s", ErrRenderOutputExists, output)
	}
	dir := filepath.Dir(output)
	if dir == "." {
		return nil
	}
	return os.MkdirAll(dir, castDirPerm)
}

func renderWithAgg(input, output string) error {
	agg, err := exec.LookPath("agg")
	if err != nil {
		return fmt.Errorf("render %s: %w", output, ErrMissingAgg)
	}
	//nolint:gosec // agg is resolved via PATH and receives cast/output paths as argv, not shell input.
	cmd := exec.Command(agg, input, output)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func renderMP4(input, output string) error {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return fmt.Errorf("render %s: %w", output, ErrMissingFFmpeg)
	}
	tmp, err := os.CreateTemp("", "atmos-cast-*.gif")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	defer func() { _ = os.Remove(tmpPath) }()
	if err := renderWithAgg(input, tmpPath); err != nil {
		return err
	}
	ffmpeg, _ := exec.LookPath("ffmpeg")
	//nolint:gosec // ffmpeg is resolved via PATH and receives file paths as argv, not shell input.
	cmd := exec.Command(ffmpeg, "-y", "-i", tmpPath, "-movflags", "+faststart", output)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
