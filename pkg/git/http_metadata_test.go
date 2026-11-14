package git

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestExecRunnerHTTPRateLimitMetadata(t *testing.T) {
	requireGit(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "2")
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset", fmt.Sprint(time.Now().Add(time.Minute).Unix()))
		w.Header().Set("Set-Cookie", "private-response-canary")
		w.Header().Set("X-Private", "private-header-canary")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()
	var output bytes.Buffer
	result, err := NewExecRunner().Run(t.Context(), "git", []string{"-c", "credential.helper=", "-c", "http.extraHeader=X-Private: private-request-canary", "ls-remote", server.URL + "/repo.git"}, &RunOptions{Dir: t.TempDir(), CaptureHTTPMetadata: true, Stderr: &output})
	require.Error(t, err)
	require.NotNil(t, result.HTTP)
	require.Equal(t, 429, result.HTTP.Status)
	require.Equal(t, 2*time.Second, *result.HTTP.RetryAfter)
	require.Zero(t, *result.HTTP.Remaining)
	require.Contains(t, result.StderrTail, "429")
	for _, text := range []string{output.String(), result.StderrTail, err.Error()} {
		require.NotContains(t, text, "canary")
		require.NotContains(t, text, "Recv header")
		require.NotContains(t, text, "Send header")
	}
}

func TestHTTPMetadataChunkingAndResponseReset(t *testing.T) {
	var output bytes.Buffer
	capture := &httpMetadataWriter{sink: &output}
	text := "12:34:56.123456 http.c:42 <= Recv header: HTTP/2 429\n" +
		"12:34:56.123456 http.c:42 <= Recv header: Retry-After: 30\n" +
		"12:34:56.123456 http.c:42 <= Recv header: Set-Cookie: SECRET\n" +
		"12:34:56.123456 http.c:42 => Send header: Authorization: SECRET\n" +
		"12:34:56.123456 http.c:42 == Info: PRIVATE\n" +
		"12:34:56.123456 http.c:42 <= Recv header: HTTP/2 200\n" +
		"remote: ordinary Git diagnostic\n"
	for _, b := range []byte(text) {
		_, err := capture.Write([]byte{b})
		require.NoError(t, err)
	}
	require.Equal(t, 200, capture.response.Status)
	require.Nil(t, capture.response.RetryAfter)
	require.Equal(t, "remote: ordinary Git diagnostic\n", output.String())
	_, err := capture.Write([]byte("12:34:56.123456 http.c:42 <= Recv header: X-Secret: " + strings.Repeat("S", httpTraceLineLimit) + "\n"))
	require.NoError(t, err)
	require.Equal(t, "remote: ordinary Git diagnostic\n", output.String())
}

func TestHTTPMetadataDateAndEnvironment(t *testing.T) {
	capture := &httpMetadataWriter{sink: &bytes.Buffer{}}
	capture.header("HTTP/2 429")
	capture.header("Retry-After: " + time.Now().Add(2*time.Minute).UTC().Format(http.TimeFormat))
	require.InDelta(t, 120, capture.response.RetryAfter.Seconds(), 2)
	env := httpMetadataEnv([]string{"GIT_TRACE_CURL=/secret-file", "GIT_TRACE_CURL_NO_DATA=0", "GIT_TRACE_BRIEF=1", "GIT_HTTP_MAX_RETRIES=50", "GIT_CURL_VERBOSE=1", "KEEP=ok"})
	require.Contains(t, env, "GIT_TRACE_CURL=1")
	require.Contains(t, env, "GIT_TRACE_CURL_NO_DATA=1")
	require.Contains(t, env, "GIT_TRACE_BRIEF=0")
	require.Contains(t, env, "GIT_HTTP_MAX_RETRIES=0")
	require.Contains(t, env, "KEEP=ok")
	require.NotContains(t, env, "GIT_TRACE_CURL=/secret-file")
	require.NotContains(t, env, "GIT_CURL_VERBOSE=1")
}

func TestHTTPMetadataRejectsInvalidCooldownsAndBoundsOverflow(t *testing.T) {
	for _, value := range []string{"-1", "garbage", "1.5", ""} {
		require.Nil(t, parseRetryAfter(value), value)
	}
	for _, value := range []string{"18446744073709551615", "184467440737095516160000"} {
		require.Equal(t, time.Duration(math.MaxInt64), *parseRetryAfter(value), value)
	}
	capture := &httpMetadataWriter{sink: &bytes.Buffer{}}
	capture.header("HTTP/2 403")
	capture.header("X-RateLimit-Remaining: -1")
	capture.header("X-RateLimit-Reset: broken")
	require.Nil(t, capture.response.Remaining)
	require.True(t, capture.response.ResetAt.IsZero())
}

func TestHTTPMetadataWriterSinkFailureStopsAtLineBoundary(t *testing.T) {
	sinkErr := errors.New("sink rejected write")
	capture := &httpMetadataWriter{sink: failingWriter{err: sinkErr}}
	n, err := capture.Write([]byte("remote: first\nremote: second\n"))
	require.ErrorIs(t, err, sinkErr)
	// The write stops after the newline that triggered the failing flush.
	require.Equal(t, len("remote: first\n"), n)
	require.Empty(t, capture.line)
}

func TestHTTPMetadataWriterDiscardsUnprefixedTraceMarkers(t *testing.T) {
	var output bytes.Buffer
	capture := &httpMetadataWriter{sink: &output}
	text := "=> Send header: Authorization: SECRET\n" +
		"<= Recv header: Set-Cookie: SECRET\n" +
		"== Info: PRIVATE\n" +
		"remote: ordinary Git diagnostic\n"
	_, err := capture.Write([]byte(text))
	require.NoError(t, err)
	require.Equal(t, "remote: ordinary Git diagnostic\n", output.String())
	require.NotContains(t, output.String(), "SECRET")
	require.NotContains(t, output.String(), "PRIVATE")
}

func TestHTTPMetadataHeaderEdgeCases(t *testing.T) {
	t.Run("header without a colon leaves the response unchanged", func(t *testing.T) {
		capture := &httpMetadataWriter{sink: &bytes.Buffer{}}
		capture.header("HTTP/2 429")
		capture.header("Retry-After: 7")
		capture.header("malformed header line")
		require.Equal(t, 429, capture.response.Status)
		require.Equal(t, 7*time.Second, *capture.response.RetryAfter)
	})

	t.Run("status line without a code resets the response", func(t *testing.T) {
		capture := &httpMetadataWriter{sink: &bytes.Buffer{}}
		capture.header("HTTP/2 429")
		capture.header("Retry-After: 7")
		capture.header("HTTP/2")
		require.Zero(t, capture.response.Status)
		require.Nil(t, capture.response.RetryAfter)
	})

	t.Run("responseStatus", func(t *testing.T) {
		for _, tt := range []struct {
			line   string
			status int
			ok     bool
		}{
			{line: "HTTP/1.1 503 Service Unavailable", status: 503, ok: true},
			{line: "HTTP/2", status: 0, ok: true},
			{line: "HTTP/2 nope", status: 0, ok: true},
			{line: "Retry-After: 5", status: 0, ok: false},
		} {
			status, ok := responseStatus(tt.line)
			require.Equal(t, tt.status, status, tt.line)
			require.Equal(t, tt.ok, ok, tt.line)
		}
	})
}

func TestHTTPMetadataEnvInheritsProcessEnvironmentWhenNil(t *testing.T) {
	t.Setenv("ATMOS_TEST_HTTP_METADATA_KEEP", "ok")
	t.Setenv("GIT_CURL_VERBOSE", "1")
	t.Setenv("GIT_TRACE_CURL", "/secret-file")
	env := httpMetadataEnv(nil)
	require.Contains(t, env, "ATMOS_TEST_HTTP_METADATA_KEEP=ok")
	require.Contains(t, env, "GIT_TRACE_CURL=1")
	require.NotContains(t, env, "GIT_TRACE_CURL=/secret-file")
	require.NotContains(t, env, "GIT_CURL_VERBOSE=1")
}
