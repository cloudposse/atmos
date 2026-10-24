package main

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

// bsdLicenseRe matches any BSD family identifier go-licenses emits
// (BSD-2-Clause, BSD-2-Clause-FreeBSD, BSD-3-Clause, ...).
var bsdLicenseRe = regexp.MustCompile(`BSD-.*Clause`)

func isApache(e LicenseEntry) bool { return e.License == "Apache-2.0" }
func isBSD(e LicenseEntry) bool    { return bsdLicenseRe.MatchString(e.License) }
func isMPL(e LicenseEntry) bool    { return e.License == "MPL-2.0" }
func isMIT(e LicenseEntry) bool    { return e.License == "MIT" }

// buildHeader renders everything in NOTICE that precedes the first rendered
// section, given description (the repository's live GitHub description --
// see fetchRepoDescription). It intentionally stops short of the separator
// before "APACHE 2.0 LICENSED DEPENDENCIES" -- writeSection supplies that
// separator so every section (including the first) is generated the same
// way. The copyright end year is today's year: 2021 is the repo's real
// founding year and stays a literal, but a hardcoded end year goes stale
// the moment the calendar turns.
func buildHeader(description string) string {
	return fmt.Sprintf(`NOTICE

%s
Copyright 2021-%d Cloud Posse, LLC

This product includes software developed by Cloud Posse, LLC and the Atmos community.

================================================================================

This product bundles the following dependencies under their respective licenses.
The license information for each dependency can be found below.

For the full license texts, see the LICENSE file in each dependency or visit
the URLs listed below.
`, description, time.Now().Year())
}

// footer is appended after the last rendered section. Its leading blank
// line plays the same role as writeSection's leading separator.
const footer = `
================================================================================

For the complete list of dependencies and their licenses, run:
  go-licenses report ./...

To view the full license text for a specific dependency, visit the URL
listed above or check the dependency's repository.

For more information about Atmos licensing, see:
  https://github.com/cloudposse/atmos
`

// Render produces the full NOTICE file contents for entries. Apache-2.0 and
// BSD sections are always present (even if empty); MPL-2.0 and MIT sections
// are omitted entirely when there are no matching entries, matching the
// previous shell script's behavior. Entries in license families other than
// these four (e.g. ISC, ...) are scanned for Summary's Total count but are
// not rendered in any section -- also matching prior behavior.
func Render(entries []LicenseEntry, description string) string {
	var b strings.Builder
	b.WriteString(buildHeader(description))

	writeSection(&b, "APACHE 2.0 LICENSED DEPENDENCIES", filterSorted(entries, isApache), apacheLabel)
	writeSection(&b, "BSD LICENSED DEPENDENCIES", filterSorted(entries, isBSD), bsdLabel)

	if mpl := filterSorted(entries, isMPL); len(mpl) > 0 {
		writeSection(&b, "MOZILLA PUBLIC LICENSE (MPL) 2.0 DEPENDENCIES", mpl, mplLabel)
	}
	if mit := filterSorted(entries, isMIT); len(mit) > 0 {
		writeSection(&b, "MIT LICENSED DEPENDENCIES", mit, mitLabel)
	}

	b.WriteString(footer)
	return b.String()
}

func apacheLabel(LicenseEntry) string { return "Apache-2.0" }
func bsdLabel(e LicenseEntry) string  { return e.License }
func mplLabel(LicenseEntry) string    { return "MPL-2.0" }
func mitLabel(LicenseEntry) string    { return "MIT" }

func writeSection(b *strings.Builder, title string, entries []LicenseEntry, licenseLabel func(LicenseEntry) string) {
	b.WriteString("\n================================================================================\n\n")
	b.WriteString(title)
	b.WriteString("\n\n")
	for _, e := range entries {
		fmt.Fprintf(b, "  - %s\n    License: %s\n    URL: %s\n\n", e.Module, licenseLabel(e), e.URL)
	}
}

// filterSorted returns the entries matching match, sorted by module path
// (byte-wise, matching the previous script's `sort` on Linux CI).
func filterSorted(entries []LicenseEntry, match func(LicenseEntry) bool) []LicenseEntry {
	var out []LicenseEntry
	for _, e := range entries {
		if match(e) {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Module < out[j].Module })
	return out
}

// summarize computes per-family counts for the CLI summary output.
func summarize(entries []LicenseEntry) Summary {
	return Summary{
		Total:  len(entries),
		Apache: len(filterSorted(entries, isApache)),
		BSD:    len(filterSorted(entries, isBSD)),
		MPL:    len(filterSorted(entries, isMPL)),
		MIT:    len(filterSorted(entries, isMIT)),
	}
}
