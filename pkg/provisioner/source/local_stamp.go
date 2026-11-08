package source

import (
	"encoding/hex"
	"fmt"
	"hash/fnv"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/provisioner/workdir"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/vendor"
)

// localStampSkippedDirs are directories a local module copy never carries into a workdir.
// Editing them must not refresh the workdir.
var localStampSkippedDirs = map[string]struct{}{".git": {}, ".terraform": {}, ".terragrunt-cache": {}}

// localSourceRoot returns the directory a local directory source copies from, resolved the same
// way as vendoring resolves it. It reports false for remote sources and for missing directories.
func localSourceRoot(spec *schema.VendorComponentSource) (string, bool) {
	uri := vendor.NormalizeURI(resolveSourceURI(spec))
	if vendor.IsLocalPath(uri) && !filepath.IsAbs(uri) {
		absolute, err := resolveLocalSourcePath(uri, "")
		if err != nil {
			return "", false
		}
		uri = absolute
	}
	dir, ok, err := localDirectorySource(uri)
	if err != nil || !ok {
		return "", false
	}
	return dir, true
}

// localSourceStamp digests a stat manifest (relative path, size, modification time, mode) of the
// local directory source. It reads no file contents, so refreshing decisions stay cheap for
// modules with many or large files. It returns an empty string for any other kind of source.
//
// The stamp is a change-detection token compared across processes, not a credential hash, so it uses
// a non-cryptographic 128-bit FNV-1a digest. It stays stable for an unchanged source.
func localSourceStamp(spec *schema.VendorComponentSource) string {
	root, ok := localSourceRoot(spec)
	if !ok {
		return ""
	}
	// Walk the target of a symlinked source root; WalkDir never descends into a root link.
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	hash := fnv.New128a()
	walkErr := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if _, skip := localStampSkippedDirs[entry.Name()]; skip && path != root {
				return filepath.SkipDir
			}
			return nil
		}
		return stampEntry(hash, root, path, entry)
	})
	if walkErr != nil {
		return ""
	}
	return hex.EncodeToString(hash.Sum(nil))
}

// stampEntry adds one non-directory entry to the manifest digest.
func stampEntry(hash io.Writer, root, path string, entry fs.DirEntry) error {
	info, err := entry.Info()
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return err
	}
	fmt.Fprintf(hash, "%s\x00%d\x00%d\x00%d\n", filepath.ToSlash(relative), info.Size(), info.ModTime().UnixNano(), info.Mode())
	if info.Mode()&os.ModeSymlink != 0 {
		// A link edited through its target changes the module without touching the link.
		if target, statErr := os.Stat(path); statErr == nil {
			fmt.Fprintf(hash, "->%d\x00%d\n", target.Size(), target.ModTime().UnixNano())
		}
	}
	return nil
}

// localSourceChanged reports whether a local directory source was edited, or removed, since the
// workdir recorded its stamp. An explicit TTL stays authoritative. A local workdir without a stamp
// (provisioned before stamps existed, or a failed stamp write) is refreshed once so it records one;
// a remote source never has a stamp, and URI, version, and TTL decide for it.
func localSourceChanged(metadata *workdir.WorkdirMetadata, spec *schema.VendorComponentSource) (bool, string) {
	if spec.TTL != "" {
		return false, ""
	}
	stamp := localSourceStamp(spec)
	if metadata.SourceStamp == "" {
		if stamp == "" {
			return false, ""
		}
		return true, "Local source has no recorded stamp, refreshing the working directory"
	}
	if stamp == metadata.SourceStamp {
		return false, ""
	}
	return true, "Local source changed, refreshing the working directory"
}

// recordSourceStamp stores the stamp taken before the local source was copied. A failure only
// causes one extra refresh on the next run, because the stored stamp then differs.
func recordSourceStamp(targetDir, stamp string, out *ui.Output) {
	defer perf.Track(nil, "source.recordSourceStamp")()

	if stamp == "" {
		return
	}
	metadata, err := workdir.ReadMetadata(targetDir)
	if err == nil && metadata != nil {
		metadata.SourceStamp = stamp
		err = workdir.WriteMetadata(targetDir, metadata)
	}
	if err != nil {
		out.Warningf("Failed to record the local source stamp: %s", err)
	}
}

// sameLocalSource reports whether two local source declarations name one directory, so a
// relative and an absolute spelling of the same path do not look like a changed source.
func sameLocalSource(left, right string) bool {
	if !isLocalSource(left) || !isLocalSource(right) {
		return false
	}
	absoluteLeft, errLeft := filepath.Abs(left)
	absoluteRight, errRight := filepath.Abs(right)
	return errLeft == nil && errRight == nil && absoluteLeft == absoluteRight
}
