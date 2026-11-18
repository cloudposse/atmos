package artifact

import (
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	// The hostile boundaries count is far above any cap a legitimate address needs, so the tests below hold
	// whatever the cap is.
	hostileBoundaries = 4096
	// The allocation budget is the memory a single hostile input may cost the whole set of entry points.
	// The expansion this guards against grows with the square of the input and costs hundreds of
	// megabytes at the input size used here, so the budget is generous and still far below it.
	allocationBudget = 32 << 20
	// The generous deadline only catches a run that never finishes. It is not a performance assertion.
	generousDeadline = 60 * time.Second
)

// hostileAddress returns an address whose userinfo holds count "@" signs.
func hostileAddress(count int) string {
	return "https://usr7:S3cr3tA" + strings.Repeat("@x", count) + "@example.com/a"
}

// TestRedact_ManyBoundariesFailClosed checks that an address with more "@" signs than any real
// address holds is withheld whole, by every entry point, without expanding a credential candidate
// for each of them.
func TestRedact_ManyBoundariesFailClosed(t *testing.T) {
	raw := hostileAddress(hostileBoundaries)
	word := "https://usr7:S3cr3tA" + strings.Repeat("@x", hostileBoundaries)

	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	start := time.Now()

	assert.Equal(t, RedactedValue, RedactURL(raw), "RedactURL withholds the address")
	assert.Equal(t, RedactedValue, RedactCredentials("fetch "+raw+" failed", raw), "RedactCredentials withholds the text")
	assert.Equal(t, RedactedValue, RedactCredentials("unrelated text", raw), "the text is withheld even when it does not quote the address")
	err := RedactError(errSentinelf(raw), raw)
	assert.Equal(t, RedactedValue, err.Error(), "RedactError withholds the message")
	assert.ErrorIs(t, err, errSentinel, "the cause stays in the chain")
	assert.Equal(t, "see "+RedactedValue+" ok", RedactCredentials("see "+word+" ok", ""), "a word with too many boundaries is withheld on its own")
	assert.NotContains(t, RedactCredentials("see "+word+" ok", ""), "S3cr3tA")

	runtime.ReadMemStats(&after)
	assert.Less(t, time.Since(start), generousDeadline)
	assert.Less(t, after.TotalAlloc-before.TotalAlloc, uint64(allocationBudget), "the work is bounded by the cap, not by the input")
}

// errSentinelf returns an error that wraps errSentinel and quotes text.
func errSentinelf(text string) error {
	return &quotedError{text: text}
}

type quotedError struct{ text string }

func (e *quotedError) Error() string { return "fetch " + e.text + " failed" }
func (e *quotedError) Unwrap() error { return errSentinel }

// TestRedact_AddressBoundsKeepRealAddresses checks that the bounds do not touch an address that is
// within them, so the credential is still cut out, not the whole address withheld.
func TestRedact_AddressBoundsKeepRealAddresses(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "user and password", raw: "https://usr7:S3cr3tA@example.com/a", want: "https://example.com/a"},
		{name: "password with a few at signs", raw: hostileAddress(3), want: "https://example.com/a"},
		{name: "digest reference", raw: "oci://ghcr.io/acme/catalog@" + sha256Digest, want: "oci://ghcr.io/acme/catalog@" + sha256Digest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, RedactURL(tt.raw))
			scrubbed := RedactCredentials("fetch "+tt.raw+" failed", tt.raw)
			assert.NotContains(t, scrubbed, "S3cr3tA")
			assert.NotEqual(t, RedactedValue, scrubbed, "text within the bounds is scrubbed, not withheld")
		})
	}
	require.Positive(t, hostileBoundaries)
}

// TestRedact_AddressBoundsEdges checks the two bounds exactly where they are crossed: an address
// at a bound is redacted in parts, and one past it is withheld whole.
func TestRedact_AddressBoundsEdges(t *testing.T) {
	longPath := strings.Repeat("a", maxAddressLength)
	tests := []struct {
		name         string
		raw          string
		wantURL      string
		wantWithheld bool
	}{
		{name: "at the boundary cap", raw: hostileAddress(maxBoundaries - 1), wantURL: "https://example.com/a"},
		{name: "past the boundary cap", raw: hostileAddress(maxBoundaries), wantWithheld: true},
		{name: "past the length cap with a credential", raw: "https://usr7:S3cr3tA@example.com/" + longPath, wantWithheld: true},
		{name: "long address without an at sign is not a credential", raw: "https://example.com/" + longPath, wantURL: "https://example.com/" + longPath},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scrubbed := RedactCredentials("fetch failed for S3cr3tA at "+tt.raw[:min(len(tt.raw), 40)], tt.raw)
			if strings.Contains(tt.raw, "S3cr3tA") {
				assert.NotContains(t, scrubbed, "S3cr3tA")
			}
			if tt.wantWithheld {
				assert.Equal(t, RedactedValue, RedactURL(tt.raw))
				assert.Equal(t, RedactedValue, scrubbed)
				return
			}
			assert.Equal(t, tt.wantURL, RedactURL(tt.raw))
			assert.NotEqual(t, RedactedValue, scrubbed)
		})
	}
}

// TestCredentialCandidates checks that only distinct strings that can carry userinfo are scrubbed
// for, in the order they were first seen.
func TestCredentialCandidates(t *testing.T) {
	got := credentialCandidates([]string{"plain", "a@b", "a@b", "", "c:d@e", "plain"})
	assert.Equal(t, []string{"a@b", "c:d@e"}, got)
	assert.Empty(t, credentialCandidates(nil))
}
