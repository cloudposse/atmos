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
	"regexp"
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

func TestVerifyChecksumSkipsUnavailableSidecarWhenAvailable(t *testing.T) {
	result, err := (&Verifier{}).Verify(context.Background(), Request{
		Tool: &registry.Tool{
			RepoOwner: "owner",
			RepoName:  "tool",
			Checksum: registry.ChecksumConfig{
				Type:       "http",
				URL:        "https://example.com/missing-checksums.txt",
				FileFormat: "raw",
				Algorithm:  "sha256",
			},
		},
		Version:   "1.0.0",
		AssetURL:  "https://example.com/tool",
		AssetPath: writeAsset(t, []byte("hello")),
		Downloader: fakeDownloader{
			"https://example.com/other.txt": []byte("not used"),
		},
		Policy: Policy{
			Checksums:  PolicyWhenAvailable,
			Signatures: PolicyDisabled,
		},
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Empty(t, result.Checksum)
	assert.Contains(t, result.SkippedReasons[0], "checksum sidecar unavailable")
}

func TestVerifyChecksumRequiredUnavailableSidecar(t *testing.T) {
	_, err := (&Verifier{}).Verify(context.Background(), Request{
		Tool: &registry.Tool{
			RepoOwner: "owner",
			RepoName:  "tool",
			Checksum: registry.ChecksumConfig{
				Type:       "http",
				URL:        "https://example.com/missing-checksums.txt",
				FileFormat: "raw",
				Algorithm:  "sha256",
			},
		},
		Version:   "1.0.0",
		AssetURL:  "https://example.com/tool",
		AssetPath: writeAsset(t, []byte("hello")),
		Downloader: fakeDownloader{
			"https://example.com/other.txt": []byte("not used"),
		},
		Policy: Policy{
			Checksums:  PolicyRequired,
			Signatures: PolicyDisabled,
		},
	})

	require.ErrorIs(t, err, ErrChecksumRequired)
}

func TestVerifyChecksumRequiredMissingAndDisabledMetadata(t *testing.T) {
	_, err := (&Verifier{}).Verify(context.Background(), Request{
		Tool:      &registry.Tool{RepoOwner: "owner", RepoName: "tool"},
		Version:   "1.0.0",
		AssetURL:  "https://example.com/tool",
		AssetPath: writeAsset(t, []byte("hello")),
		Policy:    Policy{Checksums: PolicyRequired, Signatures: PolicyDisabled},
	})
	require.ErrorIs(t, err, ErrChecksumRequired)

	disabled := false
	_, err = (&Verifier{}).Verify(context.Background(), Request{
		Tool: &registry.Tool{
			RepoOwner: "owner",
			RepoName:  "tool",
			Checksum: registry.ChecksumConfig{
				Enabled: &disabled,
				Type:    "http",
				URL:     "https://example.com/checksums.txt",
			},
		},
		Version:   "1.0.0",
		AssetURL:  "https://example.com/tool",
		AssetPath: writeAsset(t, []byte("hello")),
		Policy:    Policy{Checksums: PolicyRequired, Signatures: PolicyDisabled},
	})
	require.ErrorIs(t, err, ErrChecksumRequired)
}

func TestVerifyChecksumSkipsMissingAndDisabledWhenAvailable(t *testing.T) {
	result, err := (&Verifier{}).Verify(context.Background(), Request{
		Tool:      &registry.Tool{RepoOwner: "owner", RepoName: "tool"},
		Version:   "1.0.0",
		AssetURL:  "https://example.com/tool",
		AssetPath: writeAsset(t, []byte("hello")),
		Policy:    Policy{Checksums: PolicyWhenAvailable, Signatures: PolicyDisabled},
	})
	require.NoError(t, err)
	assert.Contains(t, result.SkippedReasons, "checksum metadata unavailable")

	disabled := false
	result, err = (&Verifier{}).Verify(context.Background(), Request{
		Tool: &registry.Tool{
			RepoOwner: "owner",
			RepoName:  "tool",
			Checksum:  registry.ChecksumConfig{Enabled: &disabled},
		},
		Version:   "1.0.0",
		AssetURL:  "https://example.com/tool",
		AssetPath: writeAsset(t, []byte("hello")),
		Policy:    Policy{Checksums: PolicyWhenAvailable, Signatures: PolicyDisabled},
	})
	require.NoError(t, err)
	assert.Contains(t, result.SkippedReasons, "checksum disabled by registry metadata")
}

func TestChecksumFileURLUsesGitHubReleaseVersionFromAssetURL(t *testing.T) {
	u, err := checksumFileURL(&registry.Tool{
		RepoOwner: "owner",
		RepoName:  "tool",
		Checksum: registry.ChecksumConfig{
			Type:  "github_release",
			Asset: "checksums.txt",
		},
	}, "1.0.0", "https://github.com/owner/tool/releases/download/v1.0.0/tool_1.0.0_linux_amd64.tar.gz", &registry.ChecksumConfig{
		Type:  "github_release",
		Asset: "checksums.txt",
	})

	require.NoError(t, err)
	assert.Equal(t, "https://github.com/owner/tool/releases/download/v1.0.0/checksums.txt", u)
}

func TestChecksumFileURLUsesHTTPVersionSegmentFromAssetURL(t *testing.T) {
	u, err := checksumFileURL(&registry.Tool{}, "1.31.4", "https://dl.k8s.io/v1.31.4/bin/darwin/arm64/kubectl", &registry.ChecksumConfig{
		Type: "http",
		URL:  "https://dl.k8s.io/{{.Version}}/bin/darwin/arm64/kubectl.sha256",
	})

	require.NoError(t, err)
	assert.Equal(t, "https://dl.k8s.io/v1.31.4/bin/darwin/arm64/kubectl.sha256", u)
}

func TestChecksumFileURLUsesEffectiveVersionInEmbeddedAssetName(t *testing.T) {
	u, err := checksumFileURL(&registry.Tool{}, "3.17.0", "https://get.helm.sh/helm-v3.17.0-darwin-arm64.tar.gz", &registry.ChecksumConfig{
		Type: "http",
		URL:  "https://get.helm.sh/helm-{{.Version}}-darwin-arm64.tar.gz.sha256sum",
	})

	require.NoError(t, err)
	assert.Equal(t, "https://get.helm.sh/helm-v3.17.0-darwin-arm64.tar.gz.sha256sum", u)
}

func TestChecksumFileURLBranchesAndErrors(t *testing.T) {
	_, err := checksumFileURL(&registry.Tool{}, "1.0.0", "https://example.com/tool", &registry.ChecksumConfig{
		Type: "http",
		URL:  "{{",
	})
	require.Error(t, err)

	_, err = checksumFileURL(&registry.Tool{}, "1.0.0", "https://example.com/tool", &registry.ChecksumConfig{
		Type:  "http",
		Asset: "{{",
	})
	require.Error(t, err)

	u, err := checksumFileURL(&registry.Tool{RepoOwner: "owner", RepoName: "tool"}, "1.0.0", "::::", &registry.ChecksumConfig{
		Type:  "github_release",
		Asset: "checksums.txt",
	})
	require.NoError(t, err)
	assert.Equal(t, "https://github.com/owner/tool/releases/download/1.0.0/checksums.txt", u)
}

func TestChecksumParsingEdgeCases(t *testing.T) {
	_, err := parseRawChecksum([]byte(""), "sha256")
	require.ErrorIs(t, err, ErrChecksumNotFound)

	_, err = parseRegexpChecksum([]byte("abc tool\nabc other\n"), "missing", "sha256", registry.ChecksumPattern{
		Checksum: "abc",
	})
	require.ErrorIs(t, err, ErrChecksumAmbiguous)

	sum, err := parseRegexpChecksum([]byte("abc artifact\n"), "artifact", "sha256", registry.ChecksumPattern{
		Checksum: "abc",
		File:     "artifact",
	})
	require.NoError(t, err)
	assert.Equal(t, "abc", sum)

	_, _, ok := matchChecksumLine("abc artifact", "missing", regexp.MustCompile("abc"), regexp.MustCompile("different"))
	assert.False(t, ok)
}

func TestDigestAndURLHelpers(t *testing.T) {
	_, err := digestFile(filepath.Join(t.TempDir(), "missing"), "sha256")
	require.Error(t, err)

	_, err = digestFile(writeAsset(t, []byte("hello")), "sha999")
	require.ErrorIs(t, err, ErrUnsupportedAlgorithm)

	assert.Equal(t, "", effectiveReleaseVersionFromAssetURL("://bad-url", "1.0.0"))
	assert.Equal(t, "", releaseVersionFromGitHubAssetURL("https://example.com/owner/tool/releases/download/v1.0.0/tool.tar.gz"))
	assert.Equal(t, "https://example.com/tool-1.0.0.sig", replaceVersionSegmentInURL("https://example.com/tool-1.0.0.sig", "1.0.0", "v1.0.0"))
	assert.Equal(t, "tool", assetNameFromURL("://bad-url/tool"))
}

func TestVerifyChecksumCosignVerifiesChecksumSidecar(t *testing.T) {
	assetPath := writeAsset(t, []byte("hello"))
	sum := sha256.Sum256([]byte("hello"))
	checksumData := []byte(fmt.Sprintf("%x  tool_1.0.0_linux_amd64.tar.gz\n", sum))
	runner := &fakeRunner{}

	result, err := (&Verifier{}).Verify(context.Background(), Request{
		Tool: &registry.Tool{
			RepoOwner: "owner",
			RepoName:  "tool",
			Checksum: registry.ChecksumConfig{
				Type:       "github_release",
				Asset:      "checksums.txt",
				FileFormat: "regexp",
				Algorithm:  "sha256",
				Cosign: registry.CosignConfig{
					Opts: []string{
						"--certificate",
						"https://github.com/owner/tool/releases/download/{{.Version}}/{{.Asset}}.pem",
						"--signature",
						"https://github.com/owner/tool/releases/download/{{.Version}}/{{.Asset}}.sig",
					},
				},
			},
		},
		Version:   "1.0.0",
		AssetURL:  "https://github.com/owner/tool/releases/download/v1.0.0/tool_1.0.0_linux_amd64.tar.gz",
		AssetPath: assetPath,
		Downloader: fakeDownloader{
			"https://github.com/owner/tool/releases/download/v1.0.0/checksums.txt": checksumData,
		},
		Policy: Policy{
			Checksums:  PolicyWhenAvailable,
			Signatures: PolicyWhenAvailable,
		},
		Runner: runner,
	})

	require.NoError(t, err)
	require.Len(t, runner.calls, 1)
	assert.Equal(t, "cosign", runner.calls[0].name)
	assert.Equal(t, []string{
		"verify-blob",
		"--certificate",
		"https://github.com/owner/tool/releases/download/v1.0.0/checksums.txt.pem",
		"--signature",
		"https://github.com/owner/tool/releases/download/v1.0.0/checksums.txt.sig",
		writeAssetPathPlaceholder(runner.calls[0].args),
	}, normalizeLastArg(runner.calls[0].args))
	assert.NotEqual(t, assetPath, runner.calls[0].args[len(runner.calls[0].args)-1])
	assert.Contains(t, result.SignatureMethods, "cosign")
	assert.Equal(t, "sha256", result.ChecksumAlgorithm)
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
