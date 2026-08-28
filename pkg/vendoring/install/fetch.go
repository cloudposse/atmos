package install

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/hashicorp/go-getter"
	cp "github.com/otiai10/copy"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/downloader"
	"github.com/cloudposse/atmos/pkg/oci"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/cloudposse/atmos/pkg/vendor"
)

// fetchTimeout bounds every remote fetch (go-getter or OCI) at the same 10 minutes the
// pre-unification pkgAtmosVendor.installer/installComponent/installMixin dispatch blocks used.
const fetchTimeout = 10 * time.Minute

// ociScheme is the URI prefix marking an OCI-registry source. Stripped before a fetch, and
// restored by lockDeclaredSource before the source is recorded in vendor.lock.yaml.
const ociScheme = "oci://"

// fetchOptions configures fetchToTempDir's per-installer-type behavior. AtmosVendorInstaller and
// componentVendorInstaller share the same pkgType dispatch but differ in a few narrow respects
// preserved here exactly as the pre-unification code did (see each field's doc comment).
type fetchOptions struct {
	// ClientMode is the go-getter client mode for a PkgTypeRemote fetch: ClientModeAny for
	// atmos-vendor and component sources, ClientModeFile for mixins (a mixin's target is a
	// single named file, not a directory).
	ClientMode downloader.ClientMode
	// Target, when non-empty, is used verbatim as the fetch destination instead of tempDir itself,
	// for both a PkgTypeRemote (go-getter) and a PkgTypeLocal (filesystem copy) fetch -- a mixin's
	// fetch, remote or local, always writes to tempDir/<mixinFilename> so the whole-tempDir copy
	// that follows lands it at the destination under that exact name.
	Target string
	// JoinSanitizedFilename, when true, joins tempDir with vendor.SanitizeFileName(uri) before a
	// PkgTypeRemote fetch. component.yaml's non-mixin install path does this (installComponent's
	// pre-unification behavior); vendor.yaml sources and mixins do not.
	JoinSanitizedFilename bool
	// SourceIsLocalFile, when true, joins tempDir with vendor.SanitizeFileName(uri) before a
	// PkgTypeLocal copy -- shared verbatim between the pre-unification atmos-vendor and
	// component-source local-copy branches, which were already identical.
	SourceIsLocalFile bool
	// Retry carries a source's configured retry policy through to the go-getter downloader.
	Retry *schema.RetryConfig
}

// fetchToTempDir fetches uri (classified by pType) into tempDir, dispatching to go-getter
// (PkgTypeRemote), OCI (PkgTypeOci), or a local filesystem copy (PkgTypeLocal) -- the single
// shared implementation of the pkgType dispatch previously duplicated across
// pkgAtmosVendor.installer, installComponent, and installMixin. Returns the directory the caller
// should treat as containing the fetched content (tempDir itself, unless a subdirectory/file join
// applied: Target, JoinSanitizedFilename, or SourceIsLocalFile) and best-effort HTTP cache
// metadata (ETag/Last-Modified), which is only ever non-empty for a PkgTypeRemote fetch of an
// actual HTTP(S) source -- OCI and local copies have no HTTP response to observe, and a
// PkgTypeRemote git fetch naturally yields zero-value metadata too (no HTTP response involved).
//
//nolint:revive // argument-limit: ctx, atmosConfig, uri, pType, and tempDir are each independently required by the pkgType dispatch; opts already bundles the installer-specific variance.
func fetchToTempDir(ctx context.Context, atmosConfig *schema.AtmosConfiguration, uri string, pType PkgType, tempDir string, opts fetchOptions) (string, downloader.FetchMetadata, error) {
	switch pType {
	case PkgTypeRemote:
		return fetchRemote(atmosConfig, uri, tempDir, opts)
	case PkgTypeOci:
		dir, err := fetchOCI(ctx, atmosConfig, uri, tempDir)
		return dir, downloader.FetchMetadata{}, err
	case PkgTypeLocal:
		dir, err := fetchLocal(uri, tempDir, opts)
		return dir, downloader.FetchMetadata{}, err
	default:
		return "", downloader.FetchMetadata{}, fmt.Errorf("%w %s", errUtils.ErrUnknownPackageType, pType.String())
	}
}

func fetchRemote(atmosConfig *schema.AtmosConfiguration, uri, tempDir string, opts fetchOptions) (string, downloader.FetchMetadata, error) {
	target := tempDir
	switch {
	case opts.Target != "":
		target = opts.Target
	case opts.JoinSanitizedFilename:
		target = filepath.Join(tempDir, vendor.SanitizeFileName(uri))
	}

	ggOpts := []downloader.GoGetterOption{}
	if opts.Retry != nil {
		ggOpts = append(ggOpts, downloader.WithRetryConfig(opts.Retry))
	}
	metadata, err := downloader.NewGoGetterDownloader(atmosConfig, ggOpts...).FetchWithMetadata(uri, target, opts.ClientMode, fetchTimeout)
	if err != nil {
		return "", downloader.FetchMetadata{}, fmt.Errorf("%w: %w", ErrDownloadPackage, err)
	}
	return target, metadata, nil
}

func fetchOCI(ctx context.Context, atmosConfig *schema.AtmosConfiguration, uri, tempDir string) (string, error) {
	ociCtx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()
	if err := oci.ProcessImage(ociCtx, atmosConfig, uri, tempDir); err != nil {
		return "", fmt.Errorf("%w: %w", ErrProcessOCIImage, err)
	}
	return tempDir, nil
}

func fetchLocal(uri, tempDir string, opts fetchOptions) (string, error) {
	target := tempDir
	switch {
	case opts.Target != "":
		target = opts.Target
	case opts.SourceIsLocalFile:
		target = filepath.Join(tempDir, vendor.SanitizeFileName(uri))
	}
	copyOptions := cp.Options{
		PreserveTimes: false,
		PreserveOwner: false,
		OnSymlink:     func(string) cp.SymlinkAction { return cp.Deep },
	}
	if err := cp.Copy(uri, target, copyOptions); err != nil {
		return "", fmt.Errorf("%w: %w", ErrCopyPackage, err)
	}
	return target, nil
}

// lockDeclaredSource restores the "oci://" scheme stripped before a fetch, so vendor.lock.yaml
// records the original declared source rather than a scheme-less URI that would degrade OCI
// sources to a plain file-tree hash for provenance resolution.
func lockDeclaredSource(kind PkgType, uri string) string {
	if kind == PkgTypeOci && !strings.HasPrefix(uri, ociScheme) {
		return ociScheme + uri
	}
	return uri
}

// needsCustomDetection replicates go-getter's unexported getForce detection-need check, so
// dry-run only probes the custom Git detector for the URI schemes a real fetch would also need
// it for, instead of running detection for every link being vendored.
func needsCustomDetection(src string) bool {
	_, getSrc := "", src
	if idx := strings.Index(src, "::"); idx >= 0 {
		_, getSrc = src[:idx], src[idx+2:]
	}

	getSrc, _ = getter.SourceDirSubdir(getSrc)

	if absPath, err := filepath.Abs(getSrc); err == nil {
		if u.FileExists(absPath) {
			return false
		}
		isDir, err := u.IsDirectory(absPath)
		if err == nil && isDir {
			return false
		}
	}

	parsed, err := url.Parse(getSrc)
	if err != nil || parsed.Scheme == "" {
		return true
	}

	supportedSchemes := map[string]bool{
		"http":      true,
		"https":     true,
		"git":       true,
		"hg":        true,
		"s3":        true,
		"gcs":       true,
		"file":      true,
		"oci":       true,
		"ssh":       true,
		"git+ssh":   true,
		"git+https": true,
	}

	if _, ok := supportedSchemes[parsed.Scheme]; ok {
		return false
	}

	return true
}

// detectIfNeeded runs the custom Git detector against uri only when needsCustomDetection reports
// it's required, matching the pre-unification dry-run behavior for both installer types.
func detectIfNeeded(atmosConfig *schema.AtmosConfiguration, uri string) error {
	if !needsCustomDetection(uri) {
		return nil
	}
	detector := downloader.NewCustomGitDetector(atmosConfig, "")
	_, _, err := detector.Detect(uri, "")
	return err
}
