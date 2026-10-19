package process

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"reflect"
	"sync"
)

// scriptPipe owns a stream independently of exec.Cmd.Wait. Inherited write handles may remain
// open after the interpreter exits; normal completion must drain them rather than truncate them.
type scriptPipe struct {
	reader *os.File
	writer *os.File
	sink   io.Writer
}

func newScriptPipe(sink io.Writer) (*scriptPipe, error) {
	reader, writer, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	return &scriptPipe{reader: reader, writer: writer, sink: writerOrDiscard(sink)}, nil
}

func (p *scriptPipe) close() {
	_ = p.reader.Close()
	_ = p.writer.Close()
}

func (p *scriptPipe) drain(cancel context.CancelFunc, done chan<- error) {
	_, err := io.Copy(p.sink, p.reader)
	if err != nil {
		// A broken destination must also interrupt the producer and the other stream.
		cancel()
	}
	done <- err
}

// coordinateScriptCancellation lets exec.Cmd and the drain watcher share one termination.
func coordinateScriptCancellation(cmd *exec.Cmd) {
	// Both exec.Cmd and the drain watcher may observe cancellation. Signal/escalate only once,
	// and do not let a completed invocation leave a delayed signal targeting a reused PID.
	terminate := cmd.Cancel
	var terminateOnce sync.Once
	var terminateErr error
	cmd.Cancel = func() error {
		terminateOnce.Do(func() { terminateErr = terminate() })
		return terminateErr
	}
}

// runScriptAndDrain waits for both the interpreter and its inherited output streams. Cancellation
// stays active until draining completes, even after exec.Cmd's context watcher has stopped.
func runScriptAndDrain(ctx context.Context, cancel context.CancelFunc, cmd *exec.Cmd, stdout, stderr io.Writer) error {
	out, err := newScriptPipe(stdout)
	if err != nil {
		return err
	}
	defer out.close()
	diagnostic := out
	// Like os/exec, merge identical destinations into one pipe. This preserves write ordering
	// and avoids concurrently writing a shared buffer that does not provide its own locking.
	if stdout == nil || !reflect.TypeOf(stdout).Comparable() || stdout != stderr {
		diagnostic, err = newScriptPipe(stderr)
		if err != nil {
			return err
		}
	}
	defer diagnostic.close()
	cmd.Stdout, cmd.Stderr = out.writer, diagnostic.writer
	group := newChildGroup(cmd)
	// Output draining has no success deadline. Cancellation has its own bounded cleanup below.
	cmd.WaitDelay = 0

	coordinateScriptCancellation(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	group.started(cancel)
	defer group.finished()
	_ = out.writer.Close()
	_ = diagnostic.writer.Close()

	canceled := make(chan struct{})
	stopCancellation := context.AfterFunc(ctx, func() {
		defer close(canceled)
		_ = cmd.Cancel()
		// A descendant outside the process group can still hold a pipe. After cancellation's
		// termination grace, stop waiting for EOF and return the cancellation as an error.
		_ = out.reader.Close()
		_ = diagnostic.reader.Close()
	})
	defer func() {
		if !stopCancellation() {
			<-canceled
		}
	}()
	drained := make(chan error, 2)
	go out.drain(cancel, drained)
	if diagnostic != out {
		go diagnostic.drain(cancel, drained)
	} else {
		drained <- nil
	}
	waitErr := cmd.Wait()
	outErr, diagnosticErr := <-drained, <-drained
	return errors.Join(waitErr, outErr, diagnosticErr, ctx.Err())
}
