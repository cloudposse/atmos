package toolchain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRunConcurrentBatchWithLiveProgress_ResultsPreserveItemOrder confirms the one guarantee
// that survives the move to live, completion-order printing (matching `atmos toolchain
// install`'s batch-mode convention): the returned results slice always lines up with the
// original items slice by index, regardless of which worker finishes first. The first item is
// made to take the longest and the last item the shortest, so completion order is guaranteed to
// be the reverse of item order -- if results were ever misattributed by completion order instead
// of original index, this would catch it.
func TestRunConcurrentBatchWithLiveProgress_ResultsPreserveItemOrder(t *testing.T) {
	items := []string{"a", "b", "c"}
	delays := map[string]time.Duration{
		"a": 30 * time.Millisecond,
		"b": 15 * time.Millisecond,
		"c": 0,
	}

	results := runConcurrentBatchWithLiveProgress(
		items,
		len(items),
		func(item string) string { return item },
		func(item string) string {
			time.Sleep(delays[item])
			return "done:" + item
		},
		func(result string) (string, batchLineStyle) { return result, batchLineSuccess },
	)

	assert.Equal(t, []string{"done:a", "done:b", "done:c"}, results,
		"results must stay indexed to the original items order even when completion order is reversed")
}

// TestRunConcurrentBatchWithLiveProgress_RunsEveryItem confirms every item is actually
// processed exactly once, including when maxConcurrency is smaller than len(items).
func TestRunConcurrentBatchWithLiveProgress_RunsEveryItem(t *testing.T) {
	items := []string{"a", "b", "c", "d", "e"}

	results := runConcurrentBatchWithLiveProgress(
		items,
		2,
		func(item string) string { return item },
		func(item string) int { return len(item) },
		func(result int) (string, batchLineStyle) { return "ok", batchLineSuccess },
	)

	assert.Equal(t, []int{1, 1, 1, 1, 1}, results)
}

// TestLiveBatchRenderer_StartTickCompleteRenderAndClear drives liveBatchRenderer's methods
// directly (bypassing the isTTY-gated newLiveBatchDisplay constructor, matching install_test.go's
// existing pattern of constructing batchRenderer directly for the same reason), asserting real
// state transitions -- not just that no method panics.
func TestLiveBatchRenderer_StartTickCompleteRenderAndClear(t *testing.T) {
	renderer := newLiveBatchRenderer(2)
	require.NotNil(t, renderer.spinner.Spinner.Frames, "newLiveBatchRenderer must configure a real spinner")

	output := captureUITestOutput(t, func() {
		renderer.start("tool-a")
		renderer.start("tool-b")
	})
	assert.Contains(t, output, "tool-a")
	assert.Contains(t, output, "tool-b")
	assert.Equal(t, []string{"tool-a", "tool-b"}, renderer.active)
	// One spinner line per active item plus one overall progress-bar line.
	assert.Equal(t, 3, renderer.renderedLines, "render must emit a line per active item plus a progress line")

	// clear() with renderedLines > 1 must walk and reset every rendered line, not just the
	// last one.
	captureUITestOutput(t, renderer.clear)
	assert.Equal(t, 0, renderer.renderedLines)

	// clear() called again immediately (renderedLines == 0) must be a no-op early return, not
	// emit stray escape codes.
	earlyReturnOutput := captureUITestOutput(t, renderer.clear)
	assert.Empty(t, earlyReturnOutput)

	captureUITestOutput(t, renderer.tick)

	completeOutput := captureUITestOutput(t, func() {
		renderer.complete("tool-a", "tool-a done", batchLineSuccess)
	})
	assert.Contains(t, completeOutput, "tool-a done")
	assert.Equal(t, []string{"tool-b"}, renderer.active, "completing tool-a must remove only tool-a from active")
	assert.Equal(t, 1, renderer.completed)

	captureUITestOutput(t, func() {
		renderer.complete("tool-b", "tool-b failed", batchLineError)
	})
	assert.Empty(t, renderer.active)
	assert.Equal(t, 2, renderer.completed)
}

// TestLiveBatchRenderer_RenderOmitsProgressLineWhenNothingActive covers render()'s
// len(r.active) == 0 branch: with nothing in flight, only the (zero) per-item lines are drawn,
// no dangling progress-bar line.
func TestLiveBatchRenderer_RenderOmitsProgressLineWhenNothingActive(t *testing.T) {
	renderer := newLiveBatchRenderer(0)

	output := captureUITestOutput(t, renderer.render)

	assert.Empty(t, output)
	assert.Equal(t, 0, renderer.renderedLines)
}

// TestLiveBatchDisplay_WithRenderer_DelegatesEveryMethod constructs a liveBatchDisplay with a
// non-nil renderer directly (bypassing newLiveBatchDisplay's isTTY gate, which can't be forced
// deterministically in a headless test run -- see isTTYForStdoutFunc's doc comment in clean.go
// for the same limitation elsewhere in this package), covering the "renderer != nil" branch of
// every liveBatchDisplay method. The renderer-nil branch is already covered by
// TestRunConcurrentBatchWithLiveProgress_RunsEveryItem and its siblings, which always run
// outside a TTY in CI.
func TestLiveBatchDisplay_WithRenderer_DelegatesEveryMethod(t *testing.T) {
	renderer := newLiveBatchRenderer(1)
	display := &liveBatchDisplay{renderer: renderer}

	output := captureUITestOutput(t, func() {
		display.start("tool")
		display.tick()
		display.complete("tool", "tool done", batchLineSuccess)
		display.clear()
	})

	assert.Contains(t, output, "tool done")
	assert.Equal(t, 1, renderer.completed)
	assert.Empty(t, renderer.active)
	assert.Equal(t, 0, renderer.renderedLines)
}
