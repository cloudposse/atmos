package resolver

import (
	"errors"
	"testing"
)

func TestSelectLatestPicksHighestSemverRegardlessOfOrder(t *testing.T) {
	candidates := []Candidate{
		{Version: "v1.2.0"},
		{Version: "v1.10.3", Digest: "abc123"},
		{Version: "v1.9.9"},
	}
	selected, err := Select(candidates, "latest", nil, nil)
	if err != nil {
		t.Fatalf("Select returned error: %v", err)
	}
	if selected.Version != "v1.10.3" {
		t.Fatalf("expected v1.10.3, got %q", selected.Version)
	}
	if selected.Digest != "abc123" {
		t.Fatalf("expected digest to ride along, got %q", selected.Digest)
	}
}

func TestSelectConstraintPicksHighestMatch(t *testing.T) {
	candidates := []Candidate{
		{Version: "v2.0.0"},
		{Version: "v1.10.3"},
		{Version: "v1.10.7"},
		{Version: "v1.9.9"},
	}
	selected, err := Select(candidates, "~1.10", nil, nil)
	if err != nil {
		t.Fatalf("Select returned error: %v", err)
	}
	if selected.Version != "v1.10.7" {
		t.Fatalf("expected v1.10.7, got %q", selected.Version)
	}
}

func TestSelectConcreteVersionReturnsMatchingCandidate(t *testing.T) {
	candidates := []Candidate{
		{Version: "v6"},
		{Version: "v5", Digest: "sha-v5"},
	}
	selected, err := Select(candidates, "v5", nil, nil)
	if err != nil {
		t.Fatalf("Select returned error: %v", err)
	}
	if selected.Digest != "sha-v5" {
		t.Fatalf("expected concrete match with digest, got %+v", selected)
	}
}

func TestSelectExcludesPrereleasesByDefault(t *testing.T) {
	candidates := []Candidate{
		{Version: "v2.0.0-rc.1"},
		{Version: "v1.9.0", Prerelease: true},
		{Version: "v1.8.0"},
	}
	selected, err := Select(candidates, "latest", nil, nil)
	if err != nil {
		t.Fatalf("Select returned error: %v", err)
	}
	if selected.Version != "v1.8.0" {
		t.Fatalf("expected v1.8.0 (prereleases excluded), got %q", selected.Version)
	}

	selected, err = Select(candidates, "latest", []string{"prerelease"}, nil)
	if err != nil {
		t.Fatalf("Select returned error: %v", err)
	}
	if selected.Version != "v2.0.0-rc.1" {
		t.Fatalf("expected v2.0.0-rc.1 with prerelease allowed, got %q", selected.Version)
	}
}

func TestSelectHonorsIgnorePatterns(t *testing.T) {
	candidates := []Candidate{
		{Version: "v1.10.0"},
		{Version: "v1.9.0"},
	}
	selected, err := Select(candidates, "latest", nil, []string{"v1.10.*"})
	if err != nil {
		t.Fatalf("Select returned error: %v", err)
	}
	if selected.Version != "v1.9.0" {
		t.Fatalf("expected glob-ignored v1.10.0 to be skipped, got %q", selected.Version)
	}
}

func TestSelectNoMatchReturnsError(t *testing.T) {
	candidates := []Candidate{{Version: "v1.0.0"}}
	if _, err := Select(candidates, "~9.9", nil, nil); !errors.Is(err, ErrNoVersionMatch) {
		t.Fatalf("expected ErrNoVersionMatch, got %v", err)
	}
	if _, err := Select(nil, "latest", nil, nil); !errors.Is(err, ErrNoVersionMatch) {
		t.Fatalf("expected ErrNoVersionMatch for empty candidates, got %v", err)
	}
}

func TestSelectLatestFallsBackToDatasourceOrderForNonSemver(t *testing.T) {
	candidates := []Candidate{
		{Version: "buster"},
		{Version: "bullseye"},
	}
	selected, err := Select(candidates, "latest", nil, nil)
	if err != nil {
		t.Fatalf("Select returned error: %v", err)
	}
	if selected.Version != "buster" {
		t.Fatalf("expected first candidate as fallback, got %q", selected.Version)
	}
}

func TestLooksLikeConstraint(t *testing.T) {
	cases := map[string]bool{
		"":               false,
		"latest":         false,
		"v1.2.3":         false,
		"1.2.3":          false,
		"~1.10":          true,
		"^2.0":           true,
		">= 1.2, < 2":    true,
		"1.2.x || 2.0.x": true,
	}
	for input, expected := range cases {
		if got := LooksLikeConstraint(input); got != expected {
			t.Errorf("LooksLikeConstraint(%q) = %v, expected %v", input, got, expected)
		}
	}
}
