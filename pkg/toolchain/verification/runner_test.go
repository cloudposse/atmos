package verification

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHTTPDownloaderDownloadSuccess(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("checksum"))
	}))
	defer ts.Close()

	body, err := (HTTPDownloader{}).Download(context.Background(), ts.URL+"/checksums.txt")

	require.NoError(t, err)
	assert.Equal(t, []byte("checksum"), body)
}

func TestHTTPDownloaderDownloadErrors(t *testing.T) {
	t.Run("bad url", func(t *testing.T) {
		_, err := (HTTPDownloader{}).Download(context.Background(), "://bad-url")
		require.ErrorIs(t, err, ErrDownloadFailed)
	})

	t.Run("non ok status", func(t *testing.T) {
		ts := httptest.NewServer(http.NotFoundHandler())
		defer ts.Close()

		_, err := (HTTPDownloader{}).Download(context.Background(), ts.URL)
		require.ErrorIs(t, err, ErrDownloadFailed)
	})

	t.Run("client error", func(t *testing.T) {
		client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("network down")
		})}

		_, err := (HTTPDownloader{Client: client}).Download(context.Background(), "https://example.com/checksums.txt")
		require.ErrorIs(t, err, ErrDownloadFailed)
	})

	t.Run("read error", func(t *testing.T) {
		client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       errReadCloser{},
			}, nil
		})}

		_, err := (HTTPDownloader{Client: client}).Download(context.Background(), "https://example.com/checksums.txt")
		require.ErrorIs(t, err, ErrDownloadFailed)
	})
}

func TestExecRunnerMissingCommand(t *testing.T) {
	err := ExecRunner{}.Run(context.Background(), "definitely-not-an-atmos-verifier")
	require.ErrorIs(t, err, ErrVerifierCommandRequired)
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type errReadCloser struct{}

func (errReadCloser) Read([]byte) (int, error) {
	return 0, io.ErrUnexpectedEOF
}

func (errReadCloser) Close() error {
	return nil
}
