package ci

import (
	"os"
	"strings"
	"sync/atomic"

	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

// StartLogGroup opens a provider-backed CI log group and returns a close
// function. It is a no-op when no provider is active or the provider has no log
// grouping capability. Grouping is best-effort: write failures are debug-only
// and must not change command success/failure behavior.
func StartLogGroup(title string) func() {
	defer perf.Track(nil, "ci.StartLogGroup")()

	title = strings.TrimSpace(title)
	if title == "" {
		return func() {}
	}
	if os.Getenv(logGroupSentinelEnvVar) != "" {
		return func() {}
	}
	if atomic.AddInt32(&logGroupDepth, 1) > 1 {
		atomic.AddInt32(&logGroupDepth, -1)
		return func() {}
	}

	p := Detect()
	if p == nil {
		atomic.AddInt32(&logGroupDepth, -1)
		return func() {}
	}
	g, ok := p.(provider.LogGrouper)
	if !ok {
		atomic.AddInt32(&logGroupDepth, -1)
		return func() {}
	}
	if err := g.StartLogGroup(title); err != nil {
		atomic.AddInt32(&logGroupDepth, -1)
		log.Debug("Failed to start CI log group", "provider", p.Name(), "title", title, "error", err)
		return func() {}
	}
	return func() {
		defer atomic.AddInt32(&logGroupDepth, -1)
		if err := g.EndLogGroup(); err != nil {
			log.Debug("Failed to end CI log group", "provider", p.Name(), "title", title, "error", err)
		}
	}
}
