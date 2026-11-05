//go:build !windows

package doctor

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"testing"
	"time"

	"github.com/creack/pty"
	"github.com/stretchr/testify/require"

	diagnostic "github.com/cloudposse/atmos/pkg/doctor"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/ui"
)

type cancellationRepository struct{}

func (cancellationRepository) Check(ctx context.Context, _ string) diagnostic.RepositoryHealth {
	<-ctx.Done()
	return diagnostic.RepositoryHealth{Status: "inconclusive", Message: "Repository check canceled"}
}

// TestDoctorPTYChild runs only in an isolated helper process with a real terminal.
func TestDoctorPTYChild(t *testing.T) {
	if os.Getenv("ATMOS_DOCTOR_PTY_CHILD") != "1" {
		return
	}
	ioContext, err := iolib.NewContext()
	require.NoError(t, err)
	ui.InitFormatter(ioContext)
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	t.Cleanup(cancel)
	engine := diagnostic.Engine{Repositories: cancellationRepository{}}
	report, err := runStreaming(ctx, &engine, &diagnostic.Options{Only: []string{"sources"}})
	require.NoError(t, render(&report, "text", renderOptions{}))
	if errors.Is(err, context.Canceled) {
		os.Exit(130)
	}
	require.NoError(t, err)
}

func TestDoctorPTYStreamingResizeAndCancellation(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "atmos.yaml"), []byte("{}\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "vendor.yaml"), []byte("spec:\n  sources:\n    - uri: https://github.com/cloudposse/atmos\n"), 0o600))
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestDoctorPTYChild$")
	child.Dir = root
	child.Env = append(os.Environ(), "ATMOS_DOCTOR_PTY_CHILD=1", "TERM=xterm-256color", "NO_COLOR=1")
	terminal, err := pty.StartWithSize(child, &pty.Winsize{Rows: 24, Cols: 100})
	require.NoError(t, err)
	t.Cleanup(func() { _ = terminal.Close() })
	var output bytes.Buffer
	received := make(chan error, 1)
	go func() {
		buffer := make([]byte, 4096)
		resized := false
		for {
			n, readErr := terminal.Read(buffer)
			output.Write(buffer[:n])
			// Answer terminal capability probes as a real terminal emulator does.
			for query, response := range map[string]string{
				"\x1b]11;?\x1b\\": "\x1b]11;rgb:0000/0000/0000\x1b\\",
				"\x1b]10;?\x1b\\": "\x1b]10;rgb:ffff/ffff/ffff\x1b\\",
				"\x1b[6n":         "\x1b[1;1R",
			} {
				if bytes.Contains(buffer[:n], []byte(query)) {
					_, _ = terminal.Write([]byte(response))
				}
			}
			if !resized && bytes.Contains(output.Bytes(), []byte("Repository metadata")) {
				resized = true
				if err := pty.Setsize(terminal, &pty.Winsize{Rows: 12, Cols: 50}); err != nil {
					received <- err
					return
				}
				if err := child.Process.Signal(os.Interrupt); err != nil {
					received <- err
					return
				}
			}
			if readErr != nil {
				received <- nil
				return
			}
		}
	}()
	err = child.Wait()
	require.Error(t, err)
	var exit *exec.ExitError
	require.ErrorAs(t, err, &exit)
	require.NoError(t, <-received)
	require.Equal(t, 130, exit.ExitCode(), output.String())
	require.Contains(t, output.String(), "Project inputs")
	require.Contains(t, output.String(), "Repository check")
	require.Contains(t, output.String(), "canceled")
	require.Contains(t, output.String(), "1 finding (none fixable)")
	require.NotContains(t, output.String(), "/0 complete")
}
