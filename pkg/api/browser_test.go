package api

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestBrowserCallbackCorrelation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	listener, err := (LocalBrowser{Interactive: true}).Listen(ctx, "/callback", "expected")
	require.NoError(t, err)
	defer listener.Close()
	for _, query := range []string{"state=wrong&code=secret", "code=secret", "state=expected&state=wrong&code=secret", "state=expected&code=one&code=two", "state=expected&code=secret&error=denied"} {
		response, err := http.Get(listener.RedirectURI() + "?" + query)
		require.NoError(t, err)
		body, err := io.ReadAll(response.Body)
		require.NoError(t, response.Body.Close())
		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, response.StatusCode)
		assert.NotContains(t, string(body), "secret")
	}
	response, err := http.Get(listener.RedirectURI() + "?state=expected&code=valid")
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	assert.Equal(t, http.StatusOK, response.StatusCode)
	callback, err := listener.Receive(ctx)
	require.NoError(t, err)
	assert.Equal(t, Callback{Code: "valid", State: "expected"}, callback)
	_, err = listener.Receive(ctx)
	require.ErrorIs(t, err, io.EOF)
	response, err = http.Get(listener.RedirectURI() + "?state=expected&code=replay")
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	assert.Equal(t, http.StatusConflict, response.StatusCode)
}

func TestBrowserNoninteractiveAndCancellation(t *testing.T) {
	t.Parallel()
	browser := LocalBrowser{}
	_, err := browser.Listen(t.Context(), "/callback", "state")
	require.ErrorIs(t, err, errUtils.ErrPluginInteractionRequired)
	require.ErrorIs(t, browser.Open(t.Context(), "https://issuer.example"), errUtils.ErrPluginInteractionRequired)
	browser.Interactive = true
	ctx, cancel := context.WithCancel(t.Context())
	listener, err := browser.Listen(ctx, "/callback", "state")
	require.NoError(t, err)
	cancel()
	_, err = listener.Receive(t.Context())
	require.ErrorIs(t, err, context.Canceled)
	require.NoError(t, listener.Close())
	client := &http.Client{Timeout: time.Second}
	response, err := client.Get(listener.RedirectURI())
	if response != nil {
		require.NoError(t, response.Body.Close())
	}
	require.Error(t, err)
}

func TestBrowserDenialAndUnrelatedPath(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	listener, err := (LocalBrowser{Interactive: true}).Listen(ctx, "/callback", "expected")
	require.NoError(t, err)
	defer listener.Close()
	unrelated := strings.Replace(listener.RedirectURI(), "/callback", "/favicon.ico", 1)
	response, err := http.Get(unrelated + "?state=expected&code=wrong-path")
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	assert.Equal(t, http.StatusBadRequest, response.StatusCode)
	response, err = http.Get(listener.RedirectURI() + "?" + url.Values{"state": {"expected"}, "error": {"access_denied"}}.Encode())
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	callback, err := listener.Receive(ctx)
	require.NoError(t, err)
	assert.Equal(t, Callback{State: "expected", Error: "access_denied"}, callback)
}

func TestBrowserRejectsInvalidConfiguration(t *testing.T) {
	t.Parallel()
	browser := LocalBrowser{Interactive: true}
	for _, path := range []string{"", "relative", "/callback?query", "/callback#fragment", "/callback\n"} {
		_, err := browser.Listen(t.Context(), path, "state")
		require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	}
	for _, target := range []string{"file:///etc/passwd", "javascript:alert(1)", "https://user:password@example.com"} {
		require.ErrorIs(t, browser.Open(t.Context(), target), errUtils.ErrPluginInvalidConfig)
	}
}

func TestBrowserCallbackReceiveCanRetry(t *testing.T) {
	t.Parallel()
	for _, timeout := range []bool{false, true} {
		t.Run(strconv.FormatBool(timeout), func(t *testing.T) {
			t.Parallel()
			listener, err := (LocalBrowser{Interactive: true}).Listen(t.Context(), "/callback", "expected")
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, listener.Close()) })
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			expected := context.Canceled
			if timeout {
				ctx, cancel = context.WithTimeout(t.Context(), time.Millisecond)
				defer cancel()
				expected = context.DeadlineExceeded
			}
			_, err = listener.Receive(ctx)
			require.ErrorIs(t, err, expected)
			response, err := http.Get(listener.RedirectURI() + "?state=expected&code=valid")
			require.NoError(t, err)
			require.NoError(t, response.Body.Close())
			ctx, cancel = context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			callback, err := listener.Receive(ctx)
			require.NoError(t, err)
			assert.Equal(t, Callback{Code: "valid", State: "expected"}, callback)
			_, err = listener.Receive(ctx)
			require.ErrorIs(t, err, io.EOF)
		})
	}
}

func TestBrowserCallbackConcurrentReceiversComplete(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	listener, err := (LocalBrowser{Interactive: true}).Listen(ctx, "/callback", "expected")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, listener.Close()) })
	const receivers = 16
	type result struct {
		callback Callback
		err      error
	}
	results := make(chan result, receivers)
	for range receivers {
		go func() {
			callback, err := listener.Receive(ctx)
			results <- result{callback, err}
		}()
	}
	response, err := http.Get(listener.RedirectURI() + "?state=expected&code=valid")
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	var delivered int
	for range receivers {
		result := <-results
		if result.err == nil {
			delivered++
			assert.Equal(t, Callback{Code: "valid", State: "expected"}, result.callback)
		} else {
			require.ErrorIs(t, result.err, io.EOF)
		}
	}
	assert.Equal(t, 1, delivered)
}

func TestBrowserOpenUsesConfiguredOpener(t *testing.T) {
	t.Parallel()
	called := false
	browser := LocalBrowser{Interactive: true, OpenURL: func(target string) error {
		called = true
		require.Equal(t, "https://issuer.example/authorize?state=expected", target)
		return io.ErrClosedPipe
	}}
	require.ErrorIs(t, browser.Open(t.Context(), "https://issuer.example/authorize?state=expected"), io.ErrClosedPipe)
	require.True(t, called)
	called = false
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, browser.Open(ctx, "https://issuer.example"), context.Canceled)
	require.False(t, called)
	listener, err := browser.Listen(ctx, "/callback", "state")
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, listener)
	browser.OpenURL = nil
	require.ErrorIs(t, browser.Open(t.Context(), "https://issuer.example"), errUtils.ErrPluginInteractionRequired)
}
