package ci

import (
	"os"
	"sync"
	"sync/atomic"

	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

func (r *reporter) Group(title string) (func(), Receipt, error) {
	defer perf.Track(r.cfg, "ci.Reporter.Group")()

	enabled := func(c *schema.AtmosConfiguration) bool { return resolveGroupMode(c) != GroupModeOff }
	g, rc, ok := routeTo[provider.LogGrouper](r, FeatureGroups, enabled, false, "group")
	if !ok {
		return func() {}, rc, nil
	}
	// CI providers do not support nested groups. Only the outermost group, in this process and across
	// the Atmos processes it spawns, emits provider markers; an inner one renders a plain heading.
	// A local rendering emits no provider markers, so it needs no slot.
	acquired := false
	if !rc.Local {
		if acquired = acquireLogGroup(); !acquired {
			return r.plainGroup(title, rc)
		}
	}
	if err := g.StartLogGroup(title); err != nil {
		if acquired {
			releaseLogGroup()
		}
		return func() {}, rc, err
	}
	var once sync.Once
	end := func() {
		once.Do(func() {
			if acquired {
				defer releaseLogGroup()
			}
			if err := g.EndLogGroup(); err != nil {
				log.Debug("Failed to close CI log group", "title", title, "error", err)
			}
		})
	}
	return end, rc, nil
}

// acquireLogGroup claims the process-wide log group slot. It fails when a group is already open here,
// or a parent Atmos process has one open (the environment sentinel).
func acquireLogGroup() bool {
	if os.Getenv(logGroupSentinelEnvVar) != "" {
		return false
	}
	if atomic.AddInt32(&logGroupDepth, 1) > 1 {
		atomic.AddInt32(&logGroupDepth, -1)
		return false
	}
	return true
}

// releaseLogGroup frees the slot taken by acquireLogGroup.
func releaseLogGroup() {
	atomic.AddInt32(&logGroupDepth, -1)
}

// plainGroup renders a nested group as a plain heading through the local provider, so the title is
// still visible without emitting an unsupported nested provider marker. The receipt reports a local rendering.
func (r *reporter) plainGroup(title string, rc Receipt) (func(), Receipt, error) {
	l := r.local()
	if l == nil {
		return func() {}, rc, nil
	}
	rc = r.localReceipt(l, "")
	g, ok := l.(provider.LogGrouper)
	if !ok {
		return func() {}, rc, nil
	}
	if err := g.StartLogGroup(title); err != nil {
		return func() {}, rc, err
	}
	return func() {}, rc, nil
}
