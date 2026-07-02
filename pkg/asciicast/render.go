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
	// ErrMissingSVGRenderer indicates that SVG output was requested without a supported renderer.
	ErrMissingSVGRenderer = errUtils.ErrMissingSVGRenderer
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
	if !scanner.Scan() {
		return Header{}, nil, fmt.Errorf("%w: %s", ErrEmptyCastFile, path)
	}
	var header Header
	if err := json.Unmarshal(scanner.Bytes(), &header); err != nil {
		return Header{}, nil, fmt.Errorf("decode cast header: %w", err)
	}
	var events []Event
	for scanner.Scan() {
		var raw []any
		if err := json.Unmarshal(scanner.Bytes(), &raw); err != nil {
			return Header{}, nil, fmt.Errorf("decode cast event: %w", err)
		}
		if len(raw) != 3 {
			continue
		}
		t, _ := raw[0].(float64)
		stream, _ := raw[1].(string)
		data, _ := raw[2].(string)
		events = append(events, Event{Time: t, Stream: stream, Data: data})
	}
	if err := scanner.Err(); err != nil {
		return Header{}, nil, err
	}
	return header, events, nil
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
	SVG string
	GIF string
	MP4 string
}

// Render generates requested media outputs from an asciicast file.
func Render(input string, opts RenderOptions) error {
	defer perf.Track(nil, "asciicast.Render")()

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

func renderTargets(opts RenderOptions) []renderTarget {
	targets := make([]renderTarget, 0, 3)
	if opts.SVG != "" {
		targets = append(targets, renderTarget{output: opts.SVG, render: renderSVG})
	}
	if opts.GIF != "" {
		targets = append(targets, renderTarget{output: opts.GIF, render: renderWithAgg})
	}
	if opts.MP4 != "" {
		targets = append(targets, renderTarget{output: opts.MP4, render: renderMP4})
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

func renderSVG(_, output string) error {
	return fmt.Errorf("render %s: %w", output, ErrMissingSVGRenderer)
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
