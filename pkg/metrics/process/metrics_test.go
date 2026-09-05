package process

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestBaseline_SinceReportsElapsedWallTime(t *testing.T) {
	snap := Baseline()
	time.Sleep(20 * time.Millisecond)

	m := snap.Since()

	assert.GreaterOrEqual(t, m.WallTime, 15*time.Millisecond)
}

func TestSnapshot_SinceIsNonNegative(t *testing.T) {
	snap := Baseline()
	m := snap.Since()

	// CPU time deltas must never be negative even on a near-instant diff.
	assert.GreaterOrEqual(t, m.UserCPUTime, time.Duration(0))
	assert.GreaterOrEqual(t, m.SystemCPUTime, time.Duration(0))
	assert.GreaterOrEqual(t, m.WallTime, time.Duration(0))
}

func TestSnapshot_MultipleCallsIndependent(t *testing.T) {
	snap := Baseline()
	first := snap.Since()
	time.Sleep(5 * time.Millisecond)
	second := snap.Since()

	assert.Greater(t, second.WallTime, first.WallTime)
}
