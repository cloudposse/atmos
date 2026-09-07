package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetchRepoDescription(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "application/vnd.github+json", r.Header.Get("Accept"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"description": "Atmos is the open-source runtime for infrastructure."}`))
	}))
	defer server.Close()

	got, err := fetchRepoDescription(server.URL)

	require.NoError(t, err)
	assert.Equal(t, "Atmos is the open-source runtime for infrastructure.", got)
}

func TestFetchRepoDescriptionSetsAuthorizationHeaderFromToken(t *testing.T) {
	t.Setenv("GH_TOKEN", "test-token")
	t.Setenv("GITHUB_TOKEN", "")

	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"description": "x"}`))
	}))
	defer server.Close()

	_, err := fetchRepoDescription(server.URL)

	require.NoError(t, err)
	assert.Equal(t, "Bearer test-token", gotAuth)
}

func TestFetchRepoDescriptionNonOKStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	_, err := fetchRepoDescription(server.URL)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "404")
}

func TestFetchRepoDescriptionMalformedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`not json`))
	}))
	defer server.Close()

	_, err := fetchRepoDescription(server.URL)

	require.Error(t, err)
}

func TestFetchRepoDescriptionEmptyDescription(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"description": ""}`))
	}))
	defer server.Close()

	_, err := fetchRepoDescription(server.URL)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no description")
}

func TestFetchRepoDescriptionRejectsNewline(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"description": "line one\nline two"}`))
	}))
	defer server.Close()

	_, err := fetchRepoDescription(server.URL)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "line breaks or non-printable characters")
}

func TestFetchRepoDescriptionRejectsControlCharacter(t *testing.T) {
	// Marshal a struct instead of hand-writing JSON, so the control
	// character (Go escape sequence, byte value 7) is embedded correctly
	// without putting a raw non-printable byte in this source file.
	body, err := json.Marshal(struct {
		Description string `json:"description"`
	}{Description: "ring" + string(rune(7)) + "bell"})
	require.NoError(t, err)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(body)
	}))
	defer server.Close()

	_, err = fetchRepoDescription(server.URL)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "line breaks or non-printable characters")
}

func TestFetchRepoDescriptionRejectsLineSeparator(t *testing.T) {
	// U+2028 (Line Separator): valid inside a JSON string, but rendered as
	// a line break by many consumers, so unicode.IsControl alone misses it.
	body, err := json.Marshal(struct {
		Description string `json:"description"`
	}{Description: "line one" + string(rune(0x2028)) + "line two"})
	require.NoError(t, err)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(body)
	}))
	defer server.Close()

	_, err = fetchRepoDescription(server.URL)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "line breaks or non-printable characters")
}

func TestFetchRepoDescriptionRejectsParagraphSeparator(t *testing.T) {
	// U+2029 (Paragraph Separator): same rationale as the Line Separator
	// case above.
	body, err := json.Marshal(struct {
		Description string `json:"description"`
	}{Description: "line one" + string(rune(0x2029)) + "line two"})
	require.NoError(t, err)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(body)
	}))
	defer server.Close()

	_, err = fetchRepoDescription(server.URL)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "line breaks or non-printable characters")
}

func TestFetchRepoDescriptionBuildRequestError(t *testing.T) {
	// A raw control character in the URL fails http.NewRequest's underlying
	// url.Parse before any network call is attempted.
	_, err := fetchRepoDescription("http://example.com/" + string(rune(0x7f)))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "build request")
}

func TestFetchRepoDescriptionDoError(t *testing.T) {
	// An unsupported URL scheme makes http.Client.Do fail immediately,
	// without depending on a real unreachable network address.
	_, err := fetchRepoDescription("unsupported-scheme://example.com")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "fetch")
}

func TestIsSingleLinePrintable(t *testing.T) {
	assert.True(t, isSingleLinePrintable("Atmos is the open-source runtime for infrastructure."))
	assert.True(t, isSingleLinePrintable("unicode em dash "+string(rune(0x2014))+" is fine"))
	assert.False(t, isSingleLinePrintable("line one\nline two"))
	assert.False(t, isSingleLinePrintable("carriage\rreturn"))
	assert.False(t, isSingleLinePrintable("tab\ttab"))
	assert.False(t, isSingleLinePrintable("line one"+string(rune(0x2028))+"line two"), "U+2028 Line Separator")
	assert.False(t, isSingleLinePrintable("line one"+string(rune(0x2029))+"line two"), "U+2029 Paragraph Separator")
}

func TestHTTPClientHasFiniteTimeout(t *testing.T) {
	assert.Equal(t, fetchTimeout, httpClient.Timeout)
	assert.NotZero(t, httpClient.Timeout, "an unbounded client can hang NOTICE generation indefinitely on a stalled server")
}

func TestGithubTokenPrefersGHToken(t *testing.T) {
	t.Setenv("GH_TOKEN", "gh-token")
	t.Setenv("GITHUB_TOKEN", "github-token")

	assert.Equal(t, "gh-token", githubToken())
}

func TestGithubTokenFallsBackToGitHubToken(t *testing.T) {
	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "github-token")

	assert.Equal(t, "github-token", githubToken())
}

func TestGithubTokenEmptyWhenNeitherSet(t *testing.T) {
	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")

	assert.Empty(t, githubToken())
}
