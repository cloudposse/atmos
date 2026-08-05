package tags

import (
	"github.com/cloudposse/atmos/pkg/perf"
)

// MatchesLabels reports whether labels contains every key=value pair in filterLabels (AND).
// An empty filterLabels always matches (no filter applied).
func MatchesLabels(labels map[string]string, filterLabels map[string]string) bool {
	defer perf.Track(nil, "tags.MatchesLabels")()

	if len(filterLabels) == 0 {
		return true
	}

	for key, wantValue := range filterLabels {
		gotValue, ok := labels[key]
		if !ok || gotValue != wantValue {
			return false
		}
	}

	return true
}
