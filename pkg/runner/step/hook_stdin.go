package step

import (
	"bytes"
	"io"
	"sync"

	"github.com/cloudposse/atmos/pkg/perf"
)

// HookStdin holds the data a Git hook received on its standard input. Git writes it once, for
// example the refs a pre-push hook is about to update, and the first read drains it. Atmos reads it
// up front so every step can see it through a file, and still hands it to the first shell step as
// that step's standard input, which is what a plain Git hook script would have read.
type HookStdin struct {
	mu    sync.Mutex
	data  []byte
	taken bool
}

// NewHookStdin wraps the captured standard input of a Git hook.
func NewHookStdin(data []byte) *HookStdin {
	defer perf.Track(nil, "step.NewHookStdin")()

	return &HookStdin{data: data}
}

// take returns a reader over the captured input the first time it is called, and nil afterwards.
// Concurrent branches share one HookStdin, so only one of them receives the input.
func (h *HookStdin) take() io.Reader {
	if h == nil {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.taken {
		return nil
	}
	h.taken = true
	return bytes.NewReader(h.data)
}
