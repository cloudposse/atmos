package manager

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/version/resolver"
)

// UpdateResult reports the outcome of one entry's policy-driven update.
type UpdateResult struct {
	Name       string `yaml:"name" json:"name"`
	From       string `yaml:"from,omitempty" json:"from,omitempty"`
	To         string `yaml:"to,omitempty" json:"to,omitempty"`
	FromDigest string `yaml:"from_digest,omitempty" json:"from_digest,omitempty"`
	ToDigest   string `yaml:"to_digest,omitempty" json:"to_digest,omitempty"`
	Updated    bool   `yaml:"updated" json:"updated"`
	// Reason explains a held-back or unchanged outcome (e.g. a newer version
	// blocked by strategy or cooldown).
	Reason string `yaml:"reason,omitempty" json:"reason,omitempty"`
}

// TrackUpdate is the payload of a policy-driven track update.
type TrackUpdate struct {
	Track   string         `yaml:"track" json:"track"`
	Results []UpdateResult `yaml:"results" json:"results"`
}

// UpdateTrack advances locked versions within each entry's effective update
// policy (strategy caps relative to the locked version, cooldown against the
// upstream release timestamp, include/exclude/prerelease rules) and writes the lock file.
// Unlike LockTrack, which resolves the desired expression as-is, UpdateTrack
// starts from the locked state and records a structured reason whenever a
// newer candidate is held back. The only filter limits the update to the
// named entries.
func UpdateTrack(atmosConfig *schema.AtmosConfiguration, track, group string, only []string) (*TrackUpdate, error) {
	defer perf.Track(atmosConfig, "manager.UpdateTrack")()

	return UpdateTrackWithContext(context.Background(), atmosConfig, track, group, only)
}

// UpdateTrackWithContext advances locked versions while honoring caller
// cancellation and deadlines for resolver calls.
func UpdateTrackWithContext(ctx context.Context, atmosConfig *schema.AtmosConfiguration, track, group string, only []string) (*TrackUpdate, error) {
	defer perf.Track(atmosConfig, "manager.UpdateTrackWithContext")()

	track = EffectiveTrack(atmosConfig, track)
	entries, err := EffectiveEntries(atmosConfig, track)
	if err != nil {
		return nil, err
	}
	lock, err := LoadLock(atmosConfig)
	if err != nil {
		return nil, err
	}
	if lock.Tracks[track] == nil {
		lock.Tracks[track] = map[string]LockEntry{}
	}

	update := &TrackUpdate{Track: track}
	now := time.Now()
	for _, name := range sortedNames(entries) {
		entry := entries[name]
		if group != "" && entry.Group != group {
			continue
		}
		if len(only) > 0 && !slices.Contains(only, name) {
			continue
		}
		locked := lock.Tracks[track][name]
		result, lockEntry, err := updateEntry(ctx, atmosConfig, &entry, &locked, now)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		lock.Tracks[track][name] = lockEntry
		update.Results = append(update.Results, result)
	}
	return update, SaveLock(atmosConfig, lock)
}

// updateEntry computes the policy-driven update for a single entry and its
// new lock entry.
func updateEntry(ctx context.Context, atmosConfig *schema.AtmosConfiguration, entry *EffectiveEntry, locked *LockEntry, now time.Time) (UpdateResult, LockEntry, error) {
	decision, err := decideUpdate(ctx, atmosConfig, entry, locked.Version, now)
	if err != nil {
		return UpdateResult{}, LockEntry{}, err
	}
	target := decision.Target

	// Acquire the digest for pinned entries. An unchanged version re-pins on
	// purpose: digest refresh catches re-tagged upstream releases, which is
	// the whole point of `strategy: pin` with `pin: digest`.
	if pinEnabled(entry) && target.Version != "" && (target.Digest == "" || target.Version == locked.Version) {
		digest, err := pinDigest(ctx, atmosConfig, entry, target.Version)
		if err != nil {
			return UpdateResult{}, LockEntry{}, err
		}
		target.Digest = digest
	}

	result := UpdateResult{
		Name:       entry.Name,
		From:       locked.Version,
		To:         target.Version,
		FromDigest: locked.Digest,
		ToDigest:   target.Digest,
		Updated:    target.Version != locked.Version || target.Digest != locked.Digest,
		Reason:     decision.Reason,
	}
	lockEntry := LockEntry{
		Version:    target.Version,
		Ecosystem:  entry.Ecosystem,
		Datasource: entry.Datasource,
		Provider:   entry.Provider,
		Package:    entry.Package,
		Digest:     target.Digest,
		ResolvedAt: now.UTC().Format(time.RFC3339),
	}
	if target.ReleasedAt != nil {
		lockEntry.ReleasedAt = target.ReleasedAt.UTC().Format(time.RFC3339)
	} else if target.Version == locked.Version {
		lockEntry.ReleasedAt = locked.ReleasedAt
	}
	return result, lockEntry, nil
}

// pinDigest resolves the immutable digest for a version via the entry's
// datasource.
func pinDigest(ctx context.Context, atmosConfig *schema.AtmosConfiguration, entry *EffectiveEntry, version string) (string, error) {
	res, datasource, ok := resolver.Lookup(entry.Datasource)
	if !ok {
		return "", fmt.Errorf("%w: datasource %q cannot pin digests", ErrResolverUnsupported, entry.Datasource)
	}
	ctx, cancel := resolverContext(ctx)
	defer cancel()
	return res.Pin(ctx, resolverRequest(atmosConfig, entry, datasource), version)
}
