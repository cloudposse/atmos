package github

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// clearGitHubEndpointEnv unsets every environment variable the endpoint resolvers read, so
// each test starts from a known (unset) baseline regardless of the ambient environment
// (e.g. a real GITHUB_ACTIONS runner exports GITHUB_SERVER_URL/GITHUB_API_URL).
func clearGitHubEndpointEnv(t *testing.T) {
	t.Helper()

	for _, envVar := range []string{
		"GITHUB_SERVER_URL",
		"GITHUB_API_URL",
		"ATMOS_TOOLCHAIN_GITHUB_URL",
		"ATMOS_TOOLCHAIN_GITHUB_API_URL",
	} {
		t.Setenv(envVar, "")
	}
}

func TestRepoEndpoints_Defaults(t *testing.T) {
	clearGitHubEndpointEnv(t)

	e := RepoEndpoints()

	assert.Equal(t, "https://github.com", e.ServerURL)
	assert.Equal(t, "https://api.github.com", e.APIURL)
	assert.Equal(t, "https://uploads.github.com", e.UploadURL)
	assert.Equal(t, "github.com", e.Host)
}

func TestRepoEndpoints_GHES(t *testing.T) {
	clearGitHubEndpointEnv(t)
	t.Setenv("GITHUB_SERVER_URL", "https://ghes.example.com")
	t.Setenv("GITHUB_API_URL", "https://ghes.example.com/api/v3")

	e := RepoEndpoints()

	assert.Equal(t, "https://ghes.example.com", e.ServerURL)
	assert.Equal(t, "https://ghes.example.com/api/v3", e.APIURL)
	assert.Equal(t, "https://ghes.example.com/api/uploads", e.UploadURL)
	assert.Equal(t, "ghes.example.com", e.Host)
}

func TestRepoEndpoints_TrailingSlashesTrimmed(t *testing.T) {
	clearGitHubEndpointEnv(t)
	t.Setenv("GITHUB_SERVER_URL", "https://ghes.example.com/")
	t.Setenv("GITHUB_API_URL", "https://ghes.example.com/api/v3/")

	e := RepoEndpoints()

	assert.Equal(t, "https://ghes.example.com", e.ServerURL)
	assert.Equal(t, "https://ghes.example.com/api/v3", e.APIURL)
}

func TestRepoEndpoints_InvalidURLFallsBackToDefault(t *testing.T) {
	clearGitHubEndpointEnv(t)
	t.Setenv("GITHUB_SERVER_URL", "not a url")
	t.Setenv("GITHUB_API_URL", "://also-not-a-url")

	e := RepoEndpoints()

	assert.Equal(t, "https://github.com", e.ServerURL)
	assert.Equal(t, "https://api.github.com", e.APIURL)
	assert.Equal(t, "github.com", e.Host)
}

// TestRepoEndpoints_GHESWithoutAPIURLDerivesAPIv3 pins that a GHES server host with no
// GITHUB_API_URL override derives "<server>/api/v3" as its API endpoint, instead of silently
// falling back to the public api.github.com -- doing the latter would send the GHES-scoped
// token to the public GitHub API (a sensitive-data-exposure bug).
func TestRepoEndpoints_GHESWithoutAPIURLDerivesAPIv3(t *testing.T) {
	clearGitHubEndpointEnv(t)
	t.Setenv("GITHUB_SERVER_URL", "https://ghes.example.com")

	e := RepoEndpoints()

	assert.Equal(t, "https://ghes.example.com/api/v3", e.APIURL)
	assert.NotEqual(t, defaultGitHubAPIURL, e.APIURL)
}

// TestRepoEndpoints_GHESWithInvalidAPIURLDerivesAPIv3 verifies the same fallback applies when
// GITHUB_API_URL is set but fails to parse, rather than falling back to the public default.
func TestRepoEndpoints_GHESWithInvalidAPIURLDerivesAPIv3(t *testing.T) {
	clearGitHubEndpointEnv(t)
	t.Setenv("GITHUB_SERVER_URL", "https://ghes.example.com")
	t.Setenv("GITHUB_API_URL", "not a url")

	e := RepoEndpoints()

	assert.Equal(t, "https://ghes.example.com/api/v3", e.APIURL)
}

func TestRepoEndpoints_SchemelessURLFallsBackToDefault(t *testing.T) {
	clearGitHubEndpointEnv(t)
	// A bare host with no scheme parses as a path under url.ParseRequestURI, not a host,
	// so it must be rejected rather than silently accepted as e.g. "http://ghes.example.com".
	t.Setenv("GITHUB_SERVER_URL", "ghes.example.com")

	e := RepoEndpoints()

	assert.Equal(t, "https://github.com", e.ServerURL)
	assert.Equal(t, "github.com", e.Host)
}

// TestResolveEndpointURL_RejectsRawQueryFragmentAndUser pins CodeRabbit thread
// PRRT_kwDOEW4XoM6h7p3K: a base URL is only ever used as a prefix that owner/repo/ref/path
// segments are appended to, so a RawQuery or Fragment would be silently discarded once those
// segments are appended after it, and a User component would leak credentials into every URL
// built from the base. All three must be rejected in favor of the default, exactly like an
// unparsable value.
func TestResolveEndpointURL_RejectsRawQueryFragmentAndUser(t *testing.T) {
	const fallback = "https://fallback.example.com"

	tests := []struct {
		name  string
		value string
	}{
		{name: "query string", value: "https://ghes.example.com?token=leaked"},
		{name: "fragment", value: "https://ghes.example.com#frag"},
		{name: "userinfo", value: "https://user:pass@ghes.example.com"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("ATMOS_TEST_ENDPOINT_URL", tc.value)
			assert.Equal(t, fallback, ResolveEndpointURL("ATMOS_TEST_ENDPOINT_URL", fallback))
		})
	}
}

// TestResolveEndpointURL_AcceptsPlainURL pins that a well-formed base URL with no query,
// fragment, or userinfo component still resolves as-is (i.e. the new rejections above are
// additive, not a regression for the common case).
func TestResolveEndpointURL_AcceptsPlainURL(t *testing.T) {
	t.Setenv("ATMOS_TEST_ENDPOINT_URL", "https://ghes.example.com/api/v3")
	assert.Equal(t, "https://ghes.example.com/api/v3", ResolveEndpointURL("ATMOS_TEST_ENDPOINT_URL", "https://fallback.example.com"))
}

func TestToolchainEndpoints_DefaultsToPublicGitHub(t *testing.T) {
	clearGitHubEndpointEnv(t)
	// Even when the repo endpoints point at GHES, toolchain endpoints must default to
	// public github.com independently -- they are a separate concern.
	t.Setenv("GITHUB_SERVER_URL", "https://ghes.example.com")
	t.Setenv("GITHUB_API_URL", "https://ghes.example.com/api/v3")

	e := ToolchainEndpoints()

	assert.Equal(t, "https://github.com", e.ServerURL)
	assert.Equal(t, "https://api.github.com", e.APIURL)
	assert.Equal(t, "github.com", e.Host)
}

func TestToolchainEndpoints_CorporateMirror(t *testing.T) {
	clearGitHubEndpointEnv(t)
	t.Setenv("ATMOS_TOOLCHAIN_GITHUB_URL", "https://releases.corp.example.com")
	t.Setenv("ATMOS_TOOLCHAIN_GITHUB_API_URL", "https://releases.corp.example.com/api/v3")

	e := ToolchainEndpoints()

	assert.Equal(t, "https://releases.corp.example.com", e.ServerURL)
	assert.Equal(t, "https://releases.corp.example.com/api/v3", e.APIURL)
	assert.Equal(t, "releases.corp.example.com", e.Host)
}

// TestToolchainEndpoints_MirrorWithoutAPIURLDerivesAPIv3 mirrors
// TestRepoEndpoints_GHESWithoutAPIURLDerivesAPIv3 for ToolchainEndpoints: a corporate mirror
// configured only via ATMOS_TOOLCHAIN_GITHUB_URL (no API URL override) must not silently send
// its token to the public api.github.com.
func TestToolchainEndpoints_MirrorWithoutAPIURLDerivesAPIv3(t *testing.T) {
	clearGitHubEndpointEnv(t)
	t.Setenv("ATMOS_TOOLCHAIN_GITHUB_URL", "https://releases.corp.example.com")

	e := ToolchainEndpoints()

	assert.Equal(t, "https://releases.corp.example.com/api/v3", e.APIURL)
}

func TestEndpoints_IsHost(t *testing.T) {
	tests := []struct {
		name string
		host string
		e    Endpoints
		want bool
	}{
		{name: "exact match", host: "ghes.example.com", e: Endpoints{Host: "ghes.example.com"}, want: true},
		{name: "case insensitive", host: "GHES.Example.COM", e: Endpoints{Host: "ghes.example.com"}, want: true},
		{name: "trailing dot", host: "ghes.example.com.", e: Endpoints{Host: "ghes.example.com"}, want: true},
		{name: "default https port stripped", host: "ghes.example.com:443", e: Endpoints{Host: "ghes.example.com"}, want: true},
		{name: "default http port stripped", host: "ghes.example.com:80", e: Endpoints{Host: "ghes.example.com"}, want: true},
		{name: "non-default port preserved", host: "ghes.example.com:8443", e: Endpoints{Host: "ghes.example.com"}, want: false},
		{name: "mismatch", host: "github.com", e: Endpoints{Host: "ghes.example.com"}, want: false},
		{name: "does not implicitly match github.com", host: "github.com", e: Endpoints{Host: "ghes.example.com"}, want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.e.IsHost(tc.host))
		})
	}
}

func TestEndpoints_RawURL_GitHubCom(t *testing.T) {
	e := Endpoints{ServerURL: "https://github.com", Host: "github.com"}

	assert.Equal(t,
		"https://raw.githubusercontent.com/cloudposse/atmos/main/README.md",
		e.RawURL("cloudposse", "atmos", "main", "README.md"))

	// Leading slash on path is stripped.
	assert.Equal(t,
		"https://raw.githubusercontent.com/cloudposse/atmos/main/README.md",
		e.RawURL("cloudposse", "atmos", "main", "/README.md"))

	// Empty path returns the ref root, no trailing slash.
	assert.Equal(t,
		"https://raw.githubusercontent.com/cloudposse/atmos/main",
		e.RawURL("cloudposse", "atmos", "main", ""))
}

func TestEndpoints_RawURL_GHES(t *testing.T) {
	e := Endpoints{ServerURL: "https://ghes.example.com", Host: "ghes.example.com"}

	assert.Equal(t,
		"https://ghes.example.com/raw/cloudposse/atmos/main/README.md",
		e.RawURL("cloudposse", "atmos", "main", "README.md"))

	assert.Equal(t,
		"https://ghes.example.com/raw/cloudposse/atmos/main",
		e.RawURL("cloudposse", "atmos", "main", ""))
}

func TestEndpoints_ReleaseAssetURL(t *testing.T) {
	e := Endpoints{ServerURL: "https://ghes.example.com", Host: "ghes.example.com"}

	assert.Equal(t,
		"https://ghes.example.com/cloudposse/atmos/releases/download/v1.0.0/atmos_linux_amd64",
		e.ReleaseAssetURL("cloudposse", "atmos", "v1.0.0", "atmos_linux_amd64"))
}

func TestEndpoints_ArchiveURL(t *testing.T) {
	e := Endpoints{ServerURL: "https://github.com", Host: "github.com"}

	assert.Equal(t,
		"https://github.com/aquaproj/aqua-registry/archive/refs/tags/v4.0.0.tar.gz",
		e.ArchiveURL("aquaproj", "aqua-registry", "v4.0.0"))
}

// TestEndpoints_RawURL_EscapesReservedCharacters pins that RawURL percent-encodes owner,
// repo, and ref as opaque single path segments (a "/" inside one of them becomes "%2F" data,
// not an extra path separator), while a genuine multi-segment file path has each of its own
// "/"-separated segments escaped independently.
func TestEndpoints_RawURL_EscapesReservedCharacters(t *testing.T) {
	e := Endpoints{ServerURL: "https://github.com", Host: "github.com"}

	assert.Equal(t,
		"https://raw.githubusercontent.com/cloudposse/atmos/main/dir%20with%20space/file%23name.tf",
		e.RawURL("cloudposse", "atmos", "main", "dir with space/file#name.tf"))

	// A "/" embedded in ref is escaped as data, not reinterpreted as a path separator.
	assert.Equal(t,
		"https://raw.githubusercontent.com/cloudposse/atmos/feature%2Ffoo/README.md",
		e.RawURL("cloudposse", "atmos", "feature/foo", "README.md"))

	ghes := Endpoints{ServerURL: "https://ghes.example.com", Host: "ghes.example.com"}
	assert.Equal(t,
		"https://ghes.example.com/raw/cloudposse/atmos/main/dir/file%3Fname.tf",
		ghes.RawURL("cloudposse", "atmos", "main", "dir/file?name.tf"))
}

// TestEndpoints_ReleaseAssetURL_EscapesReservedCharacters pins that ReleaseAssetURL
// percent-encodes every component as an opaque path segment.
func TestEndpoints_ReleaseAssetURL_EscapesReservedCharacters(t *testing.T) {
	e := Endpoints{ServerURL: "https://ghes.example.com", Host: "ghes.example.com"}

	assert.Equal(t,
		"https://ghes.example.com/cloudposse/atmos/releases/download/v1.0.0%23beta/atmos%3F.tar.gz",
		e.ReleaseAssetURL("cloudposse", "atmos", "v1.0.0#beta", "atmos?.tar.gz"))
}

// TestEndpoints_ArchiveURL_EscapesReservedCharacters pins that ArchiveURL percent-encodes
// every component as an opaque path segment.
func TestEndpoints_ArchiveURL_EscapesReservedCharacters(t *testing.T) {
	e := Endpoints{ServerURL: "https://github.com", Host: "github.com"}

	assert.Equal(t,
		"https://github.com/aquaproj/aqua-registry/archive/refs/tags/v4.0.0%23rc1.tar.gz",
		e.ArchiveURL("aquaproj", "aqua-registry", "v4.0.0#rc1"))
}

func TestEndpoints_AllowsToken(t *testing.T) {
	tests := []struct {
		name string
		e    Endpoints
		want bool
	}{
		{name: "https API URL allows token", e: Endpoints{APIURL: "https://api.github.com"}, want: true},
		{name: "http API URL withholds token", e: Endpoints{APIURL: "http://127.0.0.1:8080"}, want: false},
		{name: "GHES https API URL allows token", e: Endpoints{APIURL: "https://ghes.example.com/api/v3"}, want: true},
		{name: "invalid API URL withholds token", e: Endpoints{APIURL: "://not-a-url"}, want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.e.AllowsToken())
		})
	}
}

func TestTokenForEndpoints(t *testing.T) {
	httpsEndpoints := Endpoints{APIURL: "https://api.github.com"}
	httpEndpoints := Endpoints{APIURL: "http://127.0.0.1:8080"}

	assert.Equal(t, "secret-token", TokenForEndpoints(httpsEndpoints, "secret-token"), "https endpoint keeps the token")
	assert.Equal(t, "", TokenForEndpoints(httpEndpoints, "secret-token"), "http endpoint withholds the token")
	assert.Equal(t, "", TokenForEndpoints(httpsEndpoints, ""), "empty token stays empty")
	assert.Equal(t, "", TokenForEndpoints(httpEndpoints, ""), "empty token stays empty even over http")
}

func TestNormalizeHost(t *testing.T) {
	tests := []struct {
		name string
		host string
		want string
	}{
		{"bare bracketed IPv6 unbrackets", "[::1]", "::1"},
		{"bracketed IPv6 with default port unbrackets", "[::1]:443", "::1"},
		{"bracketed IPv6 with custom port keeps the port", "[::1]:8443", "[::1]:8443"},
		{name: "lowercased", host: "GitHub.com", want: "github.com"},
		{name: "trailing dot stripped", host: "github.com.", want: "github.com"},
		{name: "default https port stripped", host: "github.com:443", want: "github.com"},
		{name: "default http port stripped", host: "github.com:80", want: "github.com"},
		{name: "non-default port preserved", host: "github.com:8443", want: "github.com:8443"},
		{name: "trailing dot with non-default port stripped", host: "ghes.example.com.:8443", want: "ghes.example.com:8443"},
		{name: "trailing dot with default https port stripped", host: "GHES.example.com.:443", want: "ghes.example.com"},
		{name: "bare trailing dot stripped", host: "github.com.", want: "github.com"},
		{name: "IPv6 with non-default port preserved", host: "[::1]:8443", want: "[::1]:8443"},
		{name: "IPv6 with default https port stripped", host: "[::1]:443", want: "::1"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, normalizeHost(tc.host))
		})
	}
}

// TestNormalizeHostForScheme pins CodeRabbit thread PRRT_kwDOEW4XoM6h7p3L: unlike normalizeHost
// (which strips both port 80 and port 443 unconditionally), normalizeHostForScheme only strips
// a port when it is the *actual* default for scheme (443 for https, 80 for http), so an
// explicit non-default-for-that-scheme port is preserved and compared literally.
func TestNormalizeHostForScheme(t *testing.T) {
	tests := []struct {
		name   string
		host   string
		scheme string
		want   string
	}{
		{name: "https default port stripped", host: "host:443", scheme: "https", want: "host"},
		{name: "https on port 80 is preserved (not its default)", host: "host:80", scheme: "https", want: "host:80"},
		{name: "http default port stripped", host: "host:80", scheme: "http", want: "host"},
		{name: "http on port 443 is preserved (not its default)", host: "host:443", scheme: "http", want: "host:443"},
		{name: "non-default port preserved regardless of scheme", host: "host:8443", scheme: "https", want: "host:8443"},
		{name: "no port is unchanged", host: "host", scheme: "https", want: "host"},
		{name: "lowercased and trailing dot stripped", host: "HOST.example.com.", scheme: "https", want: "host.example.com"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, normalizeHostForScheme(tc.host, tc.scheme))
		})
	}
}

// TestEndpoints_IsHostForScheme pins the scheme-aware token-path host check: it must reject an
// explicit port that is not the actual default for the given scheme, even when normalizeHost
// (used by the non-scheme-aware IsHost) would treat it as equivalent to the bare host.
func TestEndpoints_IsHostForScheme(t *testing.T) {
	e := Endpoints{Host: "ghes.example.com"}

	assert.True(t, e.IsHostForScheme("ghes.example.com:443", "https"), "https on its default port 443 must match")
	assert.False(t, e.IsHostForScheme("ghes.example.com:80", "https"), "https on port 80 (not its default) must not match")
	assert.True(t, e.IsHostForScheme("ghes.example.com:80", "http"), "http on its default port 80 does match a bare host, but only for an http request")
	assert.True(t, e.IsHostForScheme("ghes.example.com", "https"), "bare host matches")
	assert.False(t, e.IsHostForScheme("ghes.example.com:8443", "https"), "explicit non-default port must match exactly")
}

func TestHostOf_InvalidURL(t *testing.T) {
	// A control character makes url.Parse fail outright, exercising the error branch.
	require.Empty(t, hostOf("http://\x7f"))
}

// TestRepoEndpoints_NonDefaultPortPreserved verifies that a GITHUB_SERVER_URL on a non-default
// port (e.g. a corporate GHES mirror behind a custom port, or a test's httptest.NewServer) keeps
// that port in Endpoints.Host, so the endpoint can recognize its own URLs via IsHost. Before
// this, hostOf built Endpoints.Host from url.URL.Hostname() (which always drops the port), while
// IsHost/normalizeHost preserve a non-default port on the candidate side -- so an Endpoints value
// for a host on a non-default port could never match even its own configured URL.
func TestRepoEndpoints_NonDefaultPortPreserved(t *testing.T) {
	clearGitHubEndpointEnv(t)
	t.Setenv("GITHUB_SERVER_URL", "http://127.0.0.1:19199")
	t.Setenv("GITHUB_API_URL", "http://127.0.0.1:19199/api/v3")

	e := RepoEndpoints()

	assert.Equal(t, "127.0.0.1:19199", e.Host)
	assert.True(t, e.IsHost("127.0.0.1:19199"))
	assert.False(t, e.IsHost("127.0.0.1"))
}
