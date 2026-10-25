package release

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/cloudposse/atmos/magefiles/internal/githubapi"
	"github.com/cloudposse/atmos/pkg/toolchain/registry"
	"github.com/cloudposse/atmos/pkg/toolchain/registry/aqua"
	"github.com/cloudposse/atmos/pkg/toolchain/verification"
)

// VerifyInstaller checks staged bytes through the same registry resolution and
// verifier used by toolchain install. Registry metadata comes from a freshly
// resolved public Aqua revision, never from a user's cached recipe or override.
func (Release) VerifyInstaller(dir, repo, tag string) error {
	if !githubapi.RepoPattern.MatchString(repo) || !strings.HasPrefix(tag, "v") {
		return fmt.Errorf("%w: invalid installer verification identity", ErrReleaseVerification)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	revision, err := aquaRevision(ctx, &http.Client{Timeout: 30 * time.Second}, "https://api.github.com/repos/aquaproj/aqua-registry/commits/main")
	if err != nil {
		return err
	}
	source := "https://raw.githubusercontent.com/aquaproj/aqua-registry/" + revision + "/pkgs/cloudposse/atmos/registry.yaml"
	fmt.Fprintln(os.Stdout, "Installer trust policy:", source)
	tool, err := registry.NewURLRegistry(source, "").GetToolWithVersion("cloudposse", "atmos", tag)
	if err != nil {
		return err
	}
	verifier := installerVerifier{Repository: repo, Runner: verification.ExecRunner{}}
	if err = verifier.verifyTool(ctx, dir, tag, tool); err != nil {
		return fmt.Errorf("%w: installer policy at Aqua %s rejected %s: %w", ErrReleaseVerification, revision, tag, err)
	}
	if summary := os.Getenv("GITHUB_STEP_SUMMARY"); summary != "" {
		file, openErr := os.OpenFile(summary, os.O_APPEND|os.O_CREATE|os.O_WRONLY, workflowFileMode) // #nosec G703 -- GitHub supplies the step summary file path.
		if openErr != nil {
			return openErr
		}
		defer file.Close()
		_, err = fmt.Fprintf(file, "\nInstaller verification for `%s` passed using [Aqua revision `%s`](%s).\n", tag, revision, source)
	}
	return err
}

func aquaRevision(ctx context.Context, client *http.Client, endpoint string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Cache-Control", "no-cache")
	response, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%w: Aqua registry revision lookup returned HTTP %d", ErrReleaseVerification, response.StatusCode)
	}
	var commit struct {
		SHA string `json:"sha"`
	}
	if err = json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(&commit); err != nil {
		return "", err
	}
	if !githubapi.SHAPattern.MatchString(commit.SHA) {
		return "", fmt.Errorf("%w: invalid Aqua revision", ErrReleaseVerification)
	}
	return commit.SHA, nil
}

type installerVerifier struct {
	Repository string
	Runner     verification.CommandRunner
}

func (v *installerVerifier) verifyTool(ctx context.Context, dir, tag string, tool *registry.Tool) error {
	assetURL, name, err := installerAsset(tool, v.Repository, tag)
	if err != nil {
		return err
	}
	local := stagedReleaseDownloader{directory: dir, prefix: "https://github.com/" + v.Repository + "/releases/download/" + tag + "/"}
	if _, err = local.assetName(assetURL); err != nil {
		return err
	}
	result, err := (&verification.Verifier{}).Verify(ctx, verification.Request{
		Tool: tool, Version: tag, AssetURL: assetURL, AssetPath: filepath.Join(dir, name),
		Downloader: local, Runner: v.Runner,
		Policy: verification.Policy{Checksums: verification.PolicyRequired, Signatures: verification.PolicyRequired, VerifierInstall: verification.VerifierInstallPathOnly},
	})
	if err != nil {
		return err
	}
	if result.ChecksumAlgorithm != "sha256" || !slices.Contains(result.SignatureMethods, "cosign") || len(result.SkippedReasons) != 0 {
		return fmt.Errorf("%w: installer must verify SHA256 and Cosign without skipped checks", ErrReleaseVerification)
	}
	return nil
}

func installerAsset(tool *registry.Tool, repo, tag string) (string, string, error) {
	if tool.RepoOwner+"/"+tool.RepoName != repo || tool.Type != "github_release" || tool.Format != "raw" {
		return "", "", fmt.Errorf("%w: installer recipe changed repository or artifact layout", ErrReleaseVerification)
	}
	assetURL, err := aqua.NewAquaRegistry().BuildAssetURL(tool, tag)
	if err != nil {
		return "", "", err
	}
	u, err := url.Parse(assetURL)
	if err != nil {
		return "", "", err
	}
	name := filepath.Base(u.Path)
	expected := "atmos_" + strings.TrimPrefix(tag, "v") + "_" + runtime.GOOS + "_" + runtime.GOARCH
	if runtime.GOOS == "windows" {
		expected += ".exe"
	}
	if name != expected {
		return "", "", fmt.Errorf("%w: installer selects unexpected binary %s", ErrReleaseVerification, name)
	}
	return assetURL, name, nil
}

// Staged assets stand in for their eventual public URLs. Verification must not
// accidentally download a previous release or a caller-controlled sidecar.
type stagedReleaseDownloader struct{ directory, prefix string }

func (d stagedReleaseDownloader) assetName(raw string) (string, error) {
	name := strings.TrimPrefix(raw, d.prefix)
	if !strings.HasPrefix(raw, d.prefix) || !validAssetName(name) {
		return "", fmt.Errorf("%w: installer requested an unexpected asset URL %s", ErrReleaseVerification, raw)
	}
	return name, nil
}

func (d stagedReleaseDownloader) Download(_ context.Context, raw string) ([]byte, error) {
	name, err := d.assetName(raw)
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(d.directory)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	return root.ReadFile(name)
}
