package asciicast

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/creack/pty"

	iolib "github.com/cloudposse/atmos/pkg/io"
)

type SessionAction struct {
	Type     string
	Text     string
	Regex    string
	Key      string
	Duration string
	Timeout  string
	Rate     string
	Interval string
	Repeat   int
}

type SessionOptions struct {
	Shell       string
	Width       int
	Height      int
	WriteRate   time.Duration
	KeyInterval time.Duration
	Actions     []SessionAction
}

func RunSession(ctx context.Context, opts SessionOptions) error {
	if opts.Width <= 0 {
		opts.Width = DefaultWidth
	}
	if opts.Height <= 0 {
		opts.Height = DefaultHeight
	}
	if opts.WriteRate < 0 {
		opts.WriteRate = 0
	}
	if opts.KeyInterval < 0 {
		opts.KeyInterval = 0
	}
	shell := opts.Shell
	if shell == "" {
		shell = os.Getenv("SHELL")
	}
	if shell == "" {
		if runtime.GOOS == "windows" {
			shell = "cmd.exe"
		} else {
			shell = "/bin/sh"
		}
	}
	cmd := exec.CommandContext(ctx, shell)
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: uint16(opts.Width), Rows: uint16(opts.Height)})
	if err != nil {
		return fmt.Errorf("start cast session shell: %w", err)
	}
	defer func() { _ = ptmx.Close() }()

	var mu sync.Mutex
	var output bytes.Buffer
	done := make(chan error, 1)
	changed := make(chan struct{}, 1)
	go func() {
		buf := make([]byte, 4096)
		for {
			n, readErr := ptmx.Read(buf)
			if n > 0 {
				chunk := append([]byte(nil), buf[:n]...)
				mu.Lock()
				_, _ = output.Write(chunk)
				mu.Unlock()
				select {
				case changed <- struct{}{}:
				default:
				}
				_, _ = iolib.GetContext().Data().Write(chunk)
			}
			if readErr != nil {
				if readErr == io.EOF || strings.Contains(readErr.Error(), "input/output error") {
					done <- nil
				} else {
					done <- readErr
				}
				return
			}
		}
	}()

	for _, action := range opts.Actions {
		if err := runAction(ctx, ptmx, &mu, &output, changed, action, opts); err != nil {
			_ = cmd.Process.Kill()
			return err
		}
	}
	// Send EOT so interactive shells exit without echoing an artificial
	// "exit" command into the recording.
	_, _ = ptmx.Write([]byte{4})
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		_ = cmd.Process.Kill()
		return ctx.Err()
	case <-time.After(2 * time.Second):
		_ = cmd.Process.Kill()
		return nil
	}
}

func runAction(ctx context.Context, ptyFile *os.File, mu *sync.Mutex, output *bytes.Buffer, changed <-chan struct{}, action SessionAction, opts SessionOptions) error {
	switch action.Type {
	case "write":
		rate := opts.WriteRate
		if action.Rate != "" {
			parsed, err := time.ParseDuration(action.Rate)
			if err != nil {
				return fmt.Errorf("invalid write rate %q: %w", action.Rate, err)
			}
			rate = parsed
		}
		for _, r := range action.Text {
			if _, err := ptyFile.Write([]byte(string(r))); err != nil {
				return err
			}
			if rate > 0 {
				time.Sleep(rate)
			}
		}
	case "key":
		repeat := action.Repeat
		if repeat <= 0 {
			repeat = 1
		}
		seq, err := keySequence(action.Key)
		if err != nil {
			return err
		}
		for i := 0; i < repeat; i++ {
			if _, err := ptyFile.Write([]byte(seq)); err != nil {
				return err
			}
			interval := opts.KeyInterval
			if action.Interval != "" {
				parsed, err := time.ParseDuration(action.Interval)
				if err != nil {
					return fmt.Errorf("invalid key interval %q: %w", action.Interval, err)
				}
				interval = parsed
			}
			if interval > 0 && i < repeat-1 {
				time.Sleep(interval)
			}
		}
	case "pause":
		duration, err := time.ParseDuration(action.Duration)
		if err != nil {
			return fmt.Errorf("invalid pause duration %q: %w", action.Duration, err)
		}
		timer := time.NewTimer(duration)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			return ctx.Err()
		}
	case "wait":
		return waitForOutput(ctx, mu, output, changed, action)
	default:
		return fmt.Errorf("unknown cast session action type %q", action.Type)
	}
	return nil
}

func waitForOutput(ctx context.Context, mu *sync.Mutex, output *bytes.Buffer, changed <-chan struct{}, action SessionAction) error {
	timeout := 30 * time.Second
	if action.Timeout != "" {
		parsed, err := time.ParseDuration(action.Timeout)
		if err != nil {
			return fmt.Errorf("invalid wait timeout %q: %w", action.Timeout, err)
		}
		timeout = parsed
	}
	var re *regexp.Regexp
	var err error
	if action.Regex != "" {
		re, err = regexp.Compile(action.Regex)
		if err != nil {
			return err
		}
	}
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for {
		mu.Lock()
		current := output.String()
		mu.Unlock()
		if action.Text != "" && strings.Contains(current, action.Text) {
			return nil
		}
		if re != nil && re.MatchString(current) {
			return nil
		}
		select {
		case <-changed:
		case <-deadline.C:
			return fmt.Errorf("timed out waiting for cast output")
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func keySequence(key string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "enter", "return":
		return "\r", nil
	case "tab":
		return "\t", nil
	case "esc", "escape":
		return "\x1b", nil
	case "backspace":
		return "\x7f", nil
	case "space":
		return " ", nil
	case "up":
		return "\x1b[A", nil
	case "down":
		return "\x1b[B", nil
	case "right":
		return "\x1b[C", nil
	case "left":
		return "\x1b[D", nil
	default:
		if len(key) == 1 {
			return key, nil
		}
		return "", fmt.Errorf("unsupported cast key %q", key)
	}
}
