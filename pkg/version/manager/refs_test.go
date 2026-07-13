package manager

import (
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestVersionRefString(t *testing.T) {
	cases := []struct {
		name     string
		ref      VersionRef
		expected string
	}{
		{"unpinned uses version", VersionRef{Version: "v4.2.2", Digest: "abc"}, "v4.2.2"},
		{"pinned uses digest", VersionRef{Version: "v4.2.2", Digest: "abc", Pin: PinDigest}, "abc"},
		{"pinned without digest falls back to version", VersionRef{Version: "v4.2.2", Pin: PinDigest}, "v4.2.2"},
	}
	for _, testCase := range cases {
		if got := testCase.ref.String(); got != testCase.expected {
			t.Errorf("%s: got %q, expected %q", testCase.name, got, testCase.expected)
		}
	}
}

func TestNormalizePin(t *testing.T) {
	cases := map[string]string{
		"":       PinNone,
		"none":   PinNone,
		"digest": PinDigest,
		"sha":    PinDigest,
		"bogus":  PinNone,
	}
	for input, expected := range cases {
		if got := normalizePin(input); got != expected {
			t.Errorf("normalizePin(%q) = %q, expected %q", input, got, expected)
		}
	}
}

func TestPinInheritsThroughPolicyChain(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Version: schema.Version{
			Defaults: schema.VersionPolicy{
				Update: schema.VersionUpdatePolicy{Pin: "sha"},
			},
			Tracks: map[string]schema.VersionTrack{
				"prod": {
					Dependencies: map[string]schema.VersionEntry{
						"checkout": {Ecosystem: "github/actions", Package: "actions/checkout", Desired: "v6"},
						"nginx": {
							Ecosystem: "oci",
							Package:   "library/nginx",
							Desired:   "1.28.0",
							Update:    schema.VersionUpdatePolicy{Pin: "none"},
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
	if !pinEnabled(&checkout) {
		t.Fatal("expected checkout to inherit pin from global defaults")
	}
	nginx := entries["nginx"]
	if pinEnabled(&nginx) {
		t.Fatal("expected nginx entry-level pin: none to override the default")
	}
}
