package toolchain

import (
	"context"
	"errors"
	"testing"

	"github.com/cloudposse/atmos/pkg/version/resolver"
)

type contextVersionLister struct {
	versions []string
	err      error
	owner    string
	repo     string
	ctx      context.Context
}

type contextKey string

func (l *contextVersionLister) GetAvailableVersionsContext(ctx context.Context, owner, repo string) ([]string, error) {
	l.ctx = ctx
	l.owner = owner
	l.repo = repo
	return l.versions, l.err
}

type legacyVersionLister struct {
	versions []string
	err      error
	owner    string
	repo     string
}

func (l *legacyVersionLister) GetAvailableVersions(owner, repo string) ([]string, error) {
	l.owner = owner
	l.repo = repo
	return l.versions, l.err
}

func TestResolverNamesAndPin(t *testing.T) {
	if got := (Resolver{}).Names(); len(got) != 1 || got[0] != Datasource {
		t.Fatalf("Names() = %v", got)
	}
	if _, err := (Resolver{}).Pin(context.Background(), &resolver.Request{}, "1.0.0"); !errors.Is(err, resolver.ErrPinUnsupported) {
		t.Fatalf("Pin error = %v, want %v", err, resolver.ErrPinUnsupported)
	}
}

func TestAvailableVersionsPrefersContextLister(t *testing.T) {
	ctx := context.WithValue(context.Background(), contextKey("test-key"), "test-value")
	reg := &contextVersionLister{versions: []string{"1.2.0", "1.1.0"}}

	versions, err := availableVersions(ctx, reg, "opentofu", "opentofu")
	if err != nil {
		t.Fatalf("availableVersions returned error: %v", err)
	}
	if stringsJoin(versions) != "1.2.0,1.1.0" {
		t.Fatalf("versions = %v", versions)
	}
	if reg.owner != "opentofu" || reg.repo != "opentofu" || reg.ctx != ctx {
		t.Fatalf("context lister got owner/repo/context = %q/%q/%v", reg.owner, reg.repo, reg.ctx)
	}
}

func TestAvailableVersionsUsesLegacyLister(t *testing.T) {
	reg := &legacyVersionLister{versions: []string{"2.0.0"}}

	versions, err := availableVersions(context.Background(), reg, "helm", "helm")
	if err != nil {
		t.Fatalf("availableVersions returned error: %v", err)
	}
	if stringsJoin(versions) != "2.0.0" {
		t.Fatalf("versions = %v", versions)
	}
	if reg.owner != "helm" || reg.repo != "helm" {
		t.Fatalf("legacy lister got owner/repo = %q/%q", reg.owner, reg.repo)
	}
}

func TestAvailableVersionsPropagatesErrorsAndUnsupportedRegistry(t *testing.T) {
	sentinel := errors.New("registry down")
	if _, err := availableVersions(context.Background(), &contextVersionLister{err: sentinel}, "owner", "repo"); !errors.Is(err, sentinel) {
		t.Fatalf("context lister error = %v, want sentinel", err)
	}
	if _, err := availableVersions(context.Background(), &legacyVersionLister{err: sentinel}, "owner", "repo"); !errors.Is(err, sentinel) {
		t.Fatalf("legacy lister error = %v, want sentinel", err)
	}
	if _, err := availableVersions(context.Background(), struct{}{}, "owner", "repo"); !errors.Is(err, resolver.ErrVersionListingUnsupported) {
		t.Fatalf("unsupported error = %v, want %v", err, resolver.ErrVersionListingUnsupported)
	}
}

func stringsJoin(values []string) string {
	if len(values) == 0 {
		return ""
	}
	result := values[0]
	for _, value := range values[1:] {
		result += "," + value
	}
	return result
}
