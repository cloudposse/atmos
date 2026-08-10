package marketplace

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"

	"github.com/cloudposse/atmos/pkg/perf"
)

// Downloader handles downloading skills from Git repositories.
type Downloader struct{}

// NewDownloader creates a new downloader instance.
func NewDownloader() *Downloader {
	return &Downloader{}
}

// Download clones a Git repository to a temporary directory, or, for a "local"
// source (see ParseSource), copies it from disk instead. Either way it returns the
// path to the temporary directory the caller should treat as the downloaded skill.
func (d *Downloader) Download(ctx context.Context, source *SourceInfo) (string, error) {
	defer perf.Track(nil, "marketplace.Downloader.Download")()

	// Create temporary directory.
	tempDir, err := os.MkdirTemp("", "atmos-skill-*")
	if err != nil {
		return "", fmt.Errorf("%w: failed to create temp directory: %w", ErrDownloadFailed, err)
	}

	if source.Type == "local" {
		// No network or Git clone involved -- just copy the directory tree,
		// reusing the same copyDir helper the multi-skill and client-distribution
		// paths already use.
		if err := copyDir(source.URL, tempDir); err != nil {
			os.RemoveAll(tempDir)
			return "", fmt.Errorf("%w: failed to copy local skill source %s: %w", ErrDownloadFailed, source.URL, err)
		}
		return tempDir, nil
	}

	// Clone options.
	cloneOpts := &git.CloneOptions{
		URL:      source.URL,
		Progress: nil, // TODO: Add progress reporting.
		Depth:    1,   // Shallow clone for faster downloads.
	}

	// If specific ref (tag/branch) requested, configure it.
	if source.Ref != "" {
		cloneOpts.ReferenceName = plumbing.NewBranchReferenceName(source.Ref)
		cloneOpts.SingleBranch = true

		// Try as tag if branch fails.
		cloneOpts.ReferenceName = plumbing.NewTagReferenceName(source.Ref)
	}

	// Clone repository.
	_, err = git.PlainCloneContext(ctx, tempDir, false, cloneOpts)
	if err != nil {
		// Cleanup temp directory on failure.
		os.RemoveAll(tempDir)

		// Check if error is due to authentication or network.
		if errors.Is(err, git.ErrRepositoryNotExists) {
			return "", fmt.Errorf("%w: repository not found: %s", ErrDownloadFailed, source.URL)
		}
		return "", fmt.Errorf("%w: git clone failed: %w", ErrDownloadFailed, err)
	}

	return tempDir, nil
}
