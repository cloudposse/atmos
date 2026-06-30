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
)

type Event struct {
	Time   float64
	Stream string
	Data   string
}

func ReadEvents(path string) (Header, []Event, error) {
	file, err := os.Open(path)
	if err != nil {
		return Header{}, nil, err
	}
	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		return Header{}, nil, fmt.Errorf("empty cast file: %s", path)
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

func Play(path string, out io.Writer) error {
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

type RenderOptions struct {
	SVG string
	GIF string
	MP4 string
}

func Render(input string, opts RenderOptions) error {
	for _, output := range []string{opts.SVG, opts.GIF, opts.MP4} {
		if output == "" {
			continue
		}
		if _, err := os.Stat(output); err == nil {
			return fmt.Errorf("render output already exists: %s", output)
		}
		if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil && filepath.Dir(output) != "." {
			return err
		}
	}
	if opts.SVG != "" {
		if err := renderWithAgg(input, opts.SVG); err != nil {
			return err
		}
	}
	if opts.GIF != "" {
		if err := renderWithAgg(input, opts.GIF); err != nil {
			return err
		}
	}
	if opts.MP4 != "" {
		if err := renderMP4(input, opts.MP4); err != nil {
			return err
		}
	}
	return nil
}

func renderWithAgg(input, output string) error {
	agg, err := exec.LookPath("agg")
	if err != nil {
		return fmt.Errorf("render %s: missing required tool `agg`; install asciinema agg and retry", output)
	}
	cmd := exec.Command(agg, input, output)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func renderMP4(input, output string) error {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return fmt.Errorf("render %s: missing required tool `ffmpeg`; install FFmpeg and retry", output)
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
	cmd := exec.Command(ffmpeg, "-y", "-i", tmpPath, "-movflags", "+faststart", output)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
