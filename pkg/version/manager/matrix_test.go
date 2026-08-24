package manager

import (
	"errors"
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
)

func matrixTestConfig(t *testing.T) *schema.AtmosConfiguration {
	t.Helper()
	dir := t.TempDir()
	cfg := &schema.AtmosConfiguration{
		BasePath: dir,
		Version: schema.Version{
			Dependencies: map[string]schema.VersionEntry{
				"opentofu": {
					Ecosystem: "toolchain",
					Package:   "opentofu",
					Desired:   "1.10.0",
				},
			},
			Tracks: map[string]schema.VersionTrack{
				"dev": {},
				"prod": {
					Dependencies: map[string]schema.VersionEntry{
						"opentofu": {Desired: "1.10.6"},
					},
				},
			},
		},
	}
	lock := &LockFile{
		Tracks: map[string]map[string]LockEntry{
			"prod": {
				"opentofu": {Version: "1.10.6"},
			},
			// dev is intentionally left unlocked for "opentofu".
		},
	}
	if err := SaveLock(cfg, lock); err != nil {
		t.Fatalf("SaveLock: %v", err)
	}
	return cfg
}

func TestTrackVersionMatrixShowLocked(t *testing.T) {
	cfg := matrixTestConfig(t)

	matrix, err := TrackVersionMatrix(cfg, ShowLocked)
	if err != nil {
		t.Fatalf("TrackVersionMatrix: %v", err)
	}
	if got := matrix["prod"]["opentofu"]; got != "1.10.6" {
		t.Fatalf("prod locked opentofu = %q, want 1.10.6", got)
	}
	if got := matrix["dev"]["opentofu"]; got != "" {
		t.Fatalf("dev locked opentofu = %q, want empty (unlocked)", got)
	}
}

func TestTrackVersionMatrixShowDesired(t *testing.T) {
	cfg := matrixTestConfig(t)

	matrix, err := TrackVersionMatrix(cfg, ShowDesired)
	if err != nil {
		t.Fatalf("TrackVersionMatrix: %v", err)
	}
	if got := matrix["prod"]["opentofu"]; got != "1.10.6" {
		t.Fatalf("prod desired opentofu = %q, want 1.10.6", got)
	}
	if got := matrix["dev"]["opentofu"]; got != "1.10.0" {
		t.Fatalf("dev desired opentofu = %q, want 1.10.0 (base catalog)", got)
	}
}

func TestTrackVersionMatrixUnsupportedShow(t *testing.T) {
	cfg := matrixTestConfig(t)

	if _, err := TrackVersionMatrix(cfg, "bogus"); !errors.Is(err, ErrUnsupportedVersionShow) {
		t.Fatalf("error = %v, want %v", err, ErrUnsupportedVersionShow)
	}
}
