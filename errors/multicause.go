package errors

import (
	"github.com/cockroachdb/errors"
)

// AllHints returns every hint attached to err and to everything it wraps, in encounter order
// and without duplicates.
//
// It behaves like cockroachdb/errors GetAllHints but also descends into multi-cause errors
// (Unwrap() []error), such as those produced by errors.Join or fmt.Errorf with more than one %w.
// GetAllHints alone treats such an error as a leaf, so any hint attached beneath it is silently
// lost once an intermediate layer wraps the cause with a second %w. Use this instead of
// GetAllHints wherever hints are rendered.
func AllHints(err error) []string {
	return collectAcrossCauses(err, errors.GetAllHints)
}

// AllDetails returns every detail (explanation) attached to err and to everything it wraps, in
// encounter order and without duplicates. Like AllHints, it descends into multi-cause errors.
func AllDetails(err error) []string {
	return collectAcrossCauses(err, errors.GetAllDetails)
}

// collectAcrossCauses gathers strings with collect along the single-cause chain of err, then
// recurses into the causes of every multi-cause error found along that chain.
func collectAcrossCauses(err error, collect func(error) []string) []string {
	if err == nil {
		return nil
	}
	var out []string
	seen := make(map[string]struct{})
	gatherAcrossCauses(err, collect, seen, &out)
	return out
}

func gatherAcrossCauses(err error, collect func(error) []string, seen map[string]struct{}, out *[]string) {
	for _, s := range collect(err) {
		if _, dup := seen[s]; dup {
			continue
		}
		seen[s] = struct{}{}
		*out = append(*out, s)
	}
	for current := err; current != nil; current = errors.UnwrapOnce(current) {
		multi, ok := current.(interface{ Unwrap() []error })
		if !ok {
			continue
		}
		for _, cause := range multi.Unwrap() {
			gatherAcrossCauses(cause, collect, seen, out)
		}
	}
}
