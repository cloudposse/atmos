// Package analyze provides AI-powered analysis of CLI command output.
// When the --ai flag is set, command output (stdout/stderr) is captured
// and sent to the configured AI provider for analysis after execution.
package analyze

import (
	"bytes"
	"io"
	"os"
	"sync"

	"github.com/cloudposse/atmos/pkg/perf"
)

// CaptureSession captures stdout and stderr while teeing to the original destinations.
// It uses os.Pipe() to intercept file descriptor writes, ensuring both Go-level writes
// (via io.Data/io.UI) and subprocess writes (terraform, helmfile) are captured.
type CaptureSession struct {
	oldStdout *os.File
	oldStderr *os.File
	stdoutW   *os.File
	stderrW   *os.File

	stdoutBuf bytes.Buffer
	stderrBuf bytes.Buffer

	wg sync.WaitGroup
}

// StartCapture begins capturing stdout and stderr.
// Output continues to flow to the terminal while also being buffered.
// Call Stop() when done to restore original streams and get captured output.
func StartCapture() (*CaptureSession, error) {
	defer perf.Track(nil, "analyze.StartCapture")()

	cs := &CaptureSession{
		oldStdout: os.Stdout,
		oldStderr: os.Stderr,
	}

	// Create pipes for stdout.
	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		return nil, err
	}

	// Create pipes for stderr.
	stderrR, stderrW, err := os.Pipe()
	if err != nil {
		stdoutR.Close()
		stdoutW.Close()
		return nil, err
	}

	cs.stdoutW = stdoutW
	cs.stderrW = stderrW

	// Replace os.Stdout/os.Stderr with pipe write ends.
	// Dynamic writers in pkg/io resolve os.Stdout at write time, so they pick this up.
	os.Stdout = stdoutW
	os.Stderr = stderrW

	// Tee goroutines: read from pipes, write to both original streams and buffers.
	cs.wg.Add(2) //nolint:mnd // Two goroutines for stdout and stderr.
	go func() {
		defer cs.wg.Done()
		_, _ = io.Copy(io.MultiWriter(cs.oldStdout, &cs.stdoutBuf), stdoutR)
		stdoutR.Close()
	}()
	go func() {
		defer cs.wg.Done()
		_, _ = io.Copy(io.MultiWriter(cs.oldStderr, &cs.stderrBuf), stderrR)
		stderrR.Close()
	}()

	return cs, nil
}

// Stop restores the original stdout/stderr and returns the captured output.
// It blocks until all captured data has been flushed.
func (cs *CaptureSession) Stop() (stdout, stderr string) {
	defer perf.Track(nil, "analyze.CaptureSession.Stop")()

	// Restore original streams first so any subsequent writes go directly to terminal.
	os.Stdout = cs.oldStdout
	os.Stderr = cs.oldStderr

	// Close pipe write ends to signal EOF to tee goroutines.
	cs.stdoutW.Close()
	cs.stderrW.Close()

	// Wait for tee goroutines to finish flushing.
	cs.wg.Wait()

	return cs.stdoutBuf.String(), cs.stderrBuf.String()
}
