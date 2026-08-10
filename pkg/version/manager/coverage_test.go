package manager

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/version/resolver"
)

func TestLockPathLoadResolveAndVersionMap(t *testing.T) {
	dir := t.TempDir()
	cfg := &schema.AtmosConfiguration{
		BasePath: dir,
		Version: schema.Version{
			Track:    "prod",
			LockFile: "locks/versions.lock.yaml",
			Tracks: map[string]schema.VersionTrack{
				"prod": {
					Dependencies: map[string]schema.VersionEntry{
						"opentofu": {
							Ecosystem: "toolchain",
							Package:   "opentofu",
							Desired:   "1.10.0",
							Update:    schema.VersionUpdatePolicy{Pin: "sha"},
						},
					},
				},
			},
		},
	}
	lockPath := filepath.Join(dir, "locks", "versions.lock.yaml")
	if got := LockFilePath(cfg); got != lockPath {
		t.Fatalf("LockFilePath = %q, want %q", got, lockPath)
	}
	if got := LockFilePath(&schema.AtmosConfiguration{Version: schema.Version{LockFile: lockPath}}); got != lockPath {
		t.Fatalf("absolute LockFilePath = %q, want %q", got, lockPath)
	}

	lock, err := LoadLock(cfg)
	if err != nil {
		t.Fatalf("LoadLock missing returned error: %v", err)
	}
	if lock.Version != lockVersion || len(lock.Tracks) != 0 {
		t.Fatalf("missing lock = %#v", lock)
	}

	if err := SaveLock(cfg, &LockFile{Tracks: map[string]map[string]LockEntry{
		"prod": {
			"opentofu": {Version: "1.10.0", Digest: "sha256:abc"},
		},
	}}); err != nil {
		t.Fatalf("SaveLock returned error: %v", err)
	}
	if got, err := ResolveLocked(cfg, "", "opentofu"); err != nil || got != "1.10.0" {
		t.Fatalf("ResolveLocked = %q, %v", got, err)
	}
	if _, err := ResolveLocked(cfg, "prod", "missing"); !errors.Is(err, ErrVersionNotLocked) {
		t.Fatalf("missing entry error = %v, want %v", err, ErrVersionNotLocked)
	}
	if _, err := ResolveLocked(cfg, "dev", "opentofu"); !errors.Is(err, ErrVersionNotLocked) {
		t.Fatalf("missing track error = %v, want %v", err, ErrVersionNotLocked)
	}

	versionMap, err := VersionMap(cfg, "prod")
	if err != nil {
		t.Fatalf("VersionMap returned error: %v", err)
	}
	if got := versionMap["opentofu"].String(); got != "sha256:abc" {
		t.Fatalf("VersionMap pinned ref = %q", got)
	}

	noTrackCfg := &schema.AtmosConfiguration{
		BasePath: dir,
		Version:  schema.Version{LockFile: "locks/versions.lock.yaml"},
	}
	versionMap, err = VersionMap(noTrackCfg, "prod")
	if err != nil {
		t.Fatalf("VersionMap with unconfigured track returned error: %v", err)
	}
	if got := versionMap["opentofu"].String(); got != "1.10.0" {
		t.Fatalf("VersionMap unconfigured ref = %q", got)
	}
}

func TestLoadLockDefaultsAndInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	cfg := &schema.AtmosConfiguration{BasePath: dir, Version: schema.Version{LockFile: "versions.lock.yaml"}}
	lockPath := LockFilePath(cfg)

	if err := os.WriteFile(lockPath, []byte("tracks:\n"), 0o644); err != nil {
		t.Fatalf("write lock: %v", err)
	}
	lock, err := LoadLock(cfg)
	if err != nil {
		t.Fatalf("LoadLock defaults returned error: %v", err)
	}
	if lock.Version != lockVersion || lock.Tracks == nil {
		t.Fatalf("defaulted lock = %#v", lock)
	}

	if err := os.WriteFile(lockPath, []byte("tracks: ["), 0o644); err != nil {
		t.Fatalf("write invalid lock: %v", err)
	}
	if _, err := LoadLock(cfg); err == nil {
		t.Fatal("expected invalid YAML error")
	}
}

func TestEffectiveTrackStackTrackNamesExtendsAndGroups(t *testing.T) {
	cfg := &schema.AtmosConfiguration{
		Version: schema.Version{
			Track: "prod",
			Groups: map[string]schema.VersionGroup{
				"excluded": {
					Patterns:        []string{"cloudposse/*"},
					ExcludePatterns: []string{"cloudposse/atmos"},
				},
				"selected": {
					Datasources: []string{"github-tags"},
					Providers:   []string{"github"},
					Patterns:    []string{"cloudposse/*"},
				},
			},
			Tracks: map[string]schema.VersionTrack{
				"base": {
					Dependencies: map[string]schema.VersionEntry{
						"atmos": {
							Ecosystem:  "github",
							Datasource: "github-tags",
							Provider:   "github",
							Package:    "cloudposse/atmos",
							Desired:    "v1.0.0",
						},
					},
				},
				"prod": {
					Extends: "base",
					Dependencies: map[string]schema.VersionEntry{
						"atmos": {Desired: "v1.1.0"},
					},
				},
			},
		},
	}

	stackInfo := &schema.ConfigAndStacksInfo{StackSection: map[string]any{"version": map[string]any{"track": "dev"}}}
	if got := EffectiveTrackFromStack(cfg, stackInfo); got != "dev" {
		t.Fatalf("EffectiveTrackFromStack = %q", got)
	}
	if got := EffectiveTrackFromStack(cfg, &schema.ConfigAndStacksInfo{}); got != "prod" {
		t.Fatalf("EffectiveTrackFromStack fallback = %q", got)
	}
	if got := strings.Join(TrackNames(cfg), ","); got != "base,prod" {
		t.Fatalf("TrackNames = %q", got)
	}

	entries, err := EffectiveEntries(cfg, "prod")
	if err != nil {
		t.Fatalf("EffectiveEntries returned error: %v", err)
	}
	atmos := entries["atmos"]
	if atmos.Desired != "v1.1.0" || atmos.Group != "selected" {
		t.Fatalf("effective atmos entry = %#v", atmos)
	}

	cfg.Version.Tracks["broken"] = schema.VersionTrack{Extends: "missing"}
	if _, err := EffectiveEntries(cfg, "broken"); !errors.Is(err, ErrTrackNotFound) {
		t.Fatalf("missing parent error = %v, want %v", err, ErrTrackNotFound)
	}
}

func TestPolicyHelperEdges(t *testing.T) {
	if _, err := parseCooldown("bad"); !errors.Is(err, ErrInvalidCooldown) {
		t.Fatalf("parseCooldown bad error = %v, want %v", err, ErrInvalidCooldown)
	}
	if _, err := parseCooldown("2x"); !errors.Is(err, ErrInvalidCooldown) {
		t.Fatalf("parseCooldown suffix error = %v, want %v", err, ErrInvalidCooldown)
	}
	if got, err := parseCooldown("2w"); err != nil || got != 14*24*time.Hour {
		t.Fatalf("parseCooldown 2w = %v, %v", got, err)
	}
	if !withinStrategy(StrategyPatch, "not-semver", "v2.0.0") {
		t.Fatal("unparseable locked version should not cap updates")
	}
	if !withinStrategy(StrategyPatch, "v1.2.3", "not-semver") {
		t.Fatal("unparseable candidate should not be capped")
	}
	if withinStrategy(StrategyPatch, "v1.2.3", "v1.3.0") {
		t.Fatal("patch strategy should reject a different minor")
	}
	if !matchesAny([]string{"github"}, "github") || matchesAny([]string{"oci"}, "github") {
		t.Fatal("matchesAny did not match exact values")
	}
	if !matchesPattern([]string{"atmos"}, "cloudposse-atmos", "cloudposse/atmos") {
		t.Fatal("matchesPattern should accept substring matches")
	}
	if got := releasedAtString(&resolver.Candidate{}); got != "unknown" {
		t.Fatalf("releasedAtString unknown = %q", got)
	}
}
