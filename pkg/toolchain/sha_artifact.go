package toolchain

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/github"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
)

const (
	// SHAVersionDirFormat is the format for SHA version directory names.
	shaVersionDirFormat = "sha-%s"
)

// SHACacheMetadata stores info about a cached SHA binary.
type SHACacheMetadata struct {
	SHA       string    `json:"sha"`        // Full SHA of the commit.
	CheckedAt time.Time `json:"checked_at"` // Last time we installed/verified.
	RunID     int64     `json:"run_id"`     // Workflow run ID.
}

// CheckSHACacheStatus determines if a SHA binary needs installation.
// Returns true if binary exists and is valid, along with the binary path.
func CheckSHACacheStatus(sha string) (exists bool, binaryPath string) {
	defer perf.Track(nil, "toolchain.CheckSHACacheStatus")()

	shortSHA := sha
	if len(shortSHA) > shortSHALength {
		shortSHA = shortSHA[:shortSHALength]
	}

	shaVersionDir := fmt.Sprintf(shaVersionDirFormat, shortSHA)
	installDir := filepath.Join(GetInstallPath(), "bin", atmosOwner, atmosRepo, shaVersionDir)

	// Check if binary exists.
	binaryName := atmosBinaryName
	if runtime.GOOS == "windows" {
		binaryName = atmosBinaryNameExe
	}
	binaryPath = filepath.Join(installDir, binaryName)

	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		return false, ""
	}

	// Binary exists - SHAs are immutable, so no need to check for updates.
	log.Debug("Found cached SHA binary", "sha", shortSHA, "path", binaryPath)
	return true, binaryPath
}

// saveSHACacheMetadata saves cache metadata for a SHA.
func saveSHACacheMetadata(sha string, meta *SHACacheMetadata) error {
	shortSHA := sha
	if len(shortSHA) > shortSHALength {
		shortSHA = shortSHA[:shortSHALength]
	}

	shaVersionDir := fmt.Sprintf(shaVersionDirFormat, shortSHA)
	metaPath := filepath.Join(GetInstallPath(), "bin", atmosOwner, atmosRepo, shaVersionDir, cacheMetadataFile)

	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(metaPath, data, cacheFilePermissions)
}

// SaveSHACacheMetadataAfterInstall saves cache metadata after a successful installation.
func SaveSHACacheMetadataAfterInstall(sha string, info *github.SHAArtifactInfo) {
	defer perf.Track(nil, "toolchain.SaveSHACacheMetadataAfterInstall")()

	meta := &SHACacheMetadata{
		SHA:       info.HeadSHA,
		CheckedAt: time.Now(),
		RunID:     info.RunID,
	}
	if err := saveSHACacheMetadata(sha, meta); err != nil {
		log.Debug("Failed to save cache metadata after install", "error", err)
	}
}

// InstallFromSHA downloads and installs Atmos from a commit SHA's build artifact.
// Returns the installed binary path.
func InstallFromSHA(sha string, showProgress bool) (string, error) {
	defer perf.Track(nil, "toolchain.InstallFromSHA")()

	ctx := context.Background()

	// Check for GitHub token (required for artifact downloads).
	token, err := github.GetGitHubTokenOrError()
	if err != nil {
		return "", buildTokenRequiredError()
	}

	shortSHA := sha
	if len(shortSHA) > shortSHALength {
		shortSHA = shortSHA[:shortSHALength]
	}

	// Show progress if requested.
	if showProgress {
		ui.Infof("Installing Atmos from SHA `%s`...", shortSHA)
	}

	// Get artifact info.
	artifactInfo, err := github.GetSHAArtifactInfo(ctx, atmosOwner, atmosRepo, sha)
	if err != nil {
		return "", handleSHAArtifactError(err, sha)
	}

	if showProgress {
		// Format timestamp and short SHA for enriched message.
		timeStr := artifactInfo.RunStartedAt.Local().Format("Jan 2, 2006 at 3:04 PM")
		displaySHA := artifactInfo.HeadSHA
		if len(displaySHA) > shortSHALength {
			displaySHA = displaySHA[:shortSHALength]
		}
		ui.Successf("Found workflow run #%d (sha: `%s`) from %s", artifactInfo.RunID, displaySHA, timeStr)
	}

	// Download and install the artifact.
	shaVersionDir := fmt.Sprintf(shaVersionDirFormat, shortSHA)

	// Convert SHAArtifactInfo to PRArtifactInfo for the shared download function.
	prInfo := &github.PRArtifactInfo{
		PRNumber:     0, // Not a PR.
		HeadSHA:      artifactInfo.HeadSHA,
		RunID:        artifactInfo.RunID,
		ArtifactID:   artifactInfo.ArtifactID,
		ArtifactName: artifactInfo.ArtifactName,
		SizeInBytes:  artifactInfo.SizeInBytes,
		DownloadURL:  artifactInfo.DownloadURL,
		RunStartedAt: artifactInfo.RunStartedAt,
	}

	binaryPath, err := downloadAndInstallArtifactToDir(ctx, token, shaVersionDir, prInfo, showProgress)
	if err != nil {
		return "", err
	}

	// Save cache metadata for future lookups.
	SaveSHACacheMetadataAfterInstall(sha, artifactInfo)

	return binaryPath, nil
}

// handleSHAArtifactError converts GitHub errors to user-friendly errors.
func handleSHAArtifactError(err error, sha string) error {
	shortSHA := sha
	if len(shortSHA) > shortSHALength {
		shortSHA = shortSHA[:shortSHALength]
	}

	commitURL := fmt.Sprintf("https://github.com/%s/%s/commit/%s", atmosOwner, atmosRepo, sha)

	// Check for specific error types.
	if isNotFoundError(err) {
		return buildSHANotFoundError(shortSHA, commitURL)
	}

	if isNoWorkflowError(err) {
		return buildNoWorkflowForSHAError(shortSHA, commitURL)
	}

	if isNoArtifactError(err) {
		return buildNoArtifactForSHAError(shortSHA, commitURL)
	}

	if isPlatformError(err) {
		return buildPlatformNotSupportedError()
	}

	// Generic error.
	return buildGenericSHAError(shortSHA, commitURL, err)
}

// isNotFoundError checks if the error is a "not found" type error.
func isNotFoundError(err error) bool {
	return github.IsNotFoundError(err)
}

// isNoWorkflowError checks if the error is a "no workflow run" error.
func isNoWorkflowError(err error) bool {
	return github.IsNoWorkflowError(err)
}

// isNoArtifactError checks if the error is a "no artifact" error.
func isNoArtifactError(err error) bool {
	return github.IsNoArtifactError(err)
}

// isPlatformError checks if the error is a platform-related error.
func isPlatformError(err error) bool {
	return github.IsPlatformError(err)
}

// buildSHANotFoundError builds a user-friendly error for SHA not found.
func buildSHANotFoundError(shortSHA, commitURL string) error {
	return errUtils.Build(errUtils.ErrToolNotFound).
		WithExplanationf("Commit %s not found", shortSHA).
		WithHintf("Check commit: %s", commitURL).
		WithExitCode(1).
		Err()
}

// buildNoWorkflowForSHAError builds a user-friendly error for no workflow run.
func buildNoWorkflowForSHAError(shortSHA, commitURL string) error {
	return errUtils.Build(errUtils.ErrToolNotFound).
		WithExplanationf("No successful CI run found for commit %s", shortSHA).
		WithHint("Possible reasons:").
		WithHint("  - CI workflow hasn't completed yet").
		WithHint("  - CI tests are failing").
		WithHint("  - Commit is from a fork").
		WithHintf("Check commit: %s", commitURL).
		WithExitCode(1).
		Err()
}

// buildNoArtifactForSHAError builds a user-friendly error for no artifact found.
func buildNoArtifactForSHAError(shortSHA, commitURL string) error {
	return errUtils.Build(errUtils.ErrToolNotFound).
		WithExplanationf("Build artifact not found for commit %s", shortSHA).
		WithHint("Possible reasons:").
		WithHint("  - Artifacts expired (90-day retention)").
		WithHint("  - Build job was skipped").
		WithHintf("Check commit: %s", commitURL).
		WithExitCode(1).
		Err()
}

// buildPlatformNotSupportedError builds a user-friendly error for unsupported platform.
func buildPlatformNotSupportedError() error {
	platforms := github.SupportedPRPlatforms()
	return errUtils.Build(errUtils.ErrToolPlatformNotSupported).
		WithExplanationf("No artifact available for %s/%s", runtime.GOOS, runtime.GOARCH).
		WithHintf("Artifacts currently only support: %s", joinStrings(platforms, ", ")).
		WithHint("For unsupported platforms, try:").
		WithHint("  - Download the release version: atmos --use-version <version>").
		WithHint("  - Build from source: go install github.com/cloudposse/atmos@<branch>").
		WithExitCode(1).
		Err()
}

// buildGenericSHAError builds a generic error with the underlying cause.
func buildGenericSHAError(shortSHA, commitURL string, err error) error {
	return errUtils.Build(errUtils.ErrToolInstall).
		WithExplanationf("Failed to get artifact info for commit %s", shortSHA).
		WithCause(err).
		WithHintf("Check commit: %s", commitURL).
		WithExitCode(1).
		Err()
}

// joinStrings joins strings with a separator.
func joinStrings(strs []string, sep string) string {
	result := ""
	for i, s := range strs {
		if i > 0 {
			result += sep
		}
		result += s
	}
	return result
}
