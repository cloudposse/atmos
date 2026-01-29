package toolchain

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/github"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/spinner"
)

const (
	// Owner and repo for Atmos.
	atmosOwner = "cloudposse"
	atmosRepo  = "atmos"

	// PrVersionPrefix is the prefix used to identify PR version specifiers.
	prVersionPrefix = "pr:"

	// Binary names.
	atmosBinaryName    = "atmos"
	atmosBinaryNameExe = "atmos.exe"

	// Directory permissions for created directories.
	dirPermissions = 0o755

	// ShortSHALength is the number of characters to display for short SHA.
	shortSHALength = 7
)

// PR artifact errors.
var (
	// ErrPRArtifactDownloadFailed indicates the PR artifact download failed.
	ErrPRArtifactDownloadFailed = errors.New("failed to download PR artifact")

	// ErrPRArtifactExtractFailed indicates the PR artifact extraction failed.
	ErrPRArtifactExtractFailed = errors.New("failed to extract PR artifact")
)

// IsPRVersion checks if the version specifier is a PR reference (e.g., "pr:2038").
// Returns the PR number and true if it's a PR reference, otherwise 0 and false.
func IsPRVersion(version string) (int, bool) {
	defer perf.Track(nil, "toolchain.IsPRVersion")()

	if !strings.HasPrefix(version, prVersionPrefix) {
		return 0, false
	}

	prNumStr := strings.TrimPrefix(version, prVersionPrefix)
	prNum, err := strconv.Atoi(prNumStr)
	if err != nil || prNum <= 0 {
		return 0, false
	}

	return prNum, true
}

// InstallFromPR downloads and installs Atmos from a PR's build artifact.
// Returns the installed binary path.
func InstallFromPR(prNumber int, showProgress bool) (string, error) {
	defer perf.Track(nil, "toolchain.InstallFromPR")()

	ctx := context.Background()

	// Check for GitHub token (required for artifact downloads).
	token, err := github.GetGitHubTokenOrError()
	if err != nil {
		return "", buildTokenRequiredError()
	}

	// Show progress if requested.
	if showProgress {
		ui.Infof("Installing Atmos from PR #%d...", prNumber)
	}

	// Get artifact info.
	artifactInfo, err := github.GetPRArtifactInfo(ctx, atmosOwner, atmosRepo, prNumber)
	if err != nil {
		return "", handlePRArtifactError(err, prNumber)
	}

	if showProgress {
		// Format timestamp and short SHA for enriched message.
		timeStr := artifactInfo.RunStartedAt.Local().Format("Jan 2, 2006 at 3:04 PM")
		shortSHA := artifactInfo.HeadSHA[:shortSHALength]
		ui.Successf("Found workflow run #%d (sha: `%s`) from %s", artifactInfo.RunID, shortSHA, timeStr)
	}

	// Download and install the artifact.
	return downloadAndInstallArtifact(ctx, token, prNumber, artifactInfo, showProgress)
}

// downloadAndInstallArtifact handles the download and installation with optional progress display.
func downloadAndInstallArtifact(
	ctx context.Context,
	token string,
	prNumber int,
	info *github.PRArtifactInfo,
	showProgress bool,
) (string, error) {
	defer perf.Track(nil, "toolchain.downloadAndInstallArtifact")()

	var binaryPath string

	if showProgress {
		progressMsg := fmt.Sprintf("Downloading %s (%s)", info.ArtifactName, formatBytes(info.SizeInBytes))
		err := spinner.ExecWithSpinnerDynamic(progressMsg, func() (string, error) {
			artifactPath, downloadErr := downloadPRArtifact(ctx, token, info)
			if downloadErr != nil {
				return "", downloadErr
			}
			defer os.Remove(artifactPath)

			var installErr error
			binaryPath, installErr = installPRArtifactBinary(prNumber, artifactPath)
			if installErr != nil {
				return "", installErr
			}
			return fmt.Sprintf("Installed to %s", binaryPath), nil
		})
		if err != nil {
			return "", err
		}
		return binaryPath, nil
	}

	// Silent mode - no progress output.
	artifactPath, err := downloadPRArtifact(ctx, token, info)
	if err != nil {
		return "", err
	}
	defer os.Remove(artifactPath)

	binaryPath, err = installPRArtifactBinary(prNumber, artifactPath)
	if err != nil {
		return "", err
	}
	return binaryPath, nil
}

// downloadPRArtifact downloads the artifact ZIP to a temporary file.
func downloadPRArtifact(ctx context.Context, token string, info *github.PRArtifactInfo) (string, error) {
	defer perf.Track(nil, "toolchain.downloadPRArtifact")()

	// Create temp file for download.
	tempFile, err := os.CreateTemp("", "atmos-pr-artifact-*.zip")
	if err != nil {
		return "", fmt.Errorf("%w: failed to create temp file: %w", ErrPRArtifactDownloadFailed, err)
	}
	tempPath := tempFile.Name()
	defer tempFile.Close()

	// Download the artifact using the archive download URL.
	// GitHub redirects to a pre-signed URL, so we need to follow redirects.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, info.DownloadURL, nil)
	if err != nil {
		os.Remove(tempPath)
		return "", fmt.Errorf("%w: failed to create request: %w", ErrPRArtifactDownloadFailed, err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")

	client := &http.Client{
		// Follow redirects but preserve auth header for GitHub domain only.
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Don't add auth header when redirected to S3 (pre-signed URL).
			if !strings.Contains(req.URL.Host, "github") {
				req.Header.Del("Authorization")
			}
			return nil
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		os.Remove(tempPath)
		return "", fmt.Errorf("%w: failed to download: %w", ErrPRArtifactDownloadFailed, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		os.Remove(tempPath)
		return "", fmt.Errorf("%w: HTTP %d", ErrPRArtifactDownloadFailed, resp.StatusCode)
	}

	// Copy response to temp file.
	_, err = io.Copy(tempFile, resp.Body)
	if err != nil {
		os.Remove(tempPath)
		return "", fmt.Errorf("%w: failed to write file: %w", ErrPRArtifactDownloadFailed, err)
	}

	return tempPath, nil
}

// installPRArtifactBinary extracts the binary from the artifact and installs it.
//
//nolint:revive // Function length is acceptable for this self-contained installation logic.
func installPRArtifactBinary(prNumber int, artifactPath string) (string, error) {
	defer perf.Track(nil, "toolchain.installPRArtifactBinary")()

	// Create extraction directory.
	extractDir, err := os.MkdirTemp("", "atmos-pr-extract-")
	if err != nil {
		return "", fmt.Errorf("%w: failed to create extract dir: %w", ErrPRArtifactExtractFailed, err)
	}
	defer os.RemoveAll(extractDir)

	// Extract ZIP.
	if err := extractZipFile(artifactPath, extractDir); err != nil {
		return "", err
	}

	// Find the binary in the extracted files.
	// The artifact structure is: build/atmos or build/atmos.exe
	binaryName := atmosBinaryName
	if runtime.GOOS == "windows" {
		binaryName = atmosBinaryNameExe
	}

	// Look for binary in expected locations.
	var sourceBinary string
	searchPaths := []string{
		filepath.Join(extractDir, binaryName),
		filepath.Join(extractDir, "build", binaryName),
	}

	for _, path := range searchPaths {
		if _, err := os.Stat(path); err == nil {
			sourceBinary = path
			break
		}
	}

	if sourceBinary == "" {
		// List what we found for debugging.
		files, _ := listFiles(extractDir)
		log.Debug("Artifact contents", "files", files)
		return "", fmt.Errorf("%w: binary '%s' not found in artifact", ErrPRArtifactExtractFailed, binaryName)
	}

	// Determine install path: ~/.atmos/bin/cloudposse/atmos/pr-{number}/atmos
	installDir := filepath.Join(GetInstallPath(), "bin", atmosOwner, atmosRepo, fmt.Sprintf("pr-%d", prNumber))
	if err := os.MkdirAll(installDir, dirPermissions); err != nil {
		return "", fmt.Errorf("%w: failed to create install dir: %w", ErrPRArtifactExtractFailed, err)
	}

	destBinary := filepath.Join(installDir, binaryName)

	// Copy binary to install location.
	if err := copyFile(sourceBinary, destBinary); err != nil {
		return "", fmt.Errorf("%w: failed to copy binary: %w", ErrPRArtifactExtractFailed, err)
	}

	// Make executable on Unix.
	if runtime.GOOS != "windows" {
		if err := os.Chmod(destBinary, dirPermissions); err != nil {
			return "", fmt.Errorf("%w: failed to make executable: %w", ErrPRArtifactExtractFailed, err)
		}
	}

	return destBinary, nil
}

// extractZipFile extracts a ZIP file to the destination directory.
func extractZipFile(zipPath, destDir string) error {
	defer perf.Track(nil, "toolchain.extractZipFile")()

	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("%w: failed to open ZIP: %w", ErrPRArtifactExtractFailed, err)
	}
	defer r.Close()

	for _, f := range r.File {
		destPath := filepath.Join(destDir, f.Name) //nolint:gosec // Paths are controlled, not user input.

		// Create parent directories.
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(destPath, dirPermissions); err != nil {
				return fmt.Errorf("%w: failed to create dir: %w", ErrPRArtifactExtractFailed, err)
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(destPath), dirPermissions); err != nil {
			return fmt.Errorf("%w: failed to create parent dir: %w", ErrPRArtifactExtractFailed, err)
		}

		// Extract file.
		if err := extractZipEntry(f, destPath); err != nil {
			return err
		}
	}

	return nil
}

// extractZipEntry extracts a single ZIP entry to the destination path.
func extractZipEntry(f *zip.File, destPath string) error {
	rc, err := f.Open()
	if err != nil {
		return fmt.Errorf("%w: failed to open ZIP entry: %w", ErrPRArtifactExtractFailed, err)
	}
	defer rc.Close()

	outFile, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("%w: failed to create file: %w", ErrPRArtifactExtractFailed, err)
	}
	defer outFile.Close()

	_, err = io.Copy(outFile, rc) //nolint:gosec // Size is limited by artifact size.
	if err != nil {
		return fmt.Errorf("%w: failed to extract: %w", ErrPRArtifactExtractFailed, err)
	}

	return nil
}

// copyFile copies a file from src to dst.
func copyFile(src, dst string) error {
	defer perf.Track(nil, "toolchain.copyFile")()

	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}

// listFiles returns a list of all files in a directory (recursively).
func listFiles(dir string) ([]string, error) {
	var files []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
		files = append(files, rel)
		return nil
	})
	return files, err
}

// buildTokenRequiredError creates a user-friendly error when GitHub token is missing.
func buildTokenRequiredError() error {
	return errUtils.Build(errUtils.ErrAuthenticationFailed).
		WithExplanation("GitHub token required to download PR artifacts").
		WithHint("Option 1: Use GitHub CLI (recommended)\n  brew install gh && gh auth login").
		WithHint("Option 2: Set environment variable\n  export GITHUB_TOKEN=ghp_xxx").
		WithHint("Generate a token at: https://github.com/settings/tokens").
		WithHint("Required scope: public_repo (for public repositories)").
		WithExitCode(1).
		Err()
}

// handlePRArtifactError converts GitHub errors to user-friendly errors.
func handlePRArtifactError(err error, prNumber int) error {
	prURL := fmt.Sprintf("https://github.com/%s/%s/pull/%d", atmosOwner, atmosRepo, prNumber)

	// Check for specific error types.
	if errors.Is(err, github.ErrPRNotFound) {
		return errUtils.Build(errUtils.ErrToolNotFound).
			WithExplanationf("Pull request #%d not found", prNumber).
			WithHintf("Check PR status: %s", prURL).
			WithExitCode(1).
			Err()
	}

	if errors.Is(err, github.ErrNoWorkflowRunFound) {
		return errUtils.Build(errUtils.ErrToolNotFound).
			WithExplanationf("No successful CI run found for PR #%d", prNumber).
			WithHint("Possible reasons:").
			WithHint("  - CI workflow hasn't completed yet").
			WithHint("  - CI tests are failing").
			WithHint("  - PR is from a fork (artifacts not accessible)").
			WithHintf("Check PR status: %s", prURL).
			WithExitCode(1).
			Err()
	}

	if errors.Is(err, github.ErrNoArtifactFound) {
		return errUtils.Build(errUtils.ErrToolNotFound).
			WithExplanationf("Build artifact not found for PR #%d", prNumber).
			WithHint("Possible reasons:").
			WithHint("  - Artifacts expired (90-day retention)").
			WithHint("  - Build job was skipped").
			WithHintf("Check PR status: %s", prURL).
			WithExitCode(1).
			Err()
	}

	if errors.Is(err, github.ErrNoArtifactForPlatform) {
		platforms := github.SupportedPRPlatforms()
		return errUtils.Build(errUtils.ErrToolPlatformNotSupported).
			WithExplanationf("No PR artifact available for %s/%s", runtime.GOOS, runtime.GOARCH).
			WithHintf("PR builds currently only support: %s", strings.Join(platforms, ", ")).
			WithHint("For unsupported platforms, try:").
			WithHint("  - Download the release version: atmos --use-version <version>").
			WithHint("  - Build from source: go install github.com/cloudposse/atmos@<branch>").
			WithExitCode(1).
			Err()
	}

	// Generic error.
	return errUtils.Build(errUtils.ErrToolInstall).
		WithExplanationf("Failed to get artifact info for PR #%d", prNumber).
		WithCause(err).
		WithHintf("Check PR status: %s", prURL).
		WithExitCode(1).
		Err()
}
