package main

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderFullEntrySet(t *testing.T) {
	entries := []LicenseEntry{
		{Module: "github.com/zzz/apache-two", URL: "https://example.com/apache-two/LICENSE", License: "Apache-2.0"},
		{Module: "github.com/aaa/apache-one", URL: "https://example.com/apache-one/LICENSE", License: "Apache-2.0"},
		{Module: "github.com/bsd/three", URL: "https://example.com/bsd-three/LICENSE", License: "BSD-3-Clause"},
		{Module: "github.com/bsd/two", URL: "https://example.com/bsd-two/LICENSE", License: "BSD-2-Clause"},
		{Module: "github.com/mpl/one", URL: "https://example.com/mpl-one/LICENSE", License: "MPL-2.0"},
		{Module: "github.com/mit/one", URL: "https://example.com/mit-one/LICENSE", License: "MIT"},
		{Module: "github.com/other/isc", URL: "https://example.com/isc/LICENSE", License: "ISC"},
	}

	got := Render(entries, "Atmos - Universal Tool for DevOps and Cloud Automation")

	want := fmt.Sprintf(`NOTICE

Atmos - Universal Tool for DevOps and Cloud Automation
Copyright 2021-%d Cloud Posse, LLC

This product includes software developed by Cloud Posse, LLC and the Atmos community.

================================================================================

This product bundles the following dependencies under their respective licenses.
The license information for each dependency can be found below.

For the full license texts, see the LICENSE file in each dependency or visit
the URLs listed below.

================================================================================

APACHE 2.0 LICENSED DEPENDENCIES

  - github.com/aaa/apache-one
    License: Apache-2.0
    URL: https://example.com/apache-one/LICENSE

  - github.com/zzz/apache-two
    License: Apache-2.0
    URL: https://example.com/apache-two/LICENSE


================================================================================

BSD LICENSED DEPENDENCIES

  - github.com/bsd/three
    License: BSD-3-Clause
    URL: https://example.com/bsd-three/LICENSE

  - github.com/bsd/two
    License: BSD-2-Clause
    URL: https://example.com/bsd-two/LICENSE


================================================================================

MOZILLA PUBLIC LICENSE (MPL) 2.0 DEPENDENCIES

  - github.com/mpl/one
    License: MPL-2.0
    URL: https://example.com/mpl-one/LICENSE


================================================================================

MIT LICENSED DEPENDENCIES

  - github.com/mit/one
    License: MIT
    URL: https://example.com/mit-one/LICENSE


================================================================================

For the complete list of dependencies and their licenses, run:
  go-licenses report ./...

To view the full license text for a specific dependency, visit the URL
listed above or check the dependency's repository.

For more information about Atmos licensing, see:
  https://github.com/cloudposse/atmos
`, time.Now().Year())

	assert.Equal(t, want, got)
}

// TestRenderOmitsEmptyOptionalSections covers the MPL/MIT section-omission
// rule: with no MPL or MIT entries, those sections (including their
// separators) must not appear at all, while Apache/BSD sections remain
// present even when empty.
func TestRenderOmitsEmptyOptionalSections(t *testing.T) {
	entries := []LicenseEntry{
		{Module: "github.com/apache/only", URL: "https://example.com/apache/LICENSE", License: "Apache-2.0"},
	}

	got := Render(entries, "A test tagline.")

	assert.Contains(t, got, "APACHE 2.0 LICENSED DEPENDENCIES")
	assert.Contains(t, got, "BSD LICENSED DEPENDENCIES")
	assert.NotContains(t, got, "MOZILLA PUBLIC LICENSE")
	assert.NotContains(t, got, "MIT LICENSED DEPENDENCIES")

	// The BSD section header must still appear even with zero entries.
	bsdIdx := strings.Index(got, "BSD LICENSED DEPENDENCIES")
	require.GreaterOrEqual(t, bsdIdx, 0)
	afterBSD := got[bsdIdx:]
	assert.True(t, strings.HasPrefix(afterBSD, "BSD LICENSED DEPENDENCIES\n\n\n================================================================================\n"),
		"expected the footer separator to immediately follow the empty BSD section, got:\n%s", afterBSD)
}

func TestRenderEmptyEntries(t *testing.T) {
	got := Render(nil, "A test tagline.")
	assert.Contains(t, got, "APACHE 2.0 LICENSED DEPENDENCIES")
	assert.Contains(t, got, "BSD LICENSED DEPENDENCIES")
	assert.NotContains(t, got, "MOZILLA PUBLIC LICENSE")
	assert.NotContains(t, got, "MIT LICENSED DEPENDENCIES")
	assert.True(t, strings.HasSuffix(got, "  https://github.com/cloudposse/atmos\n"))
}

func TestIsApache(t *testing.T) {
	assert.True(t, isApache(LicenseEntry{License: "Apache-2.0"}))
	assert.False(t, isApache(LicenseEntry{License: "Apache-2.0-with-exception"}))
	assert.False(t, isApache(LicenseEntry{License: "MIT"}))
}

func TestIsBSD(t *testing.T) {
	cases := []struct {
		license string
		want    bool
	}{
		{"BSD-2-Clause", true},
		{"BSD-2-Clause-FreeBSD", true},
		{"BSD-3-Clause", true},
		{"MIT", false},
		{"Apache-2.0", false},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, isBSD(LicenseEntry{License: tc.license}), "isBSD(%q)", tc.license)
	}
}

func TestIsMPL(t *testing.T) {
	assert.True(t, isMPL(LicenseEntry{License: "MPL-2.0"}))
	assert.False(t, isMPL(LicenseEntry{License: "MPL-1.1"}))
}

func TestIsMIT(t *testing.T) {
	assert.True(t, isMIT(LicenseEntry{License: "MIT"}))
	assert.False(t, isMIT(LicenseEntry{License: "MIT-0"}))
}

func TestFilterSortedIsByteWiseByModule(t *testing.T) {
	entries := []LicenseEntry{
		{Module: "cloud.google.com/go", License: "Apache-2.0"},
		{Module: "cel.dev/expr", License: "Apache-2.0"},
		{Module: "cloud.google.com/go/iam", License: "Apache-2.0"},
	}

	got := filterSorted(entries, isApache)

	require.Len(t, got, 3)
	assert.Equal(t, "cel.dev/expr", got[0].Module)
	assert.Equal(t, "cloud.google.com/go", got[1].Module)
	assert.Equal(t, "cloud.google.com/go/iam", got[2].Module)
}

func TestSummarize(t *testing.T) {
	entries := []LicenseEntry{
		{Module: "a", License: "Apache-2.0"},
		{Module: "b", License: "BSD-3-Clause"},
		{Module: "c", License: "MPL-2.0"},
		{Module: "d", License: "MIT"},
		{Module: "e", License: "ISC"},
	}

	got := summarize(entries)

	assert.Equal(t, Summary{Total: 5, Apache: 1, BSD: 1, MPL: 1, MIT: 1}, got)
}
