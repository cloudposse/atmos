package downloader

import (
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestBrokerInsteadOfMatchesURL is a direct table-driven unit test for the insteadOf-match helper.
func TestBrokerInsteadOfMatchesURL(t *testing.T) {
	tests := []struct {
		name         string
		count        string
		key0, value0 string
		urlStr       string
		want         bool
	}{
		{
			name:   "https insteadOf for same host+owner matches",
			count:  "1",
			key0:   "url.https://x-access-token:tok@github.com/acme/.insteadOf",
			value0: "https://github.com/acme/",
			urlStr: "https://github.com/acme/repo",
			want:   true,
		},
		{
			name:   "ssh-only insteadOf does not match an https clone",
			count:  "1",
			key0:   "url.https://x-access-token:tok@github.com/acme/.insteadOf",
			value0: "ssh://git@github.com/acme/",
			urlStr: "https://github.com/acme/repo",
			want:   false,
		},
		{
			name:   "different owner does not match",
			count:  "1",
			key0:   "url.https://x-access-token:tok@github.com/acme/.insteadOf",
			value0: "https://github.com/acme/",
			urlStr: "https://github.com/other/repo",
			want:   false,
		},
		{
			name:   "different host does not match",
			count:  "1",
			key0:   "url.https://x-access-token:tok@github.com/acme/.insteadOf",
			value0: "https://github.com/acme/",
			urlStr: "https://gitlab.com/acme/repo",
			want:   false,
		},
		{
			name:   "non-insteadOf key is ignored",
			count:  "1",
			key0:   "include.path",
			value0: "/some/path",
			urlStr: "https://github.com/acme/repo",
			want:   false,
		},
		{
			name:   "no GIT_CONFIG_COUNT",
			count:  "",
			urlStr: "https://github.com/acme/repo",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.count == "" {
				t.Setenv("GIT_CONFIG_COUNT", "")
			} else {
				t.Setenv("GIT_CONFIG_COUNT", tt.count)
				t.Setenv("GIT_CONFIG_KEY_0", tt.key0)
				t.Setenv("GIT_CONFIG_VALUE_0", tt.value0)
			}

			parsed, err := url.Parse(tt.urlStr)
			require.NoError(t, err)
			got := brokerInsteadOfMatchesURL(parsed, strings.ToLower(parsed.Hostname()))
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestResolveToken_LiveAtmosProGithubToken verifies the live-env fallback for the broker-set token:
// when the struct field is empty, resolveToken reads ATMOS_PRO_GITHUB_TOKEN from the live env; the
// struct field still wins when set.
func TestResolveToken_LiveAtmosProGithubToken(t *testing.T) {
	t.Run("live env used when struct field empty", func(t *testing.T) {
		t.Setenv("ATMOS_PRO_GITHUB_TOKEN", "live-brokered")
		d := NewCustomGitDetector(&schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{AtmosGithubToken: "atmos", GithubToken: "gh"},
		}, "")

		token, source := d.resolveToken(hostGitHub)
		assert.Equal(t, "live-brokered", token)
		assert.Equal(t, "ATMOS_PRO_GITHUB_TOKEN", source)
	})

	t.Run("struct field wins over live env", func(t *testing.T) {
		t.Setenv("ATMOS_PRO_GITHUB_TOKEN", "live-brokered")
		d := NewCustomGitDetector(&schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{AtmosProGithubToken: "struct-pro"},
		}, "")

		token, source := d.resolveToken(hostGitHub)
		assert.Equal(t, "struct-pro", token)
		assert.Equal(t, "ATMOS_PRO_GITHUB_TOKEN", source)
	})

	t.Run("falls through to ATMOS_GITHUB_TOKEN when neither pro source set", func(t *testing.T) {
		t.Setenv("ATMOS_PRO_GITHUB_TOKEN", "")
		d := NewCustomGitDetector(&schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{AtmosGithubToken: "atmos", GithubToken: "gh"},
		}, "")

		token, source := d.resolveToken(hostGitHub)
		assert.Equal(t, "atmos", token)
		assert.Equal(t, "ATMOS_GITHUB_TOKEN", source)
	})
}
