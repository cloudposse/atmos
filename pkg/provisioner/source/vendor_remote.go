package source

import (
	"context"
	"os"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/vendor"
)

type vendorResolvedOptions struct {
	vendorSourceOptions
	uri string
}

func vendorRemoteSource(ctx context.Context, atmosConfig *schema.AtmosConfiguration, sourceSpec *schema.VendorComponentSource, targetDir string, resolved vendorResolvedOptions) error {
	vendorOpts, uri := resolved.vendorSourceOptions, resolved.uri
	if err := inspectRemoteTarget(targetDir, vendorOpts); err != nil {
		return err
	}

	tempDir, err := reserveDownloadPath()
	if err != nil {
		return err
	}

	defer os.RemoveAll(tempDir)

	// Download the source into tempDir. OCI registries aren't a go-getter scheme
	// (there is no getter.Getter implementation for them, and go-getter's model
	// doesn't fit pulling registry layers), so oci:// sources go through pkg/oci
	// directly; everything else still goes through go-getter.
	metadata, err := downloadSourceWithMetadata(ctx, atmosConfig, sourceSpec, uri, tempDir)
	if err != nil {
		return err
	}
	if err := vendorOpts.captureArtifact(ctx, atmosConfig, uri, tempDir, metadata); err != nil {
		return err
	}

	// Single-file downloads keep the component target as a directory. Handle
	// them explicitly because go-getter stages local files as external symlinks,
	// which the shared directory copier correctly refuses to follow.
	if !vendor.IsOCIURI(uri) && !vendor.IsGitURI(uri) && !vendor.IsArchiveURI(uri) {
		if singleFile, ok, err := singleFileInDir(tempDir); err != nil {
			return err
		} else if ok {
			return copySingleFileToDirectory(singleFile, targetDir, sourceSpec, vendorOpts)
		}
	}

	return copySourceToTarget(tempDir, targetDir, sourceSpec, vendorOpts)
}

func inspectRemoteTarget(targetDir string, vendorOpts vendorSourceOptions) error {
	if !vendorOpts.replaceTarget {
		if _, err := os.Stat(targetDir); err == nil {
			return errUtils.Build(errUtils.ErrSourceCopyFailed).
				WithExplanation("Target directory already exists").
				WithContext("target", targetDir).
				WithHint("Remove the target directory or enable replacement before provisioning").
				Err()
		} else if err != nil && !os.IsNotExist(err) {
			return errUtils.Build(errUtils.ErrSourceCopyFailed).
				WithCause(err).
				WithExplanation("Failed to inspect target directory").
				WithContext("target", targetDir).
				Err()
		} else if targetPathBlockedByFile(targetDir) {
			// os.Stat(targetDir) alone isn't enough: when an ancestor of targetDir is a
			// regular file (not a directory), Unix reports ENOTDIR (distinct from
			// ErrNotExist, caught above), but Windows reports the equivalent stat error
			// as ErrNotExist, so it would otherwise fall through to a doomed download.
			return errUtils.Build(errUtils.ErrSourceCopyFailed).
				WithExplanation("Failed to inspect target directory").
				WithContext("target", targetDir).
				WithHint("An ancestor of the target directory is a file, not a directory").
				Err()
		}
	}

	return nil
}

// reserveDownloadPath reserves a unique name, leaving it absent so go-getter's
// local-file getter can create a symlink rather than rejecting a precreated directory.
func reserveDownloadPath() (string, error) {
	tempDir, err := os.MkdirTemp("", "atmos-source-*")
	if err != nil {
		return "", errUtils.Build(errUtils.ErrCreateTempDir).
			WithCause(err).
			WithExplanation("Failed to create temporary directory for download").
			Err()
	}
	// Remove the empty directory so go-getter can create it as a symlink (for
	// file:// URIs) or recreate it as a real directory (for remote URIs).
	if removeErr := os.Remove(tempDir); removeErr != nil && !os.IsNotExist(removeErr) {
		return "", errUtils.Build(errUtils.ErrCreateTempDir).
			WithCause(removeErr).
			WithExplanation("Failed to prepare temporary directory for download").
			Err()
	}
	return tempDir, nil
}
