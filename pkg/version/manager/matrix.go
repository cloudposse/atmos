package manager

import (
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Show selectors for TrackVersionMatrix: which version value populates each
// matrix cell.
const (
	ShowDesired = "desired"
	ShowLocked  = "locked"
)

// ErrUnsupportedVersionShow is returned when TrackVersionMatrix is called with
// an unrecognized show value.
var ErrUnsupportedVersionShow = errUtils.ErrUnsupportedVersionShow

// TrackVersionMatrix returns configured version tracks as a matrix keyed by
// track name, then dependency name, with the requested show value as each
// cell's value: ShowDesired reads the configured version straight from
// EffectiveEntries (no lock file access); ShowLocked reads the resolved
// version from versions.lock.yaml, leaving the cell empty when a dependency
// is not yet locked for that track.
func TrackVersionMatrix(atmosConfig *schema.AtmosConfiguration, show string) (map[string]map[string]string, error) {
	defer perf.Track(atmosConfig, "manager.TrackVersionMatrix")()

	if show != ShowDesired && show != ShowLocked {
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedVersionShow, show)
	}

	var lock *LockFile
	if show == ShowLocked {
		loaded, err := LoadLock(atmosConfig)
		if err != nil {
			return nil, err
		}
		lock = loaded
	}

	tracks := TrackNames(atmosConfig)
	matrix := make(map[string]map[string]string, len(tracks))
	for _, track := range tracks {
		entries, err := EffectiveEntries(atmosConfig, track)
		if err != nil {
			return nil, err
		}
		matrix[track] = trackShowValues(entries, lock, track, show)
	}
	return matrix, nil
}

// trackShowValues builds one track's row of the matrix from its effective
// entries, using either the desired version or the locked version per show.
func trackShowValues(entries map[string]EffectiveEntry, lock *LockFile, track, show string) map[string]string {
	row := make(map[string]string, len(entries))
	for name := range entries {
		switch show {
		case ShowDesired:
			row[name] = entries[name].Desired
		case ShowLocked:
			if lock != nil {
				if locked, ok := lock.Tracks[track][name]; ok {
					row[name] = locked.Version
				}
			}
		}
	}
	return row
}
