package install

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cloudposse/atmos/pkg/downloader"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/vendoring/lockfile"
)

// atmosVendorInstaller installs one target of a vendor.yaml source: fetch (via fetchToTempDir),
// copy with vendor.yaml's include/exclude glob patterns (copyToTargetWithPatterns), then record a
// vendor.lock.yaml receipt.
type atmosVendorInstaller struct {
	srcURI     string
	name       string
	targetPath string
	localFile  bool
	pType      PkgType
	version    string
	rawVersion string
	source     schema.AtmosVendorSource
}

// AtmosPackageParams are the fields NewAtmosVendorPackage needs to build one vendor.yaml target's
// VendorPackage. A struct rather than positional parameters: Options Pattern (CLAUDE.md) applies
// once a constructor has more than four parameters.
type AtmosPackageParams struct {
	Name              string
	URI               string
	TargetPath        string
	Version           string
	RawVersion        string
	PkgType           PkgType
	SourceIsLocalFile bool
	Source            schema.AtmosVendorSource
}

// NewAtmosVendorPackage builds a VendorPackage for a single target of a vendor.yaml source.
func NewAtmosVendorPackage(params *AtmosPackageParams) VendorPackage {
	defer perf.Track(nil, "install.NewAtmosVendorPackage")()

	return VendorPackage{
		Name:       params.Name,
		Version:    params.Version,
		RawVersion: params.RawVersion,
		installer: &atmosVendorInstaller{
			srcURI:     params.URI,
			name:       params.Name,
			targetPath: params.TargetPath,
			localFile:  params.SourceIsLocalFile,
			pType:      params.PkgType,
			version:    params.Version,
			rawVersion: params.RawVersion,
			source:     params.Source,
		},
	}
}

// logVendorDiagDir is a TEMPORARY diagnostic aid for tracking down a Windows-only silent
// vendor-pull failure where a package reports installed successfully but its target ends up
// missing files (https://github.com/cloudposse/atmos/pull/2958). Gated behind
// ATMOS_VENDOR_DEBUG_DIAG so it only fires for the one test case currently reproducing it, not
// every vendor pull in the suite. Remove this function and its two call sites in install() once
// root-caused.
func logVendorDiagDir(label, dir string) {
	//nolint:forbidigo // Throwaway diagnostic toggle, not an Atmos config option; removed with this function once root-caused.
	if os.Getenv("ATMOS_VENDOR_DEBUG_DIAG") == "" {
		return
	}
	var files []string
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr // Best-effort listing: skip an inaccessible entry rather than abort the whole walk.
		}
		if d.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(dir, path)
		if relErr != nil {
			rel = path
		}
		files = append(files, rel)
		return nil
	})
	log.Warn("VENDOR_DIAG", "label", label, "dir", dir, "fileCount", len(files), "files", strings.Join(files, ";"))
}

func (p *atmosVendorInstaller) install(ctx context.Context, tempDir string, atmosConfig *schema.AtmosConfiguration) error {
	fetchedDir, metadata, err := fetchToTempDir(ctx, atmosConfig, p.srcURI, p.pType, tempDir, fetchOptions{
		ClientMode:        downloader.ClientModeAny,
		SourceIsLocalFile: p.localFile,
		Retry:             p.source.Retry,
	})
	if err != nil {
		return err
	}
	logVendorDiagDir("after-fetch:"+p.name, fetchedDir)

	if err := copyToTargetWithPatterns(fetchedDir, p.targetPath, &p.source, p.localFile); err != nil {
		return fmt.Errorf("%w: %w", ErrCopyPackage, err)
	}
	logVendorDiagDir("after-copy:"+p.name, p.targetPath)

	recordOpts := lockfile.RecordOptions{
		IncludedPaths: p.source.IncludedPaths,
		ExcludedPaths: p.source.ExcludedPaths,
		HTTPMetadata:  metadata,
	}
	if p.rawVersion != "" {
		recordOpts.VersionConstraint = p.rawVersion
		recordOpts.ResolvedVersion = p.version
	}
	recordTarget := lockfile.RecordTarget{
		Kind:           p.pType.String(),
		Name:           p.name,
		TempDir:        fetchedDir,
		Path:           p.targetPath,
		DeclaredSource: lockDeclaredSource(p.pType, p.srcURI),
	}
	if err := lockfile.Record(ctx, atmosConfig, recordTarget, recordOpts); err != nil {
		return fmt.Errorf("%w: %w", ErrRecordVendorLock, err)
	}
	return nil
}

func (p *atmosVendorInstaller) dryRunCheck(_ context.Context, atmosConfig *schema.AtmosConfiguration) error {
	log.Debug("Entering dry-run flow for generic (non component/mixin) vendoring", "package", p.name)
	if err := detectIfNeeded(atmosConfig, p.srcURI); err != nil {
		return fmt.Errorf("%w: %w", ErrDryRunDetectionFailed, err)
	}
	time.Sleep(500 * time.Millisecond)
	return nil
}

func (p *atmosVendorInstaller) isMixin() bool { return false }

func (p *atmosVendorInstaller) artifactID(atmosConfig *schema.AtmosConfiguration) (string, error) {
	return lockfile.ArtifactID(atmosConfig, p.pType.String(), p.targetPath, p.name)
}

// isMaterialized returns Materialized true only for a copy mode with an exact receipt whose
// declared source, copy-filter patterns, and every destination file still match. Sources that use
// filtered or file copy modes have no exact destination plan yet and are intentionally left
// eligible for re-install.
func (p *atmosVendorInstaller) isMaterialized(atmosConfig *schema.AtmosConfiguration) (lockfile.MaterializationCheck, error) {
	id, err := p.artifactID(atmosConfig)
	if err != nil {
		return lockfile.MaterializationCheck{}, err
	}
	return lockfile.IsMaterialized(atmosConfig, lockfile.MaterializationParams{
		ID:            id,
		Declared:      lockDeclaredSource(p.pType, p.srcURI),
		Target:        p.targetPath,
		IncludedPaths: p.source.IncludedPaths,
		ExcludedPaths: p.source.ExcludedPaths,
	})
}

func (p *atmosVendorInstaller) pkgType() PkgType        { return p.pType }
func (p *atmosVendorInstaller) uri() string             { return p.srcURI }
func (p *atmosVendorInstaller) target() string          { return p.targetPath }
func (p *atmosVendorInstaller) mixinFilename() string   { return "" }
func (p *atmosVendorInstaller) sourceIsLocalFile() bool { return p.localFile }
