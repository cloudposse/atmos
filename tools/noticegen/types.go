// Package main generates the repository's NOTICE file from Go dependency
// licenses, replacing the previous scripts/generate-notice.sh. It wraps
// google/go-licenses (installed on demand) for license discovery, applies a
// small table of deterministic URL overrides for modules go-licenses can't
// resolve reliably, and renders the result in the same format as the
// hand-written NOTICE file this repository has always shipped.
package main

// LicenseEntry is one row of a go-licenses report: a Go module, the URL to
// its license text, and the SPDX-ish license identifier go-licenses
// detected for it.
type LicenseEntry struct {
	Module  string
	URL     string
	License string
}

// Summary reports how many dependencies fell into each license family
// rendered in NOTICE, plus the total number of dependencies scanned
// (including families NOTICE doesn't render a section for).
type Summary struct {
	Total  int
	Apache int
	BSD    int
	MPL    int
	MIT    int
}
