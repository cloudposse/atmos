package milestone

import (
	"fmt"
	"math"
)

// MilestoneChange is a proposed minor-milestone mutation. A zero Number creates
// a milestone; a positive Number reopens that existing, unpublished milestone.
type MilestoneChange struct {
	Number int
	Title  string
}

// MinorMilestoneWindow maintains exactly the next three minor slots after the
// latest stable version. It never creates patch/major versions or closes/deletes
// any manually maintained milestones (including farther-future minor versions).
func MinorMilestoneWindow(baseline string, milestones []Milestone) ([]MilestoneChange, error) {
	v, err := MilestoneVersion(baseline)
	if err != nil {
		return nil, err
	}
	if v.Minor() > math.MaxUint64-3 {
		return nil, fmt.Errorf("%w: minor version overflow", ErrMilestonePolicy)
	}
	existing := map[string]Milestone{}
	for _, m := range milestones {
		parsed, parseErr := MilestoneVersion(m.Title)
		if parseErr != nil {
			continue
		}
		key := parsed.String()
		if prior, ok := existing[key]; ok {
			return nil, fmt.Errorf("%w: duplicate version %s in milestones #%d and #%d", ErrMilestonePolicy, key, prior.Number, m.Number)
		}
		existing[key] = m
	}
	var changes []MilestoneChange
	for offset := uint64(1); offset <= 3; offset++ {
		version := fmt.Sprintf("%d.%d.0", v.Major(), v.Minor()+offset)
		m, exists := existing[version]
		if exists {
			if m.State == "closed" {
				changes = append(changes, MilestoneChange{m.Number, m.Title})
			}
			continue
		}
		changes = append(changes, MilestoneChange{Title: "v" + version})
	}
	return changes, nil
}
