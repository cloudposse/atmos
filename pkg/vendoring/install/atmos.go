package install

import (
	"context"
	"fmt"
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

func (p *atmosVendorInstaller) install(ctx context.Context, tempDir string, atmosConfig *schema.AtmosConfiguration) error {
	fetchedDir, metadata, err := fetchToTempDir(ctx, atmosConfig, p.srcURI, p.pType, tempDir, fetchOptions{
		ClientMode:        downloader.ClientModeAny,
		SourceIsLocalFile: p.localFile,
		Retry:             p.source.Retry,
	})
	if err != nil {
		return err
	}

	if err := copyToTargetWithPatterns(fetchedDir, p.targetPath, &p.source, p.localFile); err != nil {
		return fmt.Errorf("failed to copy package: %w", err)
	}

	recordOpts := lockfile.RecordOptions{
		IncludedPaths: p.source.IncludedPaths,
		ExcludedPaths: p.source.ExcludedPaths,
		HTTPMetadata:  metadata,
	}
	if p.rawVersion != "" {
		recordOpts.VersionConstraint = p.rawVersion
		recordOpts.ResolvedVersion = p.version
	}
	if err := lockfile.Record(ctx, atmosConfig, p.pType.String(), p.name, fetchedDir, p.targetPath, lockDeclaredSource(p.pType, p.srcURI), recordOpts); err != nil {
		return fmt.Errorf("failed to record vendor lock: %w", err)
	}
	return nil
}

func (p *atmosVendorInstaller) dryRunCheck(_ context.Context, atmosConfig *schema.AtmosConfiguration) error {
	log.Debug("Entering dry-run flow for generic (non component/mixin) vendoring", "package", p.name)
	if err := detectIfNeeded(atmosConfig, p.srcURI); err != nil {
		return fmt.Errorf("dry-run: detection failed: %w", err)
	}
	time.Sleep(500 * time.Millisecond)
	return nil
}

func (p *atmosVendorInstaller) isMixin() bool { return false }

func (p *atmosVendorInstaller) artifactID() string {
	return lockfile.ArtifactID(p.pType.String(), p.targetPath, p.name)
}

// isMaterialized returns Materialized true only for a copy mode with an exact receipt whose
// declared source, copy-filter patterns, and every destination file still match. Sources that use
// filtered or file copy modes have no exact destination plan yet and are intentionally left
// eligible for re-install.
func (p *atmosVendorInstaller) isMaterialized(atmosConfig *schema.AtmosConfiguration) (lockfile.MaterializationCheck, error) {
	return lockfile.IsMaterialized(atmosConfig, lockfile.MaterializationParams{
		ID:            p.artifactID(),
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
