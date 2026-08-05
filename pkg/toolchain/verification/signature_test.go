package verification

import (
	"context"
	"fmt"
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
	err   error
}

func (r *fakeRunner) Run(_ context.Context, name string, args ...string) error {
	r.calls = append(r.calls, commandCall{name: name, args: append([]string(nil), args...)})
	return r.err
}

func TestVerifyCosignCommandFromOpts(t *testing.T) {
	runner := &fakeRunner{}
	assetPath := writeAsset(t, []byte("hello"))
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
		AssetPath: assetPath,
		Downloader: fakeDownloader{
			"https://example.com/tool.tar.gz.sig": []byte("sig"),
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
	assert.Equal(t, []string{"verify-blob", "--signature", "--certificate-identity"}, []string{runner.calls[0].args[0], runner.calls[0].args[1], runner.calls[0].args[3]})
	assert.NotEqual(t, "https://example.com/tool.tar.gz.sig", runner.calls[0].args[2])
	assert.Equal(t, "https://github.com/owner/tool/.github/workflows/release.yaml@refs/tags/1.0.0", runner.calls[0].args[4])
	assert.Equal(t, assetPath, runner.calls[0].args[len(runner.calls[0].args)-1])
	assert.Contains(t, result.SignatureMethods, "cosign")
}

func TestVerifyCosignCommandFromOptsUsesEffectiveGitHubReleaseVersion(t *testing.T) {
	runner := &fakeRunner{}
	assetPath := writeAsset(t, []byte("hello"))
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
		AssetPath: assetPath,
		Policy: Policy{
			Checksums:  PolicyDisabled,
			Signatures: PolicyWhenAvailable,
		},
		Downloader: fakeDownloader{
			"https://github.com/owner/tool/releases/download/v1.0.0/tool_1.0.0_linux_amd64.tar.gz.pem": []byte("pem"),
			"https://github.com/owner/tool/releases/download/v1.0.0/tool_1.0.0_linux_amd64.tar.gz.sig": []byte("sig"),
		},
		Runner: runner,
	})

	require.NoError(t, err)
	require.Len(t, runner.calls, 1)
	assert.Equal(t, []string{"verify-blob", "--certificate", "--signature"}, []string{runner.calls[0].args[0], runner.calls[0].args[1], runner.calls[0].args[3]})
	assert.NotEqual(t, "https://github.com/owner/tool/releases/download/v1.0.0/tool_1.0.0_linux_amd64.tar.gz.pem", runner.calls[0].args[2])
	assert.NotEqual(t, "https://github.com/owner/tool/releases/download/v1.0.0/tool_1.0.0_linux_amd64.tar.gz.sig", runner.calls[0].args[4])
	assert.Equal(t, assetPath, runner.calls[0].args[len(runner.calls[0].args)-1])
}

func TestVerifyCosignCommandFromOptsUsesEffectiveHTTPVersionSegment(t *testing.T) {
	runner := &fakeRunner{}
	assetPath := writeAsset(t, []byte("hello"))
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
		AssetPath: assetPath,
		Policy: Policy{
			Checksums:  PolicyDisabled,
			Signatures: PolicyWhenAvailable,
		},
		Downloader: fakeDownloader{
			"https://dl.k8s.io/v1.31.4/bin/darwin/arm64/kubectl.sig":  []byte("sig"),
			"https://dl.k8s.io/v1.31.4/bin/darwin/arm64/kubectl.cert": []byte("cert"),
		},
		Runner: runner,
	})

	require.NoError(t, err)
	require.Len(t, runner.calls, 1)
	assert.Equal(t, []string{"verify-blob", "--signature", "--certificate"}, []string{runner.calls[0].args[0], runner.calls[0].args[1], runner.calls[0].args[3]})
	assert.NotEqual(t, "https://dl.k8s.io/v1.31.4/bin/darwin/arm64/kubectl.sig", runner.calls[0].args[2])
	assert.NotEqual(t, "https://dl.k8s.io/v1.31.4/bin/darwin/arm64/kubectl.cert", runner.calls[0].args[4])
	assert.Equal(t, assetPath, runner.calls[0].args[len(runner.calls[0].args)-1])
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
			downloader := fakeDownloader{
				"https://example.com/provenance.intoto.jsonl": []byte("provenance"),
				"https://example.com/tool.minisig":            []byte("minisig"),
			}
			result, err := (&Verifier{}).Verify(context.Background(), Request{
				Tool:       tt.tool,
				Version:    "1.0.0",
				AssetURL:   "https://example.com/tool.tar.gz",
				AssetPath:  writeAsset(t, []byte("hello")),
				Downloader: downloader,
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
	assert.True(t, hasSkipReasonContaining(result.SkippedReasons, "cosign sidecar unavailable"))
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

func TestVerifyCosignOptsUnavailableSidecarUsesPolicy(t *testing.T) {
	tests := []struct {
		name      string
		policy    string
		wantError error
		wantSkip  bool
	}{
		{name: "when available skips", policy: PolicyWhenAvailable, wantSkip: true},
		{name: "required returns classified error", policy: PolicyRequired, wantError: ErrSignatureRequired},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := &fakeRunner{}
			result, err := (&Verifier{}).Verify(context.Background(), Request{
				Tool: &registry.Tool{
					RepoOwner: "owner",
					RepoName:  "tool",
					Cosign: registry.CosignConfig{
						Opts: []string{"--bundle", "https://example.com/missing.bundle"},
					},
				},
				Version:   "1.0.0",
				AssetURL:  "https://example.com/tool.tar.gz",
				AssetPath: writeAsset(t, []byte("hello")),
				Downloader: fakeDownloader{
					"https://example.com/other.bundle": []byte("not used"),
				},
				Policy: Policy{Checksums: PolicyDisabled, Signatures: tt.policy},
				Runner: runner,
			})

			if tt.wantError != nil {
				require.ErrorIs(t, err, tt.wantError)
			} else {
				require.NoError(t, err)
			}
			assert.Empty(t, runner.calls)
			if tt.wantSkip {
				assert.True(t, hasSkipReasonContaining(result.SkippedReasons, "cosign sidecar unavailable"))
			}
		})
	}
}

func TestVerifyGitHubAttestationSkipsHTTP404WhenAvailable(t *testing.T) {
	runner := &fakeRunner{
		err: fmt.Errorf("%w: gh [attestation verify asset --repo terraform-linters/tflint]: exit status 1\n"+
			"Error: HTTP 404: Not Found (https://api.github.com/repos/terraform-linters/tflint/attestations/sha256:abc)",
			ErrSignatureFailed),
	}
	result, err := (&Verifier{}).Verify(context.Background(), Request{
		Tool: &registry.Tool{
			RepoOwner: "terraform-linters",
			RepoName:  "tflint",
			GitHubArtifactAttestations: registry.GitHubArtifactAttestations{
				SignerWorkflow: "terraform-linters/tflint/.github/workflows/release.yml",
				PredicateType:  "https://slsa.dev/provenance/v1",
			},
		},
		Version:   "0.58.1",
		AssetURL:  "https://github.com/terraform-linters/tflint/releases/download/v0.58.1/tflint_darwin_arm64.zip",
		AssetPath: writeAsset(t, []byte("hello")),
		Policy: Policy{
			Checksums:  PolicyDisabled,
			Signatures: PolicyWhenAvailable,
		},
		Runner: runner,
	})

	require.NoError(t, err)
	require.Len(t, runner.calls, 1)
	assert.Empty(t, result.SignatureMethods)
	assert.True(t, hasSkipReasonContaining(result.SkippedReasons, "github artifact attestations unavailable"))
}

func TestVerifyGitHubAttestationRequiredFailsHTTP404(t *testing.T) {
	_, err := (&Verifier{}).Verify(context.Background(), Request{
		Tool: &registry.Tool{
			RepoOwner: "terraform-linters",
			RepoName:  "tflint",
			GitHubArtifactAttestations: registry.GitHubArtifactAttestations{
				SignerWorkflow: "terraform-linters/tflint/.github/workflows/release.yml",
			},
		},
		Version:   "0.58.1",
		AssetURL:  "https://github.com/terraform-linters/tflint/releases/download/v0.58.1/tflint_darwin_arm64.zip",
		AssetPath: writeAsset(t, []byte("hello")),
		Policy: Policy{
			Checksums:  PolicyDisabled,
			Signatures: PolicyRequired,
		},
		Runner: &fakeRunner{
			err: fmt.Errorf("%w: Error: HTTP 404: Not Found", ErrSignatureFailed),
		},
	})

	require.ErrorIs(t, err, ErrSignatureRequired)
}

func TestVerifySLSASkipsUnavailableSidecarWhenAvailable(t *testing.T) {
	runner := &fakeRunner{}
	result, err := (&Verifier{}).Verify(context.Background(), Request{
		Tool: &registry.Tool{
			RepoOwner: "owner",
			RepoName:  "tool",
			SLSAProvenance: registry.SLSAProvenance{
				Type: "http",
				URL:  "https://example.com/missing.intoto.jsonl",
			},
		},
		Version:   "1.0.0",
		AssetURL:  "https://example.com/tool.tar.gz",
		AssetPath: writeAsset(t, []byte("hello")),
		Downloader: fakeDownloader{
			"https://example.com/other.intoto.jsonl": []byte("not used"),
		},
		Policy: Policy{
			Checksums:  PolicyDisabled,
			Signatures: PolicyWhenAvailable,
		},
		Runner: runner,
	})

	require.NoError(t, err)
	assert.Empty(t, runner.calls)
	assert.True(t, hasSkipReasonContaining(result.SkippedReasons, "slsa provenance unavailable"))
}

func TestVerifySLSARequiredUnavailableSidecar(t *testing.T) {
	_, err := (&Verifier{}).Verify(context.Background(), Request{
		Tool: &registry.Tool{
			RepoOwner: "owner",
			RepoName:  "tool",
			SLSAProvenance: registry.SLSAProvenance{
				Type: "http",
				URL:  "https://example.com/missing.intoto.jsonl",
			},
		},
		Version:   "1.0.0",
		AssetURL:  "https://example.com/tool.tar.gz",
		AssetPath: writeAsset(t, []byte("hello")),
		Downloader: fakeDownloader{
			"https://example.com/other.intoto.jsonl": []byte("not used"),
		},
		Policy: Policy{
			Checksums:  PolicyDisabled,
			Signatures: PolicyRequired,
		},
		Runner: &fakeRunner{},
	})

	require.ErrorIs(t, err, ErrSignatureRequired)
}

func TestVerifyMinisignSkipsUnavailableSidecarWhenAvailable(t *testing.T) {
	runner := &fakeRunner{}
	result, err := (&Verifier{}).Verify(context.Background(), Request{
		Tool: &registry.Tool{
			RepoOwner: "owner",
			RepoName:  "tool",
			Minisign: registry.MinisignConfig{
				Type: "http",
				URL:  "https://example.com/missing.minisig",
			},
		},
		Version:   "1.0.0",
		AssetURL:  "https://example.com/tool.tar.gz",
		AssetPath: writeAsset(t, []byte("hello")),
		Downloader: fakeDownloader{
			"https://example.com/other.minisig": []byte("not used"),
		},
		Policy: Policy{
			Checksums:  PolicyDisabled,
			Signatures: PolicyWhenAvailable,
		},
		Runner: runner,
	})

	require.NoError(t, err)
	assert.Empty(t, runner.calls)
	assert.True(t, hasSkipReasonContaining(result.SkippedReasons, "minisign signature unavailable"))
}

func TestVerifyMinisignRequiredUnavailableSidecar(t *testing.T) {
	_, err := (&Verifier{}).Verify(context.Background(), Request{
		Tool: &registry.Tool{
			RepoOwner: "owner",
			RepoName:  "tool",
			Minisign: registry.MinisignConfig{
				Type: "http",
				URL:  "https://example.com/missing.minisig",
			},
		},
		Version:   "1.0.0",
		AssetURL:  "https://example.com/tool.tar.gz",
		AssetPath: writeAsset(t, []byte("hello")),
		Downloader: fakeDownloader{
			"https://example.com/other.minisig": []byte("not used"),
		},
		Policy: Policy{
			Checksums:  PolicyDisabled,
			Signatures: PolicyRequired,
		},
		Runner: &fakeRunner{},
	})

	require.ErrorIs(t, err, ErrSignatureRequired)
}

func TestVerifyCosignOptsSkipsHTTP404WhenAvailable(t *testing.T) {
	runner := &fakeRunner{
		err: fmt.Errorf("%w: Error: HTTP 404: Not Found (https://example.com/tool.sig)", ErrSignatureFailed),
	}
	result, err := (&Verifier{}).Verify(context.Background(), Request{
		Tool: &registry.Tool{
			RepoOwner: "owner",
			RepoName:  "tool",
			Cosign: registry.CosignConfig{
				Opts: []string{"--signature", "https://example.com/{{.Asset}}.sig"},
			},
		},
		Version:   "1.0.0",
		AssetURL:  "https://example.com/tool.tar.gz",
		AssetPath: writeAsset(t, []byte("hello")),
		Downloader: fakeDownloader{
			"https://example.com/tool.tar.gz.sig": []byte("sig"),
		},
		Policy: Policy{
			Checksums:  PolicyDisabled,
			Signatures: PolicyWhenAvailable,
		},
		Runner: runner,
	})

	require.NoError(t, err)
	require.Len(t, runner.calls, 1)
	assert.Empty(t, result.SignatureMethods)
	assert.True(t, hasSkipReasonContaining(result.SkippedReasons, "cosign unavailable"))
}

func TestVerifySignatureInvalidEvidenceStillFailsWhenAvailable(t *testing.T) {
	_, err := (&Verifier{}).Verify(context.Background(), Request{
		Tool: &registry.Tool{
			RepoOwner: "owner",
			RepoName:  "tool",
			Cosign: registry.CosignConfig{
				Opts: []string{"--signature", "https://example.com/tool.sig"},
			},
		},
		Version:   "1.0.0",
		AssetURL:  "https://example.com/tool.tar.gz",
		AssetPath: writeAsset(t, []byte("hello")),
		Downloader: fakeDownloader{
			"https://example.com/tool.sig": []byte("sig"),
		},
		Policy: Policy{
			Checksums:  PolicyDisabled,
			Signatures: PolicyWhenAvailable,
		},
		Runner: &fakeRunner{
			err: fmt.Errorf("%w: Error: invalid signature when validating ASN.1 encoded signature", ErrSignatureFailed),
		},
	})

	require.ErrorIs(t, err, ErrSignatureFailed)
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

func writeAssetPathPlaceholder(_ []string) string {
	return "<asset>"
}
