package git

import (
	"bytes"
	stdio "io"
	"os"
	"testing"

	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
)

// testStreams provides in-memory stdin/stdout/stderr for tests.
type testStreams struct {
	stdin  stdio.Reader
	stdout stdio.Writer
	stderr stdio.Writer
}

func (ts *testStreams) Input() stdio.Reader     { return ts.stdin }
func (ts *testStreams) Output() stdio.Writer    { return ts.stdout }
func (ts *testStreams) Error() stdio.Writer     { return ts.stderr }
func (ts *testStreams) RawOutput() stdio.Writer { return ts.stdout }
func (ts *testStreams) RawError() stdio.Writer  { return ts.stderr }

// TestMain initializes the data and ui writers before any test runs.
// The data package requires an I/O context before Write functions can be called.
func TestMain(m *testing.M) {
	streams := &testStreams{
		stdin:  &bytes.Buffer{},
		stdout: &bytes.Buffer{},
		stderr: &bytes.Buffer{},
	}

	ioCtx, err := iolib.NewContext(iolib.WithStreams(streams))
	if err != nil {
		panic("failed to create I/O context for tests: " + err.Error())
	}

	data.InitWriter(ioCtx)

	os.Exit(m.Run())
}
