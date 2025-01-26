package testhelper

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"sync"
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
)

var (
	// DisableBuffering can be set via ATMOS_TEST_NO_BUFFER=1
	DisableBuffering = os.Getenv("ATMOS_TEST_NO_BUFFER") == "1"
	outputMutex      sync.Mutex
)

// testOutputBuffer captures output for a test
type testOutputBuffer struct {
	t          *testing.T
	stdout     *os.File
	stderr     *os.File
	origStdout *os.File
	origStderr *os.File
	outBuf     *bytes.Buffer
	errBuf     *bytes.Buffer
	outReader  *os.File
	errReader  *os.File
}

// Run executes a test function with output buffering
// Usage:
//
//	func TestSomething(t *testing.T) {
//	    testhelper.Run(t, func(t *testing.T) {
//	        // Your test code here
//	        fmt.Println("This output will be buffered")
//	    })
//	}
func Run(t *testing.T, testFunc func(*testing.T)) {
	if DisableBuffering {
		testFunc(t)
		return
	}

	buf := captureTestOutput(t)
	defer buf.stopCapture()
	testFunc(t)
}

// RunWithConfig executes a test function with output buffering and ConfigAndStacksInfo
func RunWithConfig(t *testing.T, configAndStacksInfo schema.ConfigAndStacksInfo, testFunc func(*testing.T, schema.ConfigAndStacksInfo)) {
	if DisableBuffering {
		testFunc(t, configAndStacksInfo)
		return
	}

	buf := captureTestOutput(t)
	defer buf.stopCapture()
	testFunc(t, configAndStacksInfo)
}

// captureTestOutput starts capturing output for a test
func captureTestOutput(t *testing.T) *testOutputBuffer {
	if DisableBuffering {
		return nil
	}

	outReader, outWriter, err := os.Pipe()
	if err != nil {
		t.Fatalf("Failed to create stdout pipe: %v", err)
	}

	errReader, errWriter, err := os.Pipe()
	if err != nil {
		outReader.Close()
		outWriter.Close()
		t.Fatalf("Failed to create stderr pipe: %v", err)
	}

	buf := &testOutputBuffer{
		t:          t,
		stdout:     outWriter,
		stderr:     errWriter,
		origStdout: os.Stdout,
		origStderr: os.Stderr,
		outBuf:     &bytes.Buffer{},
		errBuf:     &bytes.Buffer{},
		outReader:  outReader,
		errReader:  errReader,
	}

	outputMutex.Lock()
	os.Stdout = buf.stdout
	os.Stderr = buf.stderr
	outputMutex.Unlock()

	go io.Copy(buf.outBuf, buf.outReader)
	go io.Copy(buf.errBuf, buf.errReader)

	return buf
}

// stopCapture stops capturing output and returns the buffers
func (b *testOutputBuffer) stopCapture() {
	if b == nil {
		return
	}

	outputMutex.Lock()
	defer outputMutex.Unlock()

	// Close writers first
	b.stdout.Close()
	b.stderr.Close()

	// Wait for readers to finish
	b.outReader.Close()
	b.errReader.Close()

	// Restore original stdout/stderr
	os.Stdout = b.origStdout
	os.Stderr = b.origStderr

	// If test has failed, dump the buffers
	if b.t.Failed() {
		if stdout := b.outBuf.String(); stdout != "" {
			fmt.Fprintf(b.origStdout, "\n=== Test Output (stdout) ===\n%s\n", stdout)
		}
		if stderr := b.errBuf.String(); stderr != "" {
			fmt.Fprintf(b.origStderr, "\n=== Test Output (stderr) ===\n%s\n", stderr)
		}
	}
}
