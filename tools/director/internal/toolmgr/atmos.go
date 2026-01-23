package toolmgr

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
)

// installAtmos downloads and installs atmos from GitHub releases.
func (m *Manager) installAtmos(ctx context.Context, config *ToolConfig, destPath string) error {
	switch config.Source {
	case "github", "":
		return m.installAtmosFromGitHub(ctx, config.Version, destPath)
	case "local":
		return m.installAtmosFromLocal(destPath)
	case "path":
		// Just verify it exists in PATH, don't install.
		return nil
	default:
		return fmt.Errorf("unknown source for atmos: %s", config.Source)
	}
}

// installAtmosFromGitHub downloads atmos from GitHub releases.
func (m *Manager) installAtmosFromGitHub(ctx context.Context, version, destPath string) error {
	// Determine platform and architecture.
	platform := runtime.GOOS
	arch := runtime.GOARCH

	// Map Go arch names to atmos release names.
	archMap := map[string]string{
		"amd64": "amd64",
		"arm64": "arm64",
	}
	releaseArch, ok := archMap[arch]
	if !ok {
		return fmt.Errorf("unsupported architecture: %s", arch)
	}

	// Build download URL.
	// Format: https://github.com/cloudposse/atmos/releases/download/v1.202.1/atmos_1.202.1_darwin_arm64
	url := fmt.Sprintf(
		"https://github.com/cloudposse/atmos/releases/download/v%s/atmos_%s_%s_%s",
		version, version, platform, releaseArch,
	)

	fmt.Printf("  Downloading from %s\n", url)

	// Create HTTP request with context.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Execute request.
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download atmos: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download atmos: HTTP %d", resp.StatusCode)
	}

	// Create destination file.
	out, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer out.Close()

	// Copy response body to file.
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	// Make executable.
	if err := os.Chmod(destPath, 0o755); err != nil {
		return fmt.Errorf("failed to make executable: %w", err)
	}

	return nil
}

// installAtmosFromLocal copies the locally-built atmos binary.
func (m *Manager) installAtmosFromLocal(destPath string) error {
	// Worktree root is the parent of demosDir.
	worktreeRoot := filepath.Dir(m.demosDir)

	// Look for local atmos binary in common locations relative to worktree root.
	candidates := []string{
		filepath.Join(worktreeRoot, "build", "atmos"),
		filepath.Join(worktreeRoot, "atmos"),
	}

	var srcPath string
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			srcPath = candidate
			break
		}
	}

	if srcPath == "" {
		return fmt.Errorf("local atmos binary not found; tried: %v", candidates)
	}

	fmt.Printf("  Copying from %s\n", srcPath)

	// Copy the file.
	src, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("failed to open source: %w", err)
	}
	defer src.Close()

	dest, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create destination: %w", err)
	}
	defer dest.Close()

	if _, err := io.Copy(dest, src); err != nil {
		return fmt.Errorf("failed to copy file: %w", err)
	}

	// Make executable.
	if err := os.Chmod(destPath, 0o755); err != nil {
		return fmt.Errorf("failed to make executable: %w", err)
	}

	return nil
}
