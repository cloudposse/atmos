package verification

import (
	"context"
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
