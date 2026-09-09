// Package safenum provides small overflow-safe numeric helpers, primarily
// for sizing make() capacity hints from untrusted or accumulated lengths.
package safenum

import "github.com/cloudposse/atmos/pkg/perf"

// Cap returns a safe make() capacity hint for a+b, clamped to max so the
// addition can never overflow int and the hint never requests an absurd
// up-front allocation. Negative inputs are treated as zero.
//
// The size argument to make() is only a capacity hint -- append() still
// grows the backing array as needed -- so clamping the hint below the true
// a+b never changes program behavior, it only avoids an oversized
// allocation. Callers that must refuse to proceed entirely above some size
// (rather than just allocate conservatively and let append grow it) need
// their own guard; see pkg/merge.processAppendList for that different,
// deliberate semantic.
func Cap(a, b, max int) int {
	defer perf.Track(nil, "safenum.Cap")()

	if a < 0 {
		a = 0
	}
	if b < 0 {
		b = 0
	}
	if max < 0 {
		max = 0
	}
	if a > max || b > max-a {
		return max
	}
	return a + b
}
