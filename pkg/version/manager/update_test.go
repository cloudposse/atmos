package manager

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/version/resolver"
)

// policyFakeResolver serves the "fake-policy" datasource with time-stamped
// candidates for policy tests.
type policyFakeResolver struct{}

type policyErrorResolver struct{}

func (policyFakeResolver) Names() []string { return []string{"fake-policy"} }

func (policyFakeResolver) Versions(ctx context.Context, req *resolver.Request) ([]resolver.Candidate, error) {
	old := time.Now().Add(-52 * daysPerWeek * hoursPerDay * time.Hour)
	fresh := time.Now().Add(-2 * hoursPerDay * time.Hour)
	return []resolver.Candidate{
		{Version: "v1.9.0", ReleasedAt: &old},
		{Version: "v1.10.2", ReleasedAt: &old, Digest: "sha-1102"},
		{Version: "v1.11.0", ReleasedAt: &fresh, Digest: "sha-1110"},
		{Version: "v2.0.0", ReleasedAt: &old, Digest: "sha-2000"},
	}, nil
}

func (policyFakeResolver) Pin(ctx context.Context, req *resolver.Request, version string) (string, error) {
	return "pinned-" + version, nil
}

func (policyErrorResolver) Names() []string { return []string{"fake-policy-error"} }

func (policyErrorResolver) Versions(ctx context.Context, req *resolver.Request) ([]resolver.Candidate, error) {
	return nil, errors.New("resolver failed")
}

func (policyErrorResolver) Pin(ctx context.Context, req *resolver.Request, version string) (string, error) {
	return "", errors.New("resolver failed")
}

func init() {
	resolver.Register(policyFakeResolver{})
	resolver.Register(policyErrorResolver{})
}

// policyConfig builds a config with one fake-policy entry using the given policy.
func policyConfig(t *testing.T, update *schema.VersionUpdatePolicy) *schema.AtmosConfiguration {
	t.Helper()
	return &schema.AtmosConfiguration{
		BasePath: t.TempDir(),
		Version: schema.Version{
			Tracks: map[string]schema.VersionTrack{
				"prod": {
					Dependencies: map[string]schema.VersionEntry{
						"thing": {
							Datasource: "fake-policy",
							Package:    "acme/thing",
							Desired:    "latest",
							Update:     *update,
						},
					},
				},
			},
		},
	}
}

func TestParseCooldown(t *testing.T) {
	cases := map[string]time.Duration{
		"":    0,
		"14d": 14 * 24 * time.Hour,
		"2w":  14 * 24 * time.Hour,
		"36h": 36 * time.Hour,
	}
	for input, expected := range cases {
		got, err := parseCooldown(input)
		if err != nil {
			t.Errorf("parseCooldown(%q) returned error: %v", input, err)
		}
		if got != expected {
			t.Errorf("parseCooldown(%q) = %v, expected %v", input, got, expected)
		}
	}
	if _, err := parseCooldown("fortnight"); err == nil {
		t.Error("expected error for invalid cooldown")
	}
}

func TestWithinStrategy(t *testing.T) {
	cases := []struct {
		strategy, locked, candidate string
		expected                    bool
	}{
		{"", "v1.9.0", "v2.0.0", true},
		{StrategyMajor, "v1.9.0", "v2.0.0", true},
		{StrategyMinor, "v1.9.0", "v1.10.2", true},
		{StrategyMinor, "v1.9.0", "v2.0.0", false},
		{StrategyPatch, "v1.9.0", "v1.9.9", true},
		{StrategyPatch, "v1.9.0", "v1.10.0", false},
		{StrategyPin, "v1.9.0", "v1.9.0", true},
		{StrategyPin, "v1.9.0", "v1.9.1", false},
		// Unparseable locked version places no cap (initial lock).
		{StrategyPatch, "", "v9.9.9", true},
	}
	for _, testCase := range cases {
		got := withinStrategy(testCase.strategy, testCase.locked, testCase.candidate)
		if got != testCase.expected {
			t.Errorf("withinStrategy(%q, %q, %q) = %v, expected %v",
				testCase.strategy, testCase.locked, testCase.candidate, got, testCase.expected)
		}
	}
}

// seedLock writes an initial lock entry so updates advance from a known state.
func seedLock(t *testing.T, atmosConfig *schema.AtmosConfiguration, version string) {
	t.Helper()
	lock := emptyLock()
	lock.Tracks["prod"] = map[string]LockEntry{
		"thing": {Version: version, Datasource: "fake-policy", Package: "acme/thing"},
	}
	if err := SaveLock(atmosConfig, lock); err != nil {
		t.Fatalf("seeding lock: %v", err)
	}
}

func TestUpdateTrackStrategyMinorHoldsBackMajor(t *testing.T) {
	atmosConfig := policyConfig(t, &schema.VersionUpdatePolicy{Strategy: StrategyMinor})
	seedLock(t, atmosConfig, "v1.9.0")

	update, err := UpdateTrack(atmosConfig, "prod", "", nil)
	if err != nil {
		t.Fatalf("UpdateTrack returned error: %v", err)
	}
	result := update.Results[0]
	if result.To != "v1.11.0" {
		t.Fatalf("expected advance to v1.11.0 within minor, got %q", result.To)
	}
	if !result.Updated {
		t.Fatal("expected Updated=true")
	}
	if !strings.Contains(result.Reason, "strategy minor holds back v2.0.0") {
		t.Fatalf("expected strategy block reason for v2.0.0, got %q", result.Reason)
	}
}

func TestUpdateTrackCooldownHoldsBackFreshRelease(t *testing.T) {
	atmosConfig := policyConfig(t, &schema.VersionUpdatePolicy{Strategy: StrategyMinor, Cooldown: "14d"})
	seedLock(t, atmosConfig, "v1.9.0")

	update, err := UpdateTrack(atmosConfig, "prod", "", nil)
	if err != nil {
		t.Fatalf("UpdateTrack returned error: %v", err)
	}
	result := update.Results[0]
	if result.To != "v1.10.2" {
		t.Fatalf("expected cooldown to hold v1.11.0 and take v1.10.2, got %q", result.To)
	}
	if !strings.Contains(result.Reason, "holds back v1.11.0") {
		t.Fatalf("expected cooldown block reason, got %q", result.Reason)
	}
}

func TestUpdateTrackCooldownInitialLockWritesVersion(t *testing.T) {
	atmosConfig := policyConfig(t, &schema.VersionUpdatePolicy{Strategy: StrategyMinor, Cooldown: "14d"})

	update, err := UpdateTrack(atmosConfig, "prod", "", nil)
	if err != nil {
		t.Fatalf("UpdateTrack returned error: %v", err)
	}
	result := update.Results[0]
	if result.To == "" {
		t.Fatal("expected initial update to write a concrete version")
	}
	lock, err := LoadLock(atmosConfig)
	if err != nil {
		t.Fatalf("LoadLock returned error: %v", err)
	}
	if lock.Tracks["prod"]["thing"].Version == "" {
		t.Fatal("expected lock entry to contain a concrete version")
	}
}

func TestUpdateTrackStrategyPinRefreshesDigestOnly(t *testing.T) {
	atmosConfig := policyConfig(t, &schema.VersionUpdatePolicy{Strategy: StrategyPin, Pin: "digest"})
	seedLock(t, atmosConfig, "v1.9.0")

	update, err := UpdateTrack(atmosConfig, "prod", "", nil)
	if err != nil {
		t.Fatalf("UpdateTrack returned error: %v", err)
	}
	result := update.Results[0]
	if result.To != "v1.9.0" {
		t.Fatalf("expected pinned version to stay v1.9.0, got %q", result.To)
	}
	if result.ToDigest != "pinned-v1.9.0" {
		t.Fatalf("expected refreshed digest, got %q", result.ToDigest)
	}
	if !result.Updated {
		t.Fatal("expected digest refresh to report Updated=true")
	}
}

func TestUpdateTrackOnlyFilters(t *testing.T) {
	atmosConfig := policyConfig(t, &schema.VersionUpdatePolicy{})
	seedLock(t, atmosConfig, "v1.9.0")

	update, err := UpdateTrack(atmosConfig, "prod", "", []string{"other"})
	if err != nil {
		t.Fatalf("UpdateTrack returned error: %v", err)
	}
	if len(update.Results) != 0 {
		t.Fatalf("expected --only filter to exclude the entry, got %d results", len(update.Results))
	}
}

func TestStatusReportsBlockedNewerVersion(t *testing.T) {
	atmosConfig := policyConfig(t, &schema.VersionUpdatePolicy{Strategy: StrategyMinor})
	seedLock(t, atmosConfig, "v1.11.0")

	status, err := StatusTrack(atmosConfig, "prod", "")
	if err != nil {
		t.Fatalf("StatusTrack returned error: %v", err)
	}
	row := status.Entries[0]
	if row.Status != StatusBlocked {
		t.Fatalf("expected %q, got %q (message %q)", StatusBlocked, row.Status, row.Message)
	}
	if !strings.Contains(row.Message, "v2.0.0") {
		t.Fatalf("expected block reason mentioning v2.0.0, got %q", row.Message)
	}

	// Verify must pass: the locked version is policy-current.
	if _, err := VerifyTrack(atmosConfig, "prod"); err != nil {
		t.Fatalf("expected blocked entry to pass verification, got %v", err)
	}
}

func TestVerifyTrackFailsWhenLockedEntryCannotResolve(t *testing.T) {
	atmosConfig := policyConfig(t, &schema.VersionUpdatePolicy{})
	entry := atmosConfig.Version.Tracks["prod"].Dependencies["thing"]
	entry.Datasource = "fake-policy-error"
	atmosConfig.Version.Tracks["prod"].Dependencies["thing"] = entry
	seedLock(t, atmosConfig, "v1.9.0")

	status, err := VerifyTrack(atmosConfig, "prod")
	if err == nil {
		t.Fatal("expected VerifyTrack to fail")
	}
	if !errors.Is(err, ErrTrackNotVerified) {
		t.Fatalf("expected ErrTrackNotVerified, got %v", err)
	}
	if len(status.Entries) != 1 || status.Entries[0].Status != StatusLocked {
		t.Fatalf("expected locked status from resolver error, got %#v", status)
	}
}
