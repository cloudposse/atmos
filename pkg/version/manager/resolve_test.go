package manager

import (
	"context"
	"errors"
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/version/resolver"
)

// fakeResolver serves the "fake-test" datasource with canned candidates and
// records whether Versions was called. Pin returns a canned digest per
// version when configured, else ErrPinUnsupported.
type fakeResolver struct {
	candidates []resolver.Candidate
	digests    map[string]string
	called     *bool
}

func (f *fakeResolver) Names() []string { return []string{"fake-test"} }

func (f *fakeResolver) Versions(ctx context.Context, req *resolver.Request) ([]resolver.Candidate, error) {
	if f.called != nil {
		*f.called = true
	}
	return f.candidates, nil
}

func (f *fakeResolver) Pin(ctx context.Context, req *resolver.Request, version string) (string, error) {
	if digest, ok := f.digests[version]; ok {
		return digest, nil
	}
	return "", resolver.ErrPinUnsupported
}

var fakeResolverCalled bool

func init() {
	resolver.Register(&fakeResolver{
		candidates: []resolver.Candidate{
			{Version: "v1.2.0"},
			{Version: "v1.10.3"},
			{Version: "v2.0.0"},
		},
		digests: map[string]string{
			"v1.10.3": "1111111111111111111111111111111111111111",
			"v9.9.9":  "9999999999999999999999999999999999999999",
		},
		called: &fakeResolverCalled,
	})
}

func TestResolveTargetConstraintUsesRegisteredResolver(t *testing.T) {
	entry := &EffectiveEntry{Name: "thing", Package: "acme/thing", Datasource: "fake-test", Desired: "^1.0"}
	resolved, err := ResolveTarget(&schema.AtmosConfiguration{}, entry)
	if err != nil {
		t.Fatalf("ResolveTarget returned error: %v", err)
	}
	if resolved != "v1.10.3" {
		t.Fatalf("expected v1.10.3, got %q", resolved)
	}
}

func TestResolveTargetConcreteDesiredShortCircuits(t *testing.T) {
	fakeResolverCalled = false
	entry := &EffectiveEntry{Name: "thing", Package: "acme/thing", Datasource: "fake-test", Desired: "v9.9.9"}
	resolved, err := ResolveTarget(&schema.AtmosConfiguration{}, entry)
	if err != nil {
		t.Fatalf("ResolveTarget returned error: %v", err)
	}
	if resolved != "v9.9.9" {
		t.Fatalf("expected pass-through, got %q", resolved)
	}
	if fakeResolverCalled {
		t.Fatal("expected concrete desired version to skip the resolver")
	}
}

func TestResolveTargetUnknownDatasource(t *testing.T) {
	concrete := &EffectiveEntry{Name: "thing", Package: "x/y", Datasource: "no-such-source", Desired: "1.2.3"}
	resolved, err := ResolveTarget(&schema.AtmosConfiguration{}, concrete)
	if err != nil {
		t.Fatalf("concrete desired must pass through without a resolver: %v", err)
	}
	if resolved != "1.2.3" {
		t.Fatalf("expected 1.2.3, got %q", resolved)
	}

	constrained := &EffectiveEntry{Name: "thing", Package: "x/y", Datasource: "no-such-source", Desired: "~1.2"}
	if _, err := ResolveTarget(&schema.AtmosConfiguration{}, constrained); !errors.Is(err, ErrResolverUnsupported) {
		t.Fatalf("expected ErrResolverUnsupported, got %v", err)
	}
}

func TestResolveTargetEmptyDesired(t *testing.T) {
	entry := &EffectiveEntry{Name: "thing"}
	if _, err := ResolveTarget(&schema.AtmosConfiguration{}, entry); !errors.Is(err, ErrDesiredVersionRequired) {
		t.Fatalf("expected ErrDesiredVersionRequired, got %v", err)
	}
}

func TestResolveEntryPinsConcreteVersion(t *testing.T) {
	entry := &EffectiveEntry{Name: "thing", Package: "acme/thing", Datasource: "fake-test", Desired: "v9.9.9"}
	candidate, err := ResolveEntry(&schema.AtmosConfiguration{}, entry, true)
	if err != nil {
		t.Fatalf("ResolveEntry returned error: %v", err)
	}
	if candidate.Version != "v9.9.9" {
		t.Fatalf("expected v9.9.9, got %q", candidate.Version)
	}
	if candidate.Digest != "9999999999999999999999999999999999999999" {
		t.Fatalf("expected pinned digest, got %q", candidate.Digest)
	}
}

func TestResolveEntryPinsConstraintResolution(t *testing.T) {
	entry := &EffectiveEntry{Name: "thing", Package: "acme/thing", Datasource: "fake-test", Desired: "^1.0"}
	candidate, err := ResolveEntry(&schema.AtmosConfiguration{}, entry, true)
	if err != nil {
		t.Fatalf("ResolveEntry returned error: %v", err)
	}
	if candidate.Version != "v1.10.3" || candidate.Digest != "1111111111111111111111111111111111111111" {
		t.Fatalf("expected pinned v1.10.3, got %+v", candidate)
	}
}

func TestResolveEntryPinFailsLoudlyWhenUnsupported(t *testing.T) {
	// The fake resolver has no digest for v1.2.0, simulating a datasource
	// that cannot pin: a configured pin must fail, not be silently skipped.
	entry := &EffectiveEntry{Name: "thing", Package: "acme/thing", Datasource: "fake-test", Desired: "v1.2.0"}
	if _, err := ResolveEntry(&schema.AtmosConfiguration{}, entry, true); !errors.Is(err, resolver.ErrPinUnsupported) {
		t.Fatalf("expected ErrPinUnsupported, got %v", err)
	}

	// Pinning with no registered resolver at all is a configuration error.
	unknown := &EffectiveEntry{Name: "thing", Package: "x/y", Datasource: "no-such-source", Desired: "1.2.3"}
	if _, err := ResolveEntry(&schema.AtmosConfiguration{}, unknown, true); !errors.Is(err, ErrResolverUnsupported) {
		t.Fatalf("expected ErrResolverUnsupported, got %v", err)
	}
}

func TestLockTrackPopulatesDigestForPinnedEntries(t *testing.T) {
	tempDir := t.TempDir()
	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Version: schema.Version{
			Tracks: map[string]schema.VersionTrack{
				"prod": {
					Versions: map[string]schema.VersionEntry{
						"thing": {
							Datasource: "fake-test",
							Package:    "acme/thing",
							Desired:    "^1.0",
							Update:     schema.VersionUpdatePolicy{Pin: "sha"},
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
	entry := lock.Tracks["prod"]["thing"]
	if entry.Version != "v1.10.3" || entry.Digest != "1111111111111111111111111111111111111111" {
		t.Fatalf("expected pinned lock entry, got %+v", entry)
	}

	versionMap, err := VersionMap(atmosConfig, "prod")
	if err != nil {
		t.Fatalf("VersionMap returned error: %v", err)
	}
	if versionMap["thing"].String() != "1111111111111111111111111111111111111111" {
		t.Fatalf("expected pinned String() form, got %q", versionMap["thing"].String())
	}
	if versionMap["thing"].Version != "v1.10.3" {
		t.Fatalf("expected version accessor, got %q", versionMap["thing"].Version)
	}
}
