package cache

import (
	"fmt"
	"strings"

	"github.com/dustin/go-humanize"

	"github.com/cloudposse/atmos/pkg/http/proxy"
	"github.com/cloudposse/atmos/pkg/ui"
)

// printSavingsReport prints a one-line summary of what the registry cache did this
// run: bytes served from cache (bandwidth saved) and bytes fetched from upstream and
// added to the cache (cache warmed). This per-run, in-memory signal is the only place
// hit/miss statistics surface — there is no persistent hit/miss store. Nothing prints
// when the proxy saw no cacheable traffic (e.g. a repeat run where Terraform's plugin
// cache and the workdir's .terraform served everything without contacting the proxy).
func printSavingsReport(snap proxy.StatsSnapshot) {
	if msg, ok := formatSavingsReport(snap); ok {
		ui.Info(msg)
	}
}

// formatSavingsReport builds the savings-report line and reports whether there is
// anything to print. It returns false when the proxy saw no cacheable traffic.
func formatSavingsReport(snap proxy.StatsSnapshot) (string, bool) {
	var segments []string

	if snap.BytesSaved > 0 {
		segments = append(segments, fmt.Sprintf("%s saved (%d %s)",
			humanize.Bytes(uint64(snap.BytesSaved)), snap.Hits, plural(snap.Hits, "hit", "hits")))
	}

	if snap.BytesCached > 0 {
		segments = append(segments, fmt.Sprintf("%s downloaded and cached (%d %s)",
			humanize.Bytes(uint64(snap.BytesCached)), snap.ObjectsCached, plural(snap.ObjectsCached, "object", "objects")))
	}

	if len(segments) == 0 {
		return "", false
	}

	return "Registry cache: " + strings.Join(segments, "; "), true
}

// plural returns one when n == 1, otherwise many.
func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
