package utils

import (
	"github.com/samber/lo"

	"github.com/cloudposse/atmos/pkg/perf"
)

// Coalesce returns the first non-empty argument. Arguments must be comparable.
func Coalesce[T comparable](v ...T) (result T) {
	defer perf.Track(nil, "utils.Coalesce")()

	result, _ = lo.Coalesce(v...)
	return result
}
