package manager

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestEffectiveEntriesAppliesDefaultsTrackAndGroup(t *testing.T) {
	automerge := true
	atmosConfig := &schema.AtmosConfiguration{
		Version: schema.Version{
			Defaults: schema.VersionPolicy{
				Update: schema.VersionUpdatePolicy{
					Strategy:  "patch",
					Cooldown:  "14d",
					Automerge: &automerge,
				},
				Allow:  []string{"stable"},
				Labels: []string{"dependencies"},
			},
			Groups: map[string]schema.VersionGroup{
				"infrastructure": {
					Ecosystems: []string{"github-actions"},
					Patterns:   []string{"actions/*"},
					Update: schema.VersionUpdatePolicy{
						Strategy: "minor",
					},
					Labels: []string{"infrastructure"},
				},
			},
			Tracks: map[string]schema.VersionTrack{
				"prod": {
					Defaults: schema.VersionPolicy{
						Update: schema.VersionUpdatePolicy{
							Cooldown: "30d",
						},
					},
					Versions: map[string]schema.VersionEntry{
						"checkout": {
							Ecosystem: "github-actions",
							Provider:  "github",
							Package:   "actions/checkout",
							Desired:   "v6",
						},
					},
				},
			},
		},
	}

	entries, err := EffectiveEntries(atmosConfig, "prod")
	if err != nil {
		t.Fatalf("EffectiveEntries returned error: %v", err)
	}
	checkout := entries["checkout"]
	if checkout.Group != "infrastructure" {
		t.Fatalf("expected infrastructure group, got %q", checkout.Group)
	}
	if checkout.Update.Strategy != "minor" {
		t.Fatalf("expected group strategy, got %q", checkout.Update.Strategy)
	}
	if checkout.Update.Cooldown != "30d" {
		t.Fatalf("expected track cooldown, got %q", checkout.Update.Cooldown)
	}
	if checkout.Update.Automerge == nil || !*checkout.Update.Automerge {
		t.Fatalf("expected inherited automerge")
	}
	if len(checkout.Labels) != 2 || checkout.Labels[0] != "dependencies" || checkout.Labels[1] != "infrastructure" {
		t.Fatalf("expected merged labels, got %#v", checkout.Labels)
	}
}

func TestLockTrackAndVersionMap(t *testing.T) {
	tempDir := t.TempDir()
	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Version: schema.Version{
			LockFile: "versions.lock.yaml",
			Tracks: map[string]schema.VersionTrack{
				"prod": {
					Versions: map[string]schema.VersionEntry{
						"checkout": {
							Ecosystem:  "github-actions",
							Datasource: "github-tags",
							Provider:   "github",
							Package:    "actions/checkout",
							Desired:    "v6",
						},
					},
				},
			},
		},
	}

	lock, err := LockTrack(atmosConfig, "prod", "")
	if err != nil {
		t.Fatalf("LockTrack returned error: %v", err)
	}
	if got := lock.Tracks["prod"]["checkout"].Version; got != "v6" {
		t.Fatalf("expected v6 lock, got %q", got)
	}
	if _, err := os.Stat(filepath.Join(tempDir, "versions.lock.yaml")); err != nil {
		t.Fatalf("expected lock file to be written: %v", err)
	}
	versionMap, err := VersionMap(atmosConfig, "prod")
	if err != nil {
		t.Fatalf("VersionMap returned error: %v", err)
	}
	if versionMap["checkout"].Version != "v6" {
		t.Fatalf("expected v6 in version map, got %q", versionMap["checkout"].Version)
	}
}
