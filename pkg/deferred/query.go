package deferred

import (
	"regexp"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
)

var staticValueQuery = regexp.MustCompile(`^\.[a-zA-Z_][a-zA-Z0-9_-]*(\.[a-zA-Z_][a-zA-Z0-9_-]*)*$`)

// PathsForQuery narrows simple field queries. Arbitrary YQ expressions
// conservatively evaluate the entire value, including all filter dependencies.
func PathsForQuery(query string) [][]string {
	defer perf.Track(nil, "deferred.PathsForQuery")()

	query = strings.TrimSpace(query)
	if !staticValueQuery.MatchString(query) {
		return nil
	}
	return [][]string{strings.Split(query[1:], ".")}
}
