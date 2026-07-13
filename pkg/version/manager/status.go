package manager

import (
	"context"
	"fmt"
	"time"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Status values reported for managed dependency entries.
const (
	// StatusUnlocked means the entry has no lock file record.
	StatusUnlocked = "unlocked"
	// StatusLocked means the entry is locked but its target could not be resolved.
	StatusLocked = "locked"
	// StatusCurrent means the locked version matches the resolved target.
	StatusCurrent = "current"
	// StatusUpdateAvailable means a policy-eligible update differs from the locked version.
	StatusUpdateAvailable = "update-available"
	// StatusBlocked means a newer version exists but the update policy
	// (strategy or cooldown) holds it back; Message carries the reason.
	StatusBlocked = "newer-available (blocked)"
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

	return StatusTrackWithContext(context.Background(), atmosConfig, track, group)
}

// StatusTrackWithContext returns status for all entries in a track while
// honoring caller cancellation and deadlines for resolver calls.
func StatusTrackWithContext(ctx context.Context, atmosConfig *schema.AtmosConfiguration, track, group string) (*TrackStatus, error) {
	defer perf.Track(atmosConfig, "manager.StatusTrackWithContext")()

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
		status.Entries = append(status.Entries, statusForEntry(ctx, atmosConfig, lockedEntries, &entry))
	}
	return status, nil
}

// statusForEntry computes the status row for a single effective entry.
// Resolved reflects the policy-eligible target, so StatusUpdateAvailable
// means an update the policy would actually take; a newer version held back
// by strategy or cooldown reports StatusBlocked with the reason in Message.
func statusForEntry(ctx context.Context, atmosConfig *schema.AtmosConfiguration, lockedEntries map[string]LockEntry, entry *EffectiveEntry) StatusEntry {
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
	decision, err := decideUpdate(ctx, atmosConfig, entry, row.Locked, time.Now())
	if err != nil {
		row.Message = err.Error()
		return row
	}
	row.Resolved = decision.Target.Version
	switch {
	case row.Locked == "":
		row.Status = StatusUnlocked
	case decision.Target.Version != row.Locked:
		row.Status = StatusUpdateAvailable
	case decision.Raw.Version != row.Locked:
		row.Status = StatusBlocked
		row.Message = decision.Reason
	default:
		row.Status = StatusCurrent
	}
	return row
}

// VerifyTrack checks that all configured entries are locked and satisfy resolvable policy.
func VerifyTrack(atmosConfig *schema.AtmosConfiguration, track string) (*TrackStatus, error) {
	defer perf.Track(atmosConfig, "manager.VerifyTrack")()

	return VerifyTrackWithContext(context.Background(), atmosConfig, track)
}

// VerifyTrackWithContext checks that all configured entries are locked and
// satisfy resolvable policy while honoring caller cancellation and deadlines.
func VerifyTrackWithContext(ctx context.Context, atmosConfig *schema.AtmosConfiguration, track string) (*TrackStatus, error) {
	defer perf.Track(atmosConfig, "manager.VerifyTrackWithContext")()

	status, err := StatusTrackWithContext(ctx, atmosConfig, track, "")
	if err != nil {
		return nil, err
	}
	for i := range status.Entries {
		entry := &status.Entries[i]
		// A policy-blocked newer version is not a verification failure: the
		// locked version is exactly what the policy wants deployed.
		if entry.Status != StatusCurrent && entry.Status != StatusBlocked {
			return status, fmt.Errorf("%w: %s: %s is %s", ErrTrackNotVerified, status.Track, entry.Name, entry.Status)
		}
	}
	return status, nil
}
