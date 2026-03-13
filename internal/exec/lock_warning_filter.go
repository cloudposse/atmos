package exec

import (
	"bytes"
	stdio "io"
	"strings"
)

// tfIncompleteLockWarning is the unique string in Terraform's diagnostic output that
// identifies the "Incomplete lock file information for providers" warning.
// This warning is emitted when providers are installed from TF_PLUGIN_CACHE_DIR without
// a registry connection, producing only local zh: checksums instead of cross-platform h1: checksums.
const tfIncompleteLockWarning = "Incomplete lock file information for providers"

// tfDiagStart is the UTF-8 sequence for ╷ (U+2577, BOX DRAWINGS LIGHT DOWN), which Terraform
// uses to open a diagnostic block on its own line.
const tfDiagStart = "╷"

// tfDiagEnd is the UTF-8 sequence for ╵ (U+2575, BOX DRAWINGS LIGHT UP), which Terraform
// uses to close a diagnostic block on its own line.
const tfDiagEnd = "╵"

// incompleteLockWarningFilter is an io.Writer that suppresses the specific Terraform
// "Incomplete lock file information for providers" diagnostic block from stderr.
//
// This warning is expected and non-actionable when Terraform initializes providers from
// a plugin cache (TF_PLUGIN_CACHE_DIR) in a workdir where the lock file is intentionally
// ephemeral (e.g. the workdir provisioner creates a fresh isolated directory per run).
// Suppressing it keeps the output clean without changing Terraform's behaviour.
//
// The filter buffers each Terraform diagnostic block (the region between a lone ╷ line
// and its matching lone ╵ line).  If the block contains the lock warning, the entire
// block is silently dropped; otherwise it is forwarded verbatim to the underlying writer.
// Any output that is not part of a diagnostic block is forwarded immediately.
type incompleteLockWarningFilter struct {
	underlying stdio.Writer
	buf        []byte // partial line not yet terminated by '\n'
	blockBuf   []byte // accumulated bytes for the current diagnostic block
	inBlock    bool   // currently inside a ╷…╵ block
	suppress   bool   // the current block has been identified as the lock warning
}

// newIncompleteLockWarningFilter returns a filter writer that wraps underlying and
// suppresses Terraform's "Incomplete lock file information for providers" diagnostic block.
func newIncompleteLockWarningFilter(underlying stdio.Writer) *incompleteLockWarningFilter {
	return &incompleteLockWarningFilter{underlying: underlying}
}

// Write buffers incoming bytes, splits them into complete lines and dispatches each line
// through processLine.  Any trailing partial line (no '\n' yet) is retained in buf.
func (f *incompleteLockWarningFilter) Write(p []byte) (int, error) {
	f.buf = append(f.buf, p...)

	for {
		idx := bytes.IndexByte(f.buf, '\n')
		if idx < 0 {
			break
		}
		line := f.buf[:idx+1]
		f.buf = f.buf[idx+1:]
		if err := f.processLine(line); err != nil {
			return 0, err
		}
	}

	return len(p), nil
}

// processLine dispatches a single complete line (including its trailing '\n') through
// the diagnostic-block state machine.
func (f *incompleteLockWarningFilter) processLine(line []byte) error {
	trimmed := strings.TrimRight(string(line), "\r\n")

	if !f.inBlock {
		// ╷ on a line by itself signals the start of a Terraform diagnostic block.
		if trimmed == tfDiagStart {
			f.inBlock = true
			f.suppress = false
			f.blockBuf = append(f.blockBuf[:0], line...)
			return nil
		}
		// Normal output – pass through immediately.
		_, err := f.underlying.Write(line)
		return err
	}

	// Inside a diagnostic block: buffer the line.
	f.blockBuf = append(f.blockBuf, line...)

	// Check whether this is the warning we want to suppress.
	if strings.Contains(trimmed, tfIncompleteLockWarning) {
		f.suppress = true
	}

	// ╵ on a line by itself signals the end of the diagnostic block.
	if trimmed == tfDiagEnd {
		f.inBlock = false
		if f.suppress {
			// Discard the buffered block entirely.
			f.blockBuf = f.blockBuf[:0]
			f.suppress = false
			return nil
		}
		// Not the warning we're suppressing – forward the buffered block.
		_, err := f.underlying.Write(f.blockBuf)
		f.blockBuf = f.blockBuf[:0]
		return err
	}

	return nil
}

// Close flushes any content that remains buffered.  Under normal operation (the
// subprocess exits cleanly) this is a no-op because Terraform always closes its
// diagnostic blocks.  It is provided as a safety valve for abnormal termination.
func (f *incompleteLockWarningFilter) Close() error {
	if len(f.blockBuf) > 0 {
		_, err := f.underlying.Write(f.blockBuf)
		f.blockBuf = nil
		if err != nil {
			return err
		}
	}
	if len(f.buf) > 0 {
		_, err := f.underlying.Write(f.buf)
		f.buf = nil
		return err
	}
	return nil
}
