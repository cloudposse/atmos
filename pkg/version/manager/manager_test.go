package manager

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestEffectiveEntriesAppliesDefaultsTrackAndGroup(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Version: schema.Version{
			Defaults: schema.VersionPolicy{
				Update: schema.VersionUpdatePolicy{
					Strategy: "patch",
					Cooldown: "14d",
				},
				Include:    []string{"v1.*"},
				Exclude:    []string{"v1.0.0"},
				Prerelease: boolPtr(true),
				Labels:     []string{"dependencies"},
			},
			Groups: map[string]schema.VersionGroup{
				"infrastructure": {
					Ecosystems: []string{"github/actions"},
					Patterns:   []string{"actions/*"},
					Update: schema.VersionUpdatePolicy{
						Strategy: "minor",
					},
					Include:    []string{"v6.*"},
					Exclude:    []string{"v6.0.0"},
					Prerelease: boolPtr(true),
					Labels:     []string{"infrastructure"},
				},
			},
			Tracks: map[string]schema.VersionTrack{
				"prod": {
					Defaults: schema.VersionPolicy{
						Update: schema.VersionUpdatePolicy{
							Cooldown: "30d",
						},
						Exclude: []string{"v1.1.0"},
					},
					Dependencies: map[string]schema.VersionEntry{
						"checkout": {
							Ecosystem:  "github/actions",
							Provider:   "github",
							Package:    "actions/checkout",
							Desired:    "v6",
							Include:    []string{"v5.*"},
							Exclude:    []string{"v5.0.0"},
							Prerelease: boolPtr(false),
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
	if len(checkout.Include) != 1 || checkout.Include[0] != "v5.*" {
		t.Fatalf("expected entry include to win, got %#v", checkout.Include)
	}
	expectedExclude := []string{"v1.0.0", "v1.1.0", "v6.0.0", "v5.0.0"}
	if strings.Join(checkout.Exclude, ",") != strings.Join(expectedExclude, ",") {
		t.Fatalf("expected accumulated excludes %v, got %#v", expectedExclude, checkout.Exclude)
	}
	if checkout.Prerelease {
		t.Fatal("expected entry prerelease=false to override inherited true")
	}
	if len(checkout.Labels) != 2 || checkout.Labels[0] != "dependencies" || checkout.Labels[1] != "infrastructure" {
		t.Fatalf("expected merged labels, got %#v", checkout.Labels)
	}
}

func TestEffectiveEntriesTreatsLegacyGitHubActionsEcosystemAsGitHubActions(t *testing.T) {
	cfg := &schema.AtmosConfiguration{
		Version: schema.Version{
			Groups: map[string]schema.VersionGroup{
				"infrastructure": {
					Ecosystems: []string{"github-actions"},
				},
			},
			Tracks: map[string]schema.VersionTrack{
				"prod": {
					Dependencies: map[string]schema.VersionEntry{
						"checkout": {
							Ecosystem: "github/actions",
							Package:   "actions/checkout",
							Desired:   "v6",
						},
					},
				},
			},
		},
	}

	entries, err := EffectiveEntries(cfg, "prod")
	if err != nil {
		t.Fatalf("EffectiveEntries returned error: %v", err)
	}
	if entries["checkout"].Group != "infrastructure" {
		t.Fatalf("expected legacy github-actions group match, got %q", entries["checkout"].Group)
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
					Dependencies: map[string]schema.VersionEntry{
						"checkout": {
							Ecosystem:  "github/actions",
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

func TestEffectiveEntriesMergesBaseDependenciesWithTrackOverrides(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Version: schema.Version{
			Dependencies: map[string]schema.VersionEntry{
				"atmos": {
					Ecosystem:  "github",
					Datasource: "github-releases",
					Provider:   "github",
					Package:    "cloudposse/atmos",
					Desired:    "~1.200",
				},
			},
			Tracks: map[string]schema.VersionTrack{
				"dev": {
					Dependencies: map[string]schema.VersionEntry{
						"atmos": {
							Desired: "latest",
						},
					},
				},
			},
		},
	}

	entries, err := EffectiveEntries(atmosConfig, "dev")
	if err != nil {
		t.Fatalf("EffectiveEntries returned error: %v", err)
	}
	atmos := entries["atmos"]
	if atmos.Package != "cloudposse/atmos" {
		t.Fatalf("expected package inherited from base catalog, got %q", atmos.Package)
	}
	if atmos.Desired != "latest" {
		t.Fatalf("expected track desired override, got %q", atmos.Desired)
	}
}

func TestEffectiveEntriesUsesBaseCatalogForConfiguredDefaultTrack(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Version: schema.Version{
			Track: "prod",
			Dependencies: map[string]schema.VersionEntry{
				"atmos": {
					Ecosystem: "github",
					Package:   "cloudposse/atmos",
					Desired:   "~1.200",
				},
			},
		},
	}

	entries, err := EffectiveEntries(atmosConfig, "")
	if err != nil {
		t.Fatalf("EffectiveEntries returned error: %v", err)
	}
	if entries["atmos"].Desired != "~1.200" {
		t.Fatalf("expected base catalog dependency, got %#v", entries["atmos"])
	}
	if _, err := EffectiveEntries(atmosConfig, "dev"); err == nil {
		t.Fatal("expected unknown non-default track to fail")
	}
}
