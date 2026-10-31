package source

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
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
func localSourceStamp(spec *schema.VendorComponentSource) string {
	root, ok := localSourceRoot(spec)
	if !ok {
		return ""
	}
	hash := sha256.New()
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
// workdir recorded its stamp. An explicit TTL stays authoritative, and a workdir without a stamp
// (remote source, or provisioned before stamps existed) is never reported as changed here; URI,
// version, and TTL decide for those.
func localSourceChanged(metadata *workdir.WorkdirMetadata, spec *schema.VendorComponentSource) (bool, string) {
	if metadata.SourceStamp == "" || spec.TTL != "" {
		return false, ""
	}
	if localSourceStamp(spec) == metadata.SourceStamp {
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
