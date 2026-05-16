package verification

import (
	"context"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/toolchain/registry"
)

type fakeDownloader map[string][]byte

func (f fakeDownloader) Download(_ context.Context, url string) ([]byte, error) {
	data, ok := f[url]
	if !ok {
		return nil, fmt.Errorf("unexpected download URL %s", url)
	}
	return data, nil
}

func TestVerifyChecksumRaw(t *testing.T) {
	assetPath := writeAsset(t, []byte("hello"))
	sum := sha256.Sum256([]byte("hello"))

	verifier := &Verifier{}
	result, err := verifier.Verify(context.Background(), Request{
		Tool: &registry.Tool{
			RepoOwner: "owner",
			RepoName:  "tool",
			Checksum: registry.ChecksumConfig{
				Type:       "http",
				URL:        "https://example.com/tool.sha256",
				FileFormat: "raw",
				Algorithm:  "sha256",
			},
		},
		Version:   "1.0.0",
		AssetURL:  "https://example.com/tool.tar.gz",
		AssetPath: assetPath,
		Downloader: fakeDownloader{
			"https://example.com/tool.sha256": []byte(fmt.Sprintf("%x\n", sum)),
		},
	})

	require.NoError(t, err)
	assert.Equal(t, "sha256", result.ChecksumAlgorithm)
	assert.Equal(t, fmt.Sprintf("%x", sum), result.Checksum)
}

func TestVerifyChecksumRegexpWithTemplateAsset(t *testing.T) {
	assetPath := writeAsset(t, []byte("hello"))
	sum := sha256.Sum256([]byte("hello"))
	assetURL := "https://example.com/releases/tool_1.0.0_linux_amd64.tar.gz"

	_, err := (&Verifier{}).Verify(context.Background(), Request{
		Tool: &registry.Tool{
			RepoOwner: "owner",
			RepoName:  "tool",
			Checksum: registry.ChecksumConfig{
				Type:      "http",
				Asset:     "{{.AssetWithoutExt}}_checksums.txt",
				Algorithm: "sha256",
			},
		},
		Version:   "1.0.0",
		AssetURL:  assetURL,
		AssetPath: assetPath,
		Downloader: fakeDownloader{
			"tool_1.0.0_linux_amd64_checksums.txt": []byte(fmt.Sprintf("%x  tool_1.0.0_linux_amd64.tar.gz\n", sum)),
		},
	})

	require.NoError(t, err)
}

func TestVerifyChecksumAlgorithms(t *testing.T) {
	content := []byte("hello")
	tests := []struct {
		name      string
		algorithm string
		sum       string
	}{
		{name: "md5", algorithm: "md5", sum: fmt.Sprintf("%x", md5.Sum(content))},
		{name: "sha1", algorithm: "sha1", sum: fmt.Sprintf("%x", sha1.Sum(content))},
		{name: "sha256", algorithm: "sha256", sum: fmt.Sprintf("%x", sha256.Sum256(content))},
		{name: "sha512", algorithm: "sha512", sum: fmt.Sprintf("%x", sha512.Sum512(content))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := (&Verifier{}).Verify(context.Background(), Request{
				Tool: &registry.Tool{
					RepoOwner: "owner",
					RepoName:  "tool",
					Checksum: registry.ChecksumConfig{
						Type:       "http",
						URL:        "https://example.com/checksums.txt",
						FileFormat: "raw",
						Algorithm:  tt.algorithm,
					},
				},
				Version:   "1.0.0",
				AssetURL:  "https://example.com/tool",
				AssetPath: writeAsset(t, content),
				Downloader: fakeDownloader{
					"https://example.com/checksums.txt": []byte(tt.sum),
				},
			})
			require.NoError(t, err)
		})
	}
}

func TestVerifyChecksumMismatch(t *testing.T) {
	_, err := (&Verifier{}).Verify(context.Background(), Request{
		Tool: &registry.Tool{
			RepoOwner: "owner",
			RepoName:  "tool",
			Checksum: registry.ChecksumConfig{
				Type:       "http",
				URL:        "https://example.com/checksums.txt",
				FileFormat: "raw",
				Algorithm:  "sha256",
			},
		},
		Version:   "1.0.0",
		AssetURL:  "https://example.com/tool",
		AssetPath: writeAsset(t, []byte("hello")),
		Downloader: fakeDownloader{
			"https://example.com/checksums.txt": []byte("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
		},
	})

	require.ErrorIs(t, err, ErrChecksumMismatch)
}

func TestVerifyChecksumRequiredMissingMetadata(t *testing.T) {
	_, err := (&Verifier{}).Verify(context.Background(), Request{
		Tool:      &registry.Tool{RepoOwner: "owner", RepoName: "tool"},
		Version:   "1.0.0",
		AssetURL:  "https://example.com/tool",
		AssetPath: writeAsset(t, []byte("hello")),
		Policy: Policy{
			Checksums:  PolicyRequired,
			Signatures: PolicyDisabled,
		},
	})

	require.ErrorIs(t, err, ErrChecksumRequired)
}

func writeAsset(t *testing.T, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "asset.bin")
	require.NoError(t, os.WriteFile(path, data, 0o644))
	return path
}
