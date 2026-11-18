package keystore

import (
	"fmt"
	"sort"
)

// Normalize returns requirements sorted and de-duplicated. It does not validate the terms.
func Normalize(requirements []Requirement) []Requirement {
	seen := make(map[Requirement]struct{}, len(requirements))
	out := make([]Requirement, 0, len(requirements))
	for _, r := range requirements {
		if _, dup := seen[r]; dup {
			continue
		}
		seen[r] = struct{}{}
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// Validate normalizes requirements and returns ErrUnsupportedRequirement, naming the term, when
// the store cannot enforce one of them.
func Validate(store Store, requirements []Requirement) ([]Requirement, error) {
	out := Normalize(requirements)
	for _, r := range out {
		if !store.Supports(r) {
			return nil, fmt.Errorf("%w %q: the %s store cannot enforce it", ErrUnsupportedRequirement, string(r), store.Kind())
		}
	}
	return out, nil
}

// CheckRequirements returns ErrRequirementMismatch unless have and want hold the same terms.
func CheckRequirements(have, want []Requirement) error {
	h, w := Normalize(have), Normalize(want)
	equal := len(h) == len(w)
	for i := 0; equal && i < len(h); i++ {
		equal = h[i] == w[i]
	}
	if !equal {
		return fmt.Errorf("%w: key has %v, want %v", ErrRequirementMismatch, h, w)
	}
	return nil
}
