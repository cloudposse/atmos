package list

import (
	"sort"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	// Table dimensions.
	providerNameWidth    = 15
	providerKindWidth    = 30
	providerRegionWidth  = 12
	providerURLWidth     = 35
	providerDefaultWidth = 7

	identityNameWidth        = 18
	identityKindWidth        = 22
	identityViaProviderWidth = 18
	identityViaIdentityWidth = 18
	identityDefaultWidth     = 7
	identityAliasWidth       = 15

	// Formatting.
	defaultMarker = "✓"
	emptyMarker   = "-"
	maxURLDisplay = 32
	newline       = "\n"

	// Tree colors.
	treeBranchColor = "#555555" // Dark grey for tree branches.
	treeKeyColor    = "#888888" // Medium grey for keys.
	treeValueColor  = "#FFFFFF" // White for values.
)

// getSortedProviderNames returns sorted provider names.
func getSortedProviderNames(providers map[string]schema.Provider) []string {
	names := make([]string, 0, len(providers))
	for name := range providers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// getSortedIdentityNames returns sorted identity names.
func getSortedIdentityNames(identities map[string]schema.Identity) []string {
	names := make([]string, 0, len(identities))
	for name := range identities {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// truncateString truncates a string to the specified length with ellipsis.
func truncateString(s string, maxLen int) string {
	defer perf.Track(nil, "list.truncateString")()

	if len(s) <= maxLen {
		return s
	}
	if maxLen < 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}
