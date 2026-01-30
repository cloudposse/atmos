package toolchain

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

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

	// Binary names.
	atmosBinaryName    = "atmos"
	atmosBinaryNameExe = "atmos.exe"

	// Directory permissions for created directories.
	dirPermissions = 0o755

	// ShortSHALength is the number of characters to display for short SHA.
	shortSHALength = 7

	// PRCacheTTL is how long before checking API for updates.
	// Within this TTL, we use the cached binary without API calls.
	PRCacheTTL = 1 * time.Minute

	// CacheMetadataFile is the filename for cache metadata.
	cacheMetadataFile = ".cache.json"

	// PRVersionDirFormat is the format for PR version directory names.
	prVersionDirFormat = "pr-%d"

	// CacheFilePermissions is the file permission mode for cache metadata files.
	cacheFilePermissions = 0o600
)

// PR artifact errors.
var (
	// ErrPRArtifactDownloadFailed indicates the PR artifact download failed.
	ErrPRArtifactDownloadFailed = errors.New("failed to download PR artifact")

	// ErrPRArtifactExtractFailed indicates the PR artifact extraction failed.
	ErrPRArtifactExtractFailed = errors.New("failed to extract PR artifact")
)

// Error message format for Zip Slip attacks.
const errZipSlipFormat = "%w: illegal file path in archive (potential Zip Slip): %s"

// PRCacheMetadata stores info about a cached PR binary.
type PRCacheMetadata struct {
	HeadSHA   string    `json:"head_sha"`   // SHA of the commit when installed.
	CheckedAt time.Time `json:"checked_at"` // Last time we checked GitHub API.
	RunID     int64     `json:"run_id"`     // Workflow run ID.
}

// PRCacheStatus indicates what action to take for a cached PR binary.
type PRCacheStatus int

const (
	// PRCacheNeedsInstall means no binary exists, needs fresh install.
	PRCacheNeedsInstall PRCacheStatus = iota
	// PRCacheValid means binary exists and is within TTL, use as-is.
	PRCacheValid
	// PRCacheNeedsCheck means binary exists but TTL expired, check API for updates.
	PRCacheNeedsCheck
)

// CheckPRCacheStatus determines if a PR binary needs installation or API check.
// Returns the status and the binary path if it exists.
func CheckPRCacheStatus(prNumber int) (PRCacheStatus, string) {
	defer perf.Track(nil, "toolchain.CheckPRCacheStatus")()

	prVersionDir := fmt.Sprintf(prVersionDirFormat, prNumber)
	installDir := filepath.Join(GetInstallPath(), "bin", atmosOwner, atmosRepo, prVersionDir)

	// Check if binary exists.
	binaryName := atmosBinaryName
	if runtime.GOOS == "windows" {
		binaryName = atmosBinaryNameExe
	}
	binaryPath := filepath.Join(installDir, binaryName)

	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		return PRCacheNeedsInstall, ""
	}

	// Binary exists - check cache metadata.
	meta, err := loadPRCacheMetadata(prNumber)
	if err != nil {
		// No metadata or invalid - needs check.
		log.Debug("No valid cache metadata, needs API check", "pr", prNumber, "error", err)
		return PRCacheNeedsCheck, binaryPath
	}

	// Check if within TTL.
	if time.Since(meta.CheckedAt) < PRCacheTTL {
		log.Debug("Cache valid within TTL", "pr", prNumber, "age", time.Since(meta.CheckedAt))
		return PRCacheValid, binaryPath
	}

	log.Debug("Cache TTL expired, needs API check", "pr", prNumber, "age", time.Since(meta.CheckedAt))
	return PRCacheNeedsCheck, binaryPath
}

// CheckPRCacheAndUpdate checks if the cached PR binary is up to date with GitHub.
// If the SHA has changed, it returns true to indicate a re-download is needed.
// If the SHA is the same, it updates the timestamp and returns false.
func CheckPRCacheAndUpdate(ctx context.Context, prNumber int, showProgress bool) (needsReinstall bool, err error) {
	defer perf.Track(nil, "toolchain.CheckPRCacheAndUpdate")()

	// Load existing metadata.
	meta, err := loadPRCacheMetadata(prNumber)
	if err != nil {
		// No metadata - need to reinstall to get fresh metadata.
		return true, nil
	}

	// Check for GitHub token.
	token, err := github.GetGitHubTokenOrError()
	if err != nil {
		return false, buildTokenRequiredError()
	}

	// Get current PR head SHA.
	currentSHA, err := github.GetPRHeadSHA(ctx, atmosOwner, atmosRepo, prNumber, token)
	if err != nil {
		return false, handlePRArtifactError(err, prNumber)
	}

	// Update checked timestamp regardless.
	meta.CheckedAt = time.Now()

	shortOld := meta.HeadSHA
	if len(shortOld) > shortSHALength {
		shortOld = shortOld[:shortSHALength]
	}
	shortNew := currentSHA
	if len(shortNew) > shortSHALength {
		shortNew = shortNew[:shortSHALength]
	}

	if currentSHA == meta.HeadSHA {
		// SHA unchanged - just update timestamp, no re-download.
		if showProgress {
			ui.Successf("PR #%d is up to date (sha: `%s`)", prNumber, shortNew)
		}
		if saveErr := savePRCacheMetadata(prNumber, meta); saveErr != nil {
			log.Debug("Failed to save cache metadata", "error", saveErr)
		}
		return false, nil
	}

	// SHA changed - need to re-download.
	if showProgress {
		ui.Infof("New commit on PR #%d (sha: `%s` → `%s`)", prNumber, shortOld, shortNew)
	}
	return true, nil
}

// loadPRCacheMetadata loads cache metadata for a PR.
func loadPRCacheMetadata(prNumber int) (*PRCacheMetadata, error) {
	prVersionDir := fmt.Sprintf(prVersionDirFormat, prNumber)
	metaPath := filepath.Join(GetInstallPath(), "bin", atmosOwner, atmosRepo, prVersionDir, cacheMetadataFile)

	data, err := os.ReadFile(metaPath)
	if err != nil {
		return nil, err
	}

	var meta PRCacheMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}

	return &meta, nil
}

// savePRCacheMetadata saves cache metadata for a PR.
func savePRCacheMetadata(prNumber int, meta *PRCacheMetadata) error {
	prVersionDir := fmt.Sprintf(prVersionDirFormat, prNumber)
	metaPath := filepath.Join(GetInstallPath(), "bin", atmosOwner, atmosRepo, prVersionDir, cacheMetadataFile)

	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(metaPath, data, cacheFilePermissions)
}

// SavePRCacheMetadataAfterInstall saves cache metadata after a successful installation.
func SavePRCacheMetadataAfterInstall(prNumber int, info *github.PRArtifactInfo) {
	defer perf.Track(nil, "toolchain.SavePRCacheMetadataAfterInstall")()

	meta := &PRCacheMetadata{
		HeadSHA:   info.HeadSHA,
		CheckedAt: time.Now(),
		RunID:     info.RunID,
	}
	if err := savePRCacheMetadata(prNumber, meta); err != nil {
		log.Debug("Failed to save cache metadata after install", "error", err)
	}
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
		shortSHA := artifactInfo.HeadSHA
		if len(shortSHA) > shortSHALength {
			shortSHA = shortSHA[:shortSHALength]
		}
		ui.Successf("Found workflow run #%d (sha: `%s`) from %s", artifactInfo.RunID, shortSHA, timeStr)
	}

	// Download and install the artifact.
	binaryPath, err := downloadAndInstallArtifact(ctx, token, prNumber, artifactInfo, showProgress)
	if err != nil {
		return "", err
	}

	// Save cache metadata for future TTL checks.
	SavePRCacheMetadataAfterInstall(prNumber, artifactInfo)

	return binaryPath, nil
}

// downloadAndInstallArtifact handles the download and installation with optional progress display.
// Delegates to downloadAndInstallArtifactToDir with the PR-specific directory format.
func downloadAndInstallArtifact(
	ctx context.Context,
	token string,
	prNumber int,
	info *github.PRArtifactInfo,
	showProgress bool,
) (string, error) {
	defer perf.Track(nil, "toolchain.downloadAndInstallArtifact")()

	versionDir := fmt.Sprintf(prVersionDirFormat, prNumber)
	return downloadAndInstallArtifactToDir(ctx, token, versionDir, info, showProgress)
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

// installArtifactBinaryToDir extracts the binary from the artifact and installs it to the specified version directory.
// VersionDir is the directory name under ~/.atmos/bin/cloudposse/atmos/ (e.g., "pr-2040" or "sha-ceb7526").
//
//nolint:revive // Function length is acceptable for this self-contained installation logic.
func installArtifactBinaryToDir(versionDir, artifactPath string) (string, error) {
	defer perf.Track(nil, "toolchain.installArtifactBinaryToDir")()

	// Create extraction directory.
	extractDir, err := os.MkdirTemp("", "atmos-artifact-extract-")
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

	// Determine install path: ~/.atmos/bin/cloudposse/atmos/{versionDir}/atmos
	installDir := filepath.Join(GetInstallPath(), "bin", atmosOwner, atmosRepo, versionDir)
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

// downloadAndInstallArtifactToDir handles the download and installation to a specific version directory.
// VersionDir is the directory name under ~/.atmos/bin/cloudposse/atmos/ (e.g., "pr-2040" or "sha-ceb7526").
func downloadAndInstallArtifactToDir(
	ctx context.Context,
	token string,
	versionDir string,
	info *github.PRArtifactInfo,
	showProgress bool,
) (string, error) {
	defer perf.Track(nil, "toolchain.downloadAndInstallArtifactToDir")()

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
			binaryPath, installErr = installArtifactBinaryToDir(versionDir, artifactPath)
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

	binaryPath, err = installArtifactBinaryToDir(versionDir, artifactPath)
	if err != nil {
		return "", err
	}
	return binaryPath, nil
}

// extractZipFile extracts a ZIP file to the destination directory.
func extractZipFile(zipPath, destDir string) error {
	defer perf.Track(nil, "toolchain.extractZipFile")()

	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("%w: failed to open ZIP: %w", ErrPRArtifactExtractFailed, err)
	}
	defer r.Close()

	// Clean destDir for consistent comparison (Zip Slip protection).
	cleanDestDir := filepath.Clean(destDir) + string(os.PathSeparator)

	for _, f := range r.File {
		// Sanitize path to prevent Zip Slip attacks.
		destPath, err := sanitizeZipPath(f.Name, cleanDestDir)
		if err != nil {
			return err
		}

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

// sanitizeZipPath validates that a zip entry path is safe and returns the full destination path.
// This prevents Zip Slip attacks where malicious zip files contain paths like "../../../etc/passwd".
func sanitizeZipPath(entryName, cleanDestDir string) (string, error) {
	// Reject absolute paths (Unix and Windows style).
	if filepath.IsAbs(entryName) || strings.HasPrefix(entryName, "/") || strings.HasPrefix(entryName, "\\") {
		return "", fmt.Errorf(errZipSlipFormat, ErrPRArtifactExtractFailed, entryName)
	}

	// Reject paths containing backslash (potential Windows-style traversal on Unix).
	if strings.Contains(entryName, "\\") {
		return "", fmt.Errorf(errZipSlipFormat, ErrPRArtifactExtractFailed, entryName)
	}

	// Reject paths that start with or contain ".." traversal components.
	for _, part := range strings.Split(entryName, "/") {
		if part == ".." {
			return "", fmt.Errorf(errZipSlipFormat, ErrPRArtifactExtractFailed, entryName)
		}
	}

	// Join and clean the path.
	destPath := filepath.Join(strings.TrimSuffix(cleanDestDir, string(os.PathSeparator)), entryName)
	cleanedPath := filepath.Clean(destPath)

	// Final verification: ensure the cleaned path is still within the destination directory.
	if !strings.HasPrefix(cleanedPath+string(os.PathSeparator), cleanDestDir) &&
		cleanedPath != strings.TrimSuffix(cleanDestDir, string(os.PathSeparator)) {
		return "", fmt.Errorf(errZipSlipFormat, ErrPRArtifactExtractFailed, entryName)
	}

	return cleanedPath, nil
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
