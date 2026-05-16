package verification

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/toolchain/registry"
)

type commandCall struct {
	name string
	args []string
}

type fakeRunner struct {
	calls []commandCall
}

func (r *fakeRunner) Run(_ context.Context, name string, args ...string) error {
	r.calls = append(r.calls, commandCall{name: name, args: append([]string(nil), args...)})
	return nil
}

func TestVerifyCosignCommandFromOpts(t *testing.T) {
	runner := &fakeRunner{}
	result, err := (&Verifier{}).Verify(context.Background(), Request{
		Tool: &registry.Tool{
			RepoOwner: "owner",
			RepoName:  "tool",
			Cosign: registry.CosignConfig{
				Opts: []string{
					"--signature",
					"https://example.com/{{.Asset}}.sig",
					"--certificate-identity",
					"https://github.com/owner/tool/.github/workflows/release.yaml@refs/tags/{{.Version}}",
				},
			},
		},
		Version:   "1.0.0",
		AssetURL:  "https://example.com/tool.tar.gz",
		AssetPath: writeAsset(t, []byte("hello")),
		Policy: Policy{
			Checksums:  PolicyDisabled,
			Signatures: PolicyWhenAvailable,
		},
		Runner: runner,
	})

	require.NoError(t, err)
	require.Len(t, runner.calls, 1)
	assert.Equal(t, "cosign", runner.calls[0].name)
	assert.Equal(t, []string{
		"verify-blob",
		"--signature",
		"https://example.com/tool.tar.gz.sig",
		"--certificate-identity",
		"https://github.com/owner/tool/.github/workflows/release.yaml@refs/tags/1.0.0",
		writeAssetPathPlaceholder(runner.calls[0].args),
	}, normalizeLastArg(runner.calls[0].args))
	assert.Contains(t, result.SignatureMethods, "cosign")
}

func TestVerifyCosignCommandFromOptsUsesEffectiveGitHubReleaseVersion(t *testing.T) {
	runner := &fakeRunner{}
	_, err := (&Verifier{}).Verify(context.Background(), Request{
		Tool: &registry.Tool{
			RepoOwner: "owner",
			RepoName:  "tool",
			Cosign: registry.CosignConfig{
				Opts: []string{
					"--certificate",
					"https://github.com/owner/tool/releases/download/{{.Version}}/tool_{{.Version}}_linux_amd64.tar.gz.pem",
					"--signature",
					"https://github.com/owner/tool/releases/download/{{.Version}}/tool_{{.Version}}_linux_amd64.tar.gz.sig",
				},
			},
		},
		Version:   "1.0.0",
		AssetURL:  "https://github.com/owner/tool/releases/download/v1.0.0/tool_1.0.0_linux_amd64.tar.gz",
		AssetPath: writeAsset(t, []byte("hello")),
		Policy: Policy{
			Checksums:  PolicyDisabled,
			Signatures: PolicyWhenAvailable,
		},
		Runner: runner,
	})

	require.NoError(t, err)
	require.Len(t, runner.calls, 1)
	assert.Equal(t, []string{
		"verify-blob",
		"--certificate",
		"https://github.com/owner/tool/releases/download/v1.0.0/tool_1.0.0_linux_amd64.tar.gz.pem",
		"--signature",
		"https://github.com/owner/tool/releases/download/v1.0.0/tool_1.0.0_linux_amd64.tar.gz.sig",
		writeAssetPathPlaceholder(runner.calls[0].args),
	}, normalizeLastArg(runner.calls[0].args))
}

func TestVerifyCosignCommandFromOptsUsesEffectiveHTTPVersionSegment(t *testing.T) {
	runner := &fakeRunner{}
	_, err := (&Verifier{}).Verify(context.Background(), Request{
		Tool: &registry.Tool{
			RepoOwner: "kubernetes",
			RepoName:  "kubectl",
			Cosign: registry.CosignConfig{
				Opts: []string{
					"--signature",
					"https://dl.k8s.io/{{.Version}}/bin/darwin/arm64/kubectl.sig",
					"--certificate",
					"https://dl.k8s.io/{{.Version}}/bin/darwin/arm64/kubectl.cert",
				},
			},
		},
		Version:   "1.31.4",
		AssetURL:  "https://dl.k8s.io/v1.31.4/bin/darwin/arm64/kubectl",
		AssetPath: writeAsset(t, []byte("hello")),
		Policy: Policy{
			Checksums:  PolicyDisabled,
			Signatures: PolicyWhenAvailable,
		},
		Runner: runner,
	})

	require.NoError(t, err)
	require.Len(t, runner.calls, 1)
	assert.Equal(t, []string{
		"verify-blob",
		"--signature",
		"https://dl.k8s.io/v1.31.4/bin/darwin/arm64/kubectl.sig",
		"--certificate",
		"https://dl.k8s.io/v1.31.4/bin/darwin/arm64/kubectl.cert",
		writeAssetPathPlaceholder(runner.calls[0].args),
	}, normalizeLastArg(runner.calls[0].args))
}

func TestVerifySignatureCommands(t *testing.T) {
	tests := []struct {
		name        string
		tool        *registry.Tool
		commandName string
		method      string
	}{
		{
			name: "slsa",
			tool: &registry.Tool{
				RepoOwner: "owner",
				RepoName:  "tool",
				SLSAProvenance: registry.SLSAProvenance{
					Type:      "http",
					URL:       "https://example.com/provenance.intoto.jsonl",
					SourceURI: "github.com/owner/tool",
					SourceTag: "refs/tags/1.0.0",
				},
			},
			commandName: "slsa-verifier",
			method:      "slsa_provenance",
		},
		{
			name: "github attestations",
			tool: &registry.Tool{
				RepoOwner: "owner",
				RepoName:  "tool",
				GitHubArtifactAttestations: registry.GitHubArtifactAttestations{
					SignerWorkflow: "owner/tool/.github/workflows/release.yaml",
				},
			},
			commandName: "gh",
			method:      "github_artifact_attestations",
		},
		{
			name: "minisign",
			tool: &registry.Tool{
				RepoOwner: "owner",
				RepoName:  "tool",
				Minisign: registry.MinisignConfig{
					Type:      "http",
					URL:       "https://example.com/tool.minisig",
					PublicKey: "RWTOKEN",
				},
			},
			commandName: "minisign",
			method:      "minisign",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := &fakeRunner{}
			result, err := (&Verifier{}).Verify(context.Background(), Request{
				Tool:      tt.tool,
				Version:   "1.0.0",
				AssetURL:  "https://example.com/tool.tar.gz",
				AssetPath: writeAsset(t, []byte("hello")),
				Policy: Policy{
					Checksums:  PolicyDisabled,
					Signatures: PolicyWhenAvailable,
				},
				Runner: runner,
			})
			require.NoError(t, err)
			require.Len(t, runner.calls, 1)
			assert.Equal(t, tt.commandName, runner.calls[0].name)
			assert.Contains(t, result.SignatureMethods, tt.method)
		})
	}
}

func TestVerifySignatureRequiredMissingMetadata(t *testing.T) {
	_, err := (&Verifier{}).Verify(context.Background(), Request{
		Tool:      &registry.Tool{RepoOwner: "owner", RepoName: "tool"},
		Version:   "1.0.0",
		AssetURL:  "https://example.com/tool",
		AssetPath: writeAsset(t, []byte("hello")),
		Policy: Policy{
			Checksums:  PolicyDisabled,
			Signatures: PolicyRequired,
		},
		Runner: &fakeRunner{},
	})

	require.ErrorIs(t, err, ErrSignatureRequired)
}

func TestVerifyCosignSkipsUnavailableSidecarWhenAvailable(t *testing.T) {
	runner := &fakeRunner{}
	result, err := (&Verifier{}).Verify(context.Background(), Request{
		Tool: &registry.Tool{
			RepoOwner: "owner",
			RepoName:  "tool",
			Cosign: registry.CosignConfig{
				Signature: registry.DownloadedFile{
					Type:  "http",
					URL:   "https://example.com/missing.sig",
					Asset: "missing.sig",
				},
			},
		},
		Version:   "1.0.0",
		AssetURL:  "https://example.com/tool.tar.gz",
		AssetPath: writeAsset(t, []byte("hello")),
		Downloader: fakeDownloader{
			"https://example.com/other.sig": []byte("not used"),
		},
		Policy: Policy{
			Checksums:  PolicyDisabled,
			Signatures: PolicyWhenAvailable,
		},
		Runner: runner,
	})

	require.NoError(t, err)
	require.Empty(t, runner.calls)
	assert.Contains(t, result.SkippedReasons, "checksum verification disabled")
	require.Len(t, result.SkippedReasons, 2)
	assert.Contains(t, result.SkippedReasons[1], "cosign sidecar unavailable")
}

func TestVerifyCosignRequiredUnavailableSidecar(t *testing.T) {
	_, err := (&Verifier{}).Verify(context.Background(), Request{
		Tool: &registry.Tool{
			RepoOwner: "owner",
			RepoName:  "tool",
			Cosign: registry.CosignConfig{
				Signature: registry.DownloadedFile{
					Type:  "http",
					URL:   "https://example.com/missing.sig",
					Asset: "missing.sig",
				},
			},
		},
		Version:   "1.0.0",
		AssetURL:  "https://example.com/tool.tar.gz",
		AssetPath: writeAsset(t, []byte("hello")),
		Downloader: fakeDownloader{
			"https://example.com/other.sig": []byte("not used"),
		},
		Policy: Policy{
			Checksums:  PolicyDisabled,
			Signatures: PolicyRequired,
		},
		Runner: &fakeRunner{},
	})

	require.ErrorIs(t, err, ErrSignatureRequired)
}

func TestVerifyCosignDownloadsSidecars(t *testing.T) {
	runner := &fakeRunner{}
	result, err := (&Verifier{}).Verify(context.Background(), Request{
		Tool: &registry.Tool{
			RepoOwner: "owner",
			RepoName:  "tool",
			Cosign: registry.CosignConfig{
				Signature:   registry.DownloadedFile{Type: "http", URL: "https://example.com/tool.sig"},
				Certificate: registry.DownloadedFile{Type: "http", URL: "https://example.com/tool.pem"},
				Key:         registry.DownloadedFile{Type: "http", URL: "https://example.com/tool.pub"},
				Bundle:      registry.DownloadedFile{Type: "http", URL: "https://example.com/tool.bundle"},
			},
		},
		Version:   "1.0.0",
		AssetURL:  "https://example.com/tool.tar.gz",
		AssetPath: writeAsset(t, []byte("hello")),
		Downloader: fakeDownloader{
			"https://example.com/tool.sig":    []byte("sig"),
			"https://example.com/tool.pem":    []byte("pem"),
			"https://example.com/tool.pub":    []byte("pub"),
			"https://example.com/tool.bundle": []byte("bundle"),
		},
		Policy: Policy{
			Checksums:  PolicyDisabled,
			Signatures: PolicyWhenAvailable,
		},
		Runner: runner,
	})

	require.NoError(t, err)
	require.Len(t, runner.calls, 1)
	assert.Equal(t, "cosign", runner.calls[0].name)
	assert.Contains(t, runner.calls[0].args, "--signature")
	assert.Contains(t, runner.calls[0].args, "--certificate")
	assert.Contains(t, runner.calls[0].args, "--key")
	assert.Contains(t, runner.calls[0].args, "--bundle")
	assert.Contains(t, result.SignatureMethods, "cosign")
}

func TestVerifySignaturesDisabledByMetadata(t *testing.T) {
	disabled := false
	runner := &fakeRunner{}
	result, err := (&Verifier{}).Verify(context.Background(), Request{
		Tool: &registry.Tool{
			RepoOwner: "owner",
			RepoName:  "tool",
			Cosign:    registry.CosignConfig{Enabled: &disabled, Opts: []string{"--signature", "sig"}},
			SLSAProvenance: registry.SLSAProvenance{
				Enabled: &disabled,
				URL:     "https://example.com/provenance",
			},
			Minisign: registry.MinisignConfig{
				Enabled: &disabled,
				URL:     "https://example.com/tool.minisig",
			},
			GitHubArtifactAttestations: registry.GitHubArtifactAttestations{
				Enabled:        &disabled,
				SignerWorkflow: "release.yaml",
			},
		},
		Version:   "1.0.0",
		AssetURL:  "https://example.com/tool.tar.gz",
		AssetPath: writeAsset(t, []byte("hello")),
		Policy:    Policy{Checksums: PolicyDisabled, Signatures: PolicyWhenAvailable},
		Runner:    runner,
	})

	require.NoError(t, err)
	assert.Empty(t, runner.calls)
	assert.Empty(t, result.SignatureMethods)
}

func TestSidecarURLUsesGitHubReleaseVersionFromAssetURL(t *testing.T) {
	u, err := sidecarURL(&registry.Tool{
		RepoOwner: "owner",
		RepoName:  "tool",
	}, "1.0.0", "https://github.com/owner/tool/releases/download/v1.0.0/tool_1.0.0_linux_amd64.tar.gz", &registry.DownloadedFile{
		Type:  "github_release",
		Asset: "tool_1.0.0_linux_amd64.tar.gz.sig",
	})

	require.NoError(t, err)
	assert.Equal(t, "https://github.com/owner/tool/releases/download/v1.0.0/tool_1.0.0_linux_amd64.tar.gz.sig", u)
}

func TestSidecarURLBranchesAndErrors(t *testing.T) {
	_, err := sidecarURL(&registry.Tool{}, "1.0.0", "https://example.com/tool", &registry.DownloadedFile{
		Type: "http",
		URL:  "{{",
	})
	require.Error(t, err)

	_, err = sidecarURL(&registry.Tool{}, "1.0.0", "https://example.com/tool", &registry.DownloadedFile{
		Type:  "http",
		Asset: "{{",
	})
	require.Error(t, err)

	u, err := sidecarURL(&registry.Tool{RepoOwner: "owner", RepoName: "tool"}, "1.0.0", "https://example.com/v1.0.0/tool.tar.gz", &registry.DownloadedFile{
		Type: "http",
		URL:  "https://example.com/{{.Version}}/tool.sig",
	})
	require.NoError(t, err)
	assert.Equal(t, "https://example.com/v1.0.0/tool.sig", u)

	u, err = sidecarURL(&registry.Tool{RepoOwner: "owner", RepoName: "tool"}, "1.0.0", "https://example.com/tool.tar.gz", &registry.DownloadedFile{
		Type:      "github_release",
		RepoOwner: "other",
		RepoName:  "repo",
		Asset:     "tool.sig",
	})
	require.NoError(t, err)
	assert.Equal(t, "https://github.com/other/repo/releases/download/1.0.0/tool.sig", u)
}

func TestDownloadTempSidecar(t *testing.T) {
	path, err := (&Verifier{}).downloadTempSidecar(context.Background(), &Request{
		Downloader: fakeDownloader{"https://example.com/tool.sig": []byte("sig")},
	}, "https://example.com/tool.sig")
	require.NoError(t, err)
	defer os.Remove(path)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, []byte("sig"), data)
}

func normalizeLastArg(args []string) []string {
	out := append([]string(nil), args...)
	if len(out) > 0 {
		out[len(out)-1] = writeAssetPathPlaceholder(args)
	}
	return out
}

func writeAssetPathPlaceholder(_ []string) string {
	return "<asset>"
}
