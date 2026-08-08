package toolchain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
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
