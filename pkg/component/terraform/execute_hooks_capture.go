package terraform

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"sync"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/component/execution"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

// phaseCapture owns the temporary files that record phase and hook output for error hook
// matching. Cleanup removes every file once the lifecycle, including matching, has finished.
type phaseCapture struct {
	mu    sync.Mutex
	files []*captureFile
}

// create starts a new capture file. If the file cannot be created, the command still runs and
// the capture reports the failure when error hooks read it.
func (c *phaseCapture) create() *captureFile {
	file, err := os.CreateTemp("", "atmos-phase-output-*")
	capture := &captureFile{file: file}
	if err != nil {
		capture.err = captureError(err)
		return capture
	}
	capture.path = file.Name()
	c.mu.Lock()
	c.files = append(c.files, capture)
	c.mu.Unlock()
	return capture
}

// cleanup closes and removes every capture file.
func (c *phaseCapture) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, file := range c.files {
		file.close()
		if err := os.Remove(file.path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			log.Debug("Failed to remove captured command output", "path", file.path, "error", err)
		}
	}
	c.files = nil
}

// captureFile records output in a temporary file. A write failure is kept and reported when
// the output is read. The command keeps running, so a full disk cannot fail it.
type captureFile struct {
	mu   sync.Mutex
	file *os.File
	path string
	err  error
}

// Write appends to the capture and always reports success to the writer chain.
//
//nolint:lintroller // Called for every output chunk; tracking would dominate the copy.
func (c *captureFile) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err == nil && c.file != nil {
		if _, err := c.file.Write(p); err != nil {
			c.err = captureError(err)
		}
	}
	return len(p), nil
}

// finish closes the file for writing and returns it as lifecycle output.
func (c *captureFile) finish() execution.Output {
	c.close()
	return c
}

func (c *captureFile) close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.file == nil {
		return
	}
	if err := c.file.Close(); err != nil && c.err == nil {
		c.err = captureError(err)
	}
	c.file = nil
}

// Open returns a reader over the captured output, or the capture failure.
func (c *captureFile) Open() (io.Reader, error) {
	defer perf.Track(nil, "terraform.captureFile.Open")()

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err != nil {
		return nil, c.err
	}
	file, err := os.Open(c.path)
	if err != nil {
		return nil, captureError(err)
	}
	return file, nil
}

// captureError marks a failure to record or read captured command output.
func captureError(err error) error {
	return fmt.Errorf("%w: %w", errUtils.ErrComponentOutputCapture, err)
}
