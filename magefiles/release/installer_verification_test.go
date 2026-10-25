package release

//go:generate go run go.uber.org/mock/mockgen -destination=installer_runner_mock_test.go -package=release github.com/cloudposse/atmos/pkg/toolchain/verification CommandRunner

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/cloudposse/atmos/pkg/toolchain/registry"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestFreshAquaRevisionFailsClosed(t *testing.T) {
	for _, mode := range []string{"valid", "unavailable", "malformed", "branch instead of SHA"} {
		t.Run(mode, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "no-cache", r.Header.Get("Cache-Control"))
				switch mode {
				case "unavailable":
					http.Error(w, "unavailable", http.StatusServiceUnavailable)
				case "malformed":
					fmt.Fprint(w, "{")
				case "branch instead of SHA":
					fmt.Fprint(w, `{"sha":"main"}`)
				default:
					fmt.Fprintf(w, `{"sha":%q}`, attestationSourceSHA)
				}
			}))
			defer server.Close()
			sha, err := aquaRevision(context.Background(), server.Client(), server.URL)
			if mode == "valid" {
				require.NoError(t, err)
				require.Equal(t, attestationSourceSHA, sha)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestInstallerGateVerifiesStagedChecksumUsingSelectedPolicy(t *testing.T) {
	for _, mode := range []string{"current policy", "RC policy", "stale policy", "bad checksum", "missing bundle", "no signature", "disabled signature", "wrong repository", "wrong asset", "wrong bundle release"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			tag := "v1.235.0"
			if mode == "RC policy" {
				tag += "-rc.1"
			}
			name := "atmos_" + strings.TrimPrefix(tag, "v") + "_" + runtime.GOOS + "_" + runtime.GOARCH
			if runtime.GOOS == "windows" {
				name += ".exe"
			}
			body := []byte("staged binary")
			sumName := "atmos_" + strings.TrimPrefix(tag, "v") + "_SHA256SUMS"
			require.NoError(t, os.WriteFile(filepath.Join(dir, name), body, 0o600))
			checksum := fmt.Sprintf("%x  %s\n", sha256.Sum256(body), name)
			if mode == "bad checksum" {
				checksum = strings.Repeat("0", 64) + "  " + name + "\n"
			}
			require.NoError(t, os.WriteFile(filepath.Join(dir, sumName), []byte(checksum), 0o600))
			if mode != "missing bundle" {
				require.NoError(t, os.WriteFile(filepath.Join(dir, sumName+".sigstore.json"), []byte("bundle"), 0o600))
			}
			signer := "https://github.com/cloudposse/atmos/" + taggedBuildWorkflow + "@refs/tags/{{.Version}}"
			if mode == "stale policy" {
				signer = "https://github.com/cloudposse/atmos/.github/workflows/release-pipeline.yml@refs/heads/main"
			}
			tool := &registry.Tool{Type: "github_release", RepoOwner: "cloudposse", RepoName: "atmos", Format: "raw", Asset: "atmos_{{trimV .Version}}_{{.OS}}_{{.Arch}}", Checksum: registry.ChecksumConfig{
				Type: "github_release", Asset: "atmos_{{trimV .Version}}_SHA256SUMS", Algorithm: "sha256", Cosign: registry.CosignConfig{
					Bundle: registry.DownloadedFile{Type: "github_release", Asset: "atmos_{{trimV .Version}}_SHA256SUMS.sigstore.json"},
					Opts:   []string{"--certificate-identity", signer, "--certificate-oidc-issuer", "https://token.actions.githubusercontent.com"},
				},
			}}
			if runtime.GOOS == "windows" {
				tool.Asset += ".exe"
			}
			switch mode {
			case "no signature":
				tool.Checksum.Cosign = registry.CosignConfig{}
			case "disabled signature":
				disabled := false
				tool.Checksum.Cosign.Enabled = &disabled
			case "wrong repository":
				tool.RepoOwner = "other"
			case "wrong asset":
				tool.Asset = "unrelated"
			case "wrong bundle release":
				tool.Checksum.Cosign.Bundle = registry.DownloadedFile{Type: "http", URL: "https://github.com/cloudposse/atmos/releases/download/v1.234.0/bundle.json"}
			}
			runner := NewMockCommandRunner(gomock.NewController(t))
			// A real checksum and the real installer argument rendering run here. Only
			// the external Sigstore verification is substituted; live OIDC is an RC gate.
			if mode == "current policy" || mode == "RC policy" || mode == "stale policy" || mode == "bad checksum" {
				runner.EXPECT().Run(gomock.Any(), "cosign", gomock.Any()).DoAndReturn(func(_ context.Context, _ string, args ...string) error {
					require.Contains(t, args, "https://token.actions.githubusercontent.com")
					require.Contains(t, args, strings.ReplaceAll(signer, "{{.Version}}", tag))
					data, err := os.ReadFile(args[len(args)-1])
					require.NoError(t, err)
					require.Equal(t, checksum, string(data))
					if mode == "stale policy" {
						return errors.New("certificate identity mismatch")
					}
					return nil
				})
			}
			verifier := installerVerifier{Repository: attestationRepository, Runner: runner}
			err := verifier.verifyTool(context.Background(), dir, tag, tool)
			if mode == "current policy" || mode == "RC policy" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestStagedDownloaderRejectsOtherReleasesAndTraversal(t *testing.T) {
	d := stagedReleaseDownloader{directory: t.TempDir(), prefix: "https://github.com/cloudposse/atmos/releases/download/v1.235.0/"}
	for _, raw := range []string{"https://elsewhere/asset", d.prefix + "../secret", d.prefix + "%2e%2e%2fsecret", d.prefix + "asset?token=1", strings.Replace(d.prefix, "1.235.0", "1.234.0", 1) + "asset"} {
		_, err := d.Download(context.Background(), raw)
		require.Error(t, err, raw)
	}
}

func TestInstallerRecipePreservesVersionedChecksumSigners(t *testing.T) {
	t.Setenv("ATMOS_XDG_CACHE_HOME", t.TempDir())
	recipe := `packages:
  - type: github_release
    repo_owner: cloudposse
    repo_name: atmos
    version_constraint: "false"
    version_overrides:
      - version_constraint: semver("<= 1.227.0")
        asset: atmos_{{trimV .Version}}_{{.OS}}_{{.Arch}}
        format: raw
        checksum:
          type: github_release
          asset: atmos_{{trimV .Version}}_SHA256SUMS
          algorithm: sha256
      - version_constraint: semver("< 1.233.2-0")
        asset: atmos_{{trimV .Version}}_{{.OS}}_{{.Arch}}
        format: raw
        checksum:
          type: github_release
          asset: atmos_{{trimV .Version}}_SHA256SUMS
          algorithm: sha256
          cosign:
            bundle:
              type: github_release
              asset: atmos_{{trimV .Version}}_SHA256SUMS.sigstore.json
            opts: [--certificate-identity-regexp, shared-workflow]
      - version_constraint: 'Version matches "^v?[0-9]+\\.[0-9]+\\.[0-9]+(-rc\\.[0-9]+)?$" && semver(">= 1.235.0-0")'
        asset: atmos_{{trimV .Version}}_{{.OS}}_{{.Arch}}
        format: raw
        checksum:
          type: github_release
          asset: atmos_{{trimV .Version}}_SHA256SUMS
          algorithm: sha256
          cosign:
            bundle:
              type: github_release
              asset: atmos_{{trimV .Version}}_SHA256SUMS.sigstore.json
            opts: [--certificate-identity, 'release-tag-build.yml@refs/tags/{{.Version}}']
      - version_constraint: "true"
        asset: atmos_{{trimV .Version}}_{{.OS}}_{{.Arch}}
        format: raw
        checksum:
          type: github_release
          asset: atmos_{{trimV .Version}}_SHA256SUMS
          algorithm: sha256
          cosign:
            bundle:
              type: github_release
              asset: atmos_{{trimV .Version}}_SHA256SUMS.sigstore.json
            opts: [--certificate-identity, release-pipeline.yml@refs/heads/main]
`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, recipe) }))
	defer server.Close()
	for _, tc := range []struct{ tag, signer string }{
		{"v1.227.0", ""}, {"v1.233.0", "shared-workflow"}, {"v1.233.2-rc.3", "release-pipeline.yml@refs/heads/main"}, {"v1.233.2", "release-pipeline.yml@refs/heads/main"}, {"v1.234.0", "release-pipeline.yml@refs/heads/main"}, {"v1.235.0-rc.1", "release-tag-build.yml@refs/tags/{{.Version}}"}, {"v1.235.0", "release-tag-build.yml@refs/tags/{{.Version}}"}, {"v1.235.0-nightly.77", "release-pipeline.yml@refs/heads/main"}, {"v1.235.0-feature.42.77", "release-pipeline.yml@refs/heads/main"},
	} {
		t.Run(tc.tag, func(t *testing.T) {
			tool, err := resolveInstallerPolicy(server.URL, tc.tag)
			require.NoError(t, err)
			require.Equal(t, "sha256", tool.Checksum.Algorithm)
			if tc.signer == "" {
				require.Empty(t, tool.Checksum.Cosign.Opts)
			} else {
				require.Contains(t, tool.Checksum.Cosign.Opts, tc.signer)
				require.Equal(t, "atmos_{{trimV .Version}}_SHA256SUMS.sigstore.json", tool.Checksum.Cosign.Bundle.Asset)
			}
		})
	}
}
