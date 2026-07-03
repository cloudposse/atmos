package manager

import (
	"fmt"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Status values reported for managed version entries.
const (
	// StatusUnlocked means the entry has no lock file record.
	StatusUnlocked = "unlocked"
	// StatusLocked means the entry is locked but its target could not be resolved.
	StatusLocked = "locked"
	// StatusCurrent means the locked version matches the resolved target.
	StatusCurrent = "current"
	// StatusUpdateAvailable means the resolved target differs from the locked version.
	StatusUpdateAvailable = "update-available"
)

// StatusEntry reports the lock/update status for one managed version.
type StatusEntry struct {
	Name       string `yaml:"name" json:"name"`
	Ecosystem  string `yaml:"ecosystem,omitempty" json:"ecosystem,omitempty"`
	Datasource string `yaml:"datasource,omitempty" json:"datasource,omitempty"`
	Provider   string `yaml:"provider,omitempty" json:"provider,omitempty"`
	Package    string `yaml:"package,omitempty" json:"package,omitempty"`
	Desired    string `yaml:"desired,omitempty" json:"desired,omitempty"`
	Locked     string `yaml:"locked,omitempty" json:"locked,omitempty"`
	Resolved   string `yaml:"resolved,omitempty" json:"resolved,omitempty"`
	Group      string `yaml:"group,omitempty" json:"group,omitempty"`
	Status     string `yaml:"status" json:"status"`
	Message    string `yaml:"message,omitempty" json:"message,omitempty"`
}

// TrackStatus is the status payload for a track.
type TrackStatus struct {
	Track   string        `yaml:"track" json:"track"`
	Entries []StatusEntry `yaml:"entries" json:"entries"`
}

// StatusTrack returns status for all entries in a track.
func StatusTrack(atmosConfig *schema.AtmosConfiguration, track, group string) (*TrackStatus, error) {
	defer perf.Track(atmosConfig, "manager.StatusTrack")()

	track = EffectiveTrack(atmosConfig, track)
	entries, err := EffectiveEntries(atmosConfig, track)
	if err != nil {
		return nil, err
	}
	lock, err := LoadLock(atmosConfig)
	if err != nil {
		return nil, err
	}
	status := &TrackStatus{Track: track}
	lockedEntries := lock.Tracks[track]
	for _, name := range sortedNames(entries) {
		entry := entries[name]
		if group != "" && entry.Group != group {
			continue
		}
		status.Entries = append(status.Entries, statusForEntry(atmosConfig, lockedEntries, &entry))
	}
	return status, nil
}

// statusForEntry computes the status row for a single effective entry.
func statusForEntry(atmosConfig *schema.AtmosConfiguration, lockedEntries map[string]LockEntry, entry *EffectiveEntry) StatusEntry {
	row := StatusEntry{
		Name:       entry.Name,
		Ecosystem:  entry.Ecosystem,
		Datasource: entry.Datasource,
		Provider:   entry.Provider,
		Package:    entry.Package,
		Desired:    entry.Desired,
		Group:      entry.Group,
		Status:     StatusUnlocked,
	}
	if locked, ok := lockedEntries[entry.Name]; ok {
		row.Locked = locked.Version
		row.Status = StatusLocked
	}
	resolved, err := ResolveTarget(atmosConfig, entry)
	if err != nil {
		row.Message = err.Error()
		return row
	}
	row.Resolved = resolved
	switch row.Locked {
	case "":
		row.Status = StatusUnlocked
	case resolved:
		row.Status = StatusCurrent
	default:
		row.Status = StatusUpdateAvailable
	}
	return row
}

// VerifyTrack checks that all configured entries are locked and satisfy resolvable policy.
func VerifyTrack(atmosConfig *schema.AtmosConfiguration, track string) (*TrackStatus, error) {
	defer perf.Track(atmosConfig, "manager.VerifyTrack")()

	status, err := StatusTrack(atmosConfig, track, "")
	if err != nil {
		return nil, err
	}
	for i := range status.Entries {
		entry := &status.Entries[i]
		if entry.Status != StatusCurrent && entry.Status != StatusLocked {
			return status, fmt.Errorf("%w: %s: %s is %s", ErrTrackNotVerified, status.Track, entry.Name, entry.Status)
		}
	}
	return status, nil
}
