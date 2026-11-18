package checksum

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sha256Hex is a test helper that hashes input and returns the lowercase hex digest.
func sha256Hex(t *testing.T, b []byte) string {
	t.Helper()
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func TestCompute(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		wantSize int64
	}{
		{name: "empty input", input: []byte{}, wantSize: 0},
		{name: "one byte", input: []byte{0x42}, wantSize: 1},
		{name: "1 KiB", input: make([]byte, 1024), wantSize: 1024},
		{name: "non-zero pattern", input: []byte("hello world"), wantSize: 11},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotHash, gotSize, err := Compute(strings.NewReader(string(tc.input)))
			require.NoError(t, err)
			assert.Equal(t, sha256Hex(t, tc.input), gotHash)
			assert.Equal(t, tc.wantSize, gotSize)
		})
	}
}

// errReader is an io.Reader that always returns the configured error after returning n bytes.
type errReader struct {
	n   int
	err error
}

func (r *errReader) Read(p []byte) (int, error) {
	if r.n > 0 {
		nb := len(p)
		if nb > r.n {
			nb = r.n
		}
		for i := 0; i < nb; i++ {
			p[i] = 0
		}
		r.n -= nb
		return nb, nil
	}
	return 0, r.err
}

func TestCompute_ReaderError(t *testing.T) {
	sentinel := errors.New("read failure")
	_, _, err := Compute(&errReader{n: 0, err: sentinel})
	require.Error(t, err)
	assert.ErrorIs(t, err, sentinel, "underlying read error must propagate")
}

func TestCompute_PartialReadCountsInSize(t *testing.T) {
	// Reader returns 5 bytes successfully, then fails. The byte count from a partial read
	// matters for diagnostics — assert it's not silently zeroed.
	sentinel := errors.New("eof-ish")
	_, size, err := Compute(&errReader{n: 5, err: sentinel})
	require.Error(t, err)
	assert.Equal(t, int64(5), size, "partial-read byte count must be preserved")
}

func TestFetchFromChecksumFile_HappyPath(t *testing.T) {
	expected := sha256Hex(t, []byte("kubectl-bytes"))
	body := expected + "  kubectl_linux_amd64.tar.gz\n"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, body)
	}))
	defer srv.Close()

	got, err := FetchFromChecksumFile(context.Background(), srv.Client(), srv.URL, "kubectl_linux_amd64.tar.gz")
	require.NoError(t, err)
	assert.Equal(t, expected, got)
}

func TestFetchFromChecksumFile_AssetMissing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Manifest parses, but doesn't include kubectl_darwin_arm64.tar.gz.
		_, _ = io.WriteString(w, sha256Hex(t, []byte("linux-only"))+"  kubectl_linux_amd64.tar.gz\n")
	}))
	defer srv.Close()

	_, err := FetchFromChecksumFile(context.Background(), srv.Client(), srv.URL, "kubectl_darwin_arm64.tar.gz")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrAssetNotInChecksumFile)
}

func TestFetchFromChecksumFile_HTTP404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusNotFound)
	}))
	defer srv.Close()

	_, err := FetchFromChecksumFile(context.Background(), srv.Client(), srv.URL, "anything")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrChecksumUnavailable)
}

func TestFetchFromChecksumFile_MalformedManifest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "this is not a checksum manifest\n")
	}))
	defer srv.Close()

	_, err := FetchFromChecksumFile(context.Background(), srv.Client(), srv.URL, "anything")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrChecksumUnavailable)
	// Underlying cause should be detectable too.
	assert.ErrorIs(t, err, ErrEmptyChecksumFile)
}

func TestFetchFromChecksumFile_TooLarge(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Write 2 MiB to bust the 1 MiB cap.
		big := make([]byte, 2<<20)
		_, _ = w.Write(big)
	}))
	defer srv.Close()

	_, err := FetchFromChecksumFile(context.Background(), srv.Client(), srv.URL, "anything")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrChecksumUnavailable)
	assert.ErrorIs(t, err, ErrChecksumFileTooLarge)
}

func TestFetchFromChecksumFile_ContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Block until the client disconnects.
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Immediate cancel.

	_, err := FetchFromChecksumFile(ctx, srv.Client(), srv.URL, "anything")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrChecksumUnavailable)
}

func TestFetchByDownload_HappyPath(t *testing.T) {
	payload := []byte("kubectl-binary-payload")
	expected := sha256Hex(t, payload)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	gotHash, gotSize, err := FetchByDownload(context.Background(), srv.Client(), srv.URL)
	require.NoError(t, err)
	assert.Equal(t, expected, gotHash)
	assert.Equal(t, int64(len(payload)), gotSize)
}

func TestFetchByDownload_HTTP404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusNotFound)
	}))
	defer srv.Close()

	_, _, err := FetchByDownload(context.Background(), srv.Client(), srv.URL)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrChecksumUnavailable)
}

func TestFetchByDownload_NetworkError(t *testing.T) {
	// Server that hangs up immediately — produces a transport error, not an HTTP status.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hj, ok := w.(http.Hijacker)
		require.True(t, ok, "test server must support hijacking")
		conn, _, _ := hj.Hijack()
		conn.Close()
	}))
	defer srv.Close()

	_, _, err := FetchByDownload(context.Background(), srv.Client(), srv.URL)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrChecksumUnavailable)
}

func TestFetchByDownload_InvalidURL(t *testing.T) {
	_, _, err := FetchByDownload(context.Background(), http.DefaultClient, "://not a url")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrChecksumUnavailable)
}

func TestFetchFromChecksumFile_InvalidURL(t *testing.T) {
	_, err := FetchFromChecksumFile(context.Background(), http.DefaultClient, "://not a url", "anything")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrChecksumUnavailable)
}

// TestFetchByDownload_LargePayload exercises the streaming SHA path with a multi-MiB body so
// we'd catch a regression that buffered the whole asset before hashing.
func TestFetchByDownload_LargePayload(t *testing.T) {
	const size = 4 << 20 // 4 MiB.
	payload := make([]byte, size)
	for i := range payload {
		payload[i] = byte(i % 251)
	}
	expected := sha256Hex(t, payload)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	gotHash, gotSize, err := FetchByDownload(context.Background(), srv.Client(), srv.URL)
	require.NoError(t, err)
	assert.Equal(t, expected, gotHash)
	assert.Equal(t, int64(size), gotSize)
}

// TestFetchFromChecksumFile_PositivePrerequisite is the negative-path pair to the various failure
// tests above: prove that when all preconditions are met, the function returns success. Without
// this, a refactor that always-fails would still pass the failure tests.
func TestFetchFromChecksumFile_PositivePrerequisite(t *testing.T) {
	expected := sha256Hex(t, []byte("test"))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s  asset.tar.gz\n", expected)
	}))
	defer srv.Close()

	got, err := FetchFromChecksumFile(context.Background(), srv.Client(), srv.URL, "asset.tar.gz")
	require.NoError(t, err)
	assert.Equal(t, expected, got)
}
