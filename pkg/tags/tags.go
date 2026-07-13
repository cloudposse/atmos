// Package tags provides shared tag/label matching primitives used across
// Atmos subsystems (auth identities/providers, components, bulk component
// selection) so every consumer applies the same any/all-match semantics.
package tags

import (
	"sort"

	"github.com/samber/lo"

	"github.com/cloudposse/atmos/pkg/perf"
)

// TagMode defines how multiple filter tags combine when matching against an entity's tags.
type TagMode string

const (
	// TagModeAny matches if the entity has at least one of the filter tags (OR).
	TagModeAny TagMode = "any"
	// TagModeAll matches only if the entity has every filter tag (AND).
	TagModeAll TagMode = "all"
)

// MatchesTags reports whether tags satisfies filterTags under mode.
// An empty filterTags always matches (no filter applied).
func MatchesTags(tags []string, filterTags []string, mode TagMode) bool {
	defer perf.Track(nil, "tags.MatchesTags")()

	if len(filterTags) == 0 {
		return true
	}

	if mode == TagModeAll {
		return len(lo.Without(filterTags, tags...)) == 0
	}

	return len(lo.Intersect(filterTags, tags)) > 0
}

// CollectAvailableTags returns the sorted, deduplicated set of tags across all
// items, using getTags to extract each item's tags. Used to build "available
// tags" hints for no-match errors (e.g. auth identities/providers).
func CollectAvailableTags[T any](items map[string]T, getTags func(T) []string) []string {
	defer perf.Track(nil, "tags.CollectAvailableTags")()

	tagSet := make(map[string]struct{})
	for name := range items {
		for _, tag := range getTags(items[name]) {
			tagSet[tag] = struct{}{}
		}
	}

	availableTags := make([]string, 0, len(tagSet))
	for tag := range tagSet {
		availableTags = append(availableTags, tag)
	}
	sort.Strings(availableTags)
	return availableTags
}
