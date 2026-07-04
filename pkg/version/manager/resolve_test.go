package manager

import (
	"context"
	"errors"
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/version/resolver"
)

// fakeResolver serves the "fake-test" datasource with canned candidates and
// records whether Versions was called.
type fakeResolver struct {
	candidates []resolver.Candidate
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
