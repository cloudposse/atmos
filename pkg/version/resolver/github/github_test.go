package github

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/cloudposse/atmos/pkg/version/resolver"
)

func TestResolverNames(t *testing.T) {
	got := Resolver{}.Names()
	if len(got) != 2 || got[0] != DatasourceTags || got[1] != DatasourceReleases {
		t.Fatalf("Names() = %v", got)
	}
}

func TestResolverRejectsInvalidPackageBeforeNetwork(t *testing.T) {
	req := &resolver.Request{Package: "not-a-repo", Datasource: DatasourceTags}
	if _, err := (Resolver{}).Versions(context.Background(), req); !errors.Is(err, ErrInvalidPackage) {
		t.Fatalf("Versions error = %v, want %v", err, ErrInvalidPackage)
	}
	if _, err := (Resolver{}).Pin(context.Background(), req, "v1.0.0"); !errors.Is(err, ErrInvalidPackage) {
		t.Fatalf("Pin error = %v, want %v", err, ErrInvalidPackage)
	}
}

func TestSplitPackageUsesOwnerAndRepo(t *testing.T) {
	owner, repo, err := splitPackage("cloudposse/atmos/.github/workflows/test.yml")
	if err != nil {
		t.Fatalf("splitPackage returned error: %v", err)
	}
	if owner != "cloudposse" || repo != "atmos" {
		t.Fatalf("splitPackage owner/repo = %q/%q", owner, repo)
	}

	for _, pkg := range []string{"", "cloudposse", "/atmos", "cloudposse/"} {
		if _, _, err := splitPackage(pkg); !errors.Is(err, ErrInvalidPackage) {
			t.Fatalf("splitPackage(%q) error = %v, want %v", pkg, err, ErrInvalidPackage)
		}
	}
}

func TestAppendReleaseCandidateSkipsEmptyAndCarriesMetadata(t *testing.T) {
	published := time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)
	var candidates []resolver.Candidate
	candidates = appendReleaseCandidate(candidates, "", false, nil)
	candidates = appendReleaseCandidate(candidates, "v1.2.3", true, &published)
	candidates = appendReleaseCandidate(candidates, "v1.2.2", false, nil)

	if len(candidates) != 2 {
		t.Fatalf("candidate count = %d, want 2: %#v", len(candidates), candidates)
	}
	if candidates[0].Version != "v1.2.3" || !candidates[0].Prerelease {
		t.Fatalf("first candidate = %#v", candidates[0])
	}
	if candidates[0].ReleasedAt == nil || !candidates[0].ReleasedAt.Equal(published) {
		t.Fatalf("released at = %v, want %v", candidates[0].ReleasedAt, published)
	}
	if candidates[1].Version != "v1.2.2" || candidates[1].ReleasedAt != nil {
		t.Fatalf("second candidate = %#v", candidates[1])
	}
}

func TestAppendTagCandidateSkipsEmptyAndCarriesDigest(t *testing.T) {
	var candidates []resolver.Candidate
	candidates = appendTagCandidate(candidates, "", "")
	candidates = appendTagCandidate(candidates, "v1.2.3", "abc123")
	candidates = appendTagCandidate(candidates, "v1.2.2", "")

	if len(candidates) != 2 {
		t.Fatalf("candidate count = %d, want 2: %#v", len(candidates), candidates)
	}
	if candidates[0].Version != "v1.2.3" || candidates[0].Digest != "abc123" {
		t.Fatalf("first candidate = %#v", candidates[0])
	}
	if candidates[1].Version != "v1.2.2" || candidates[1].Digest != "" {
		t.Fatalf("second candidate = %#v", candidates[1])
	}
}
