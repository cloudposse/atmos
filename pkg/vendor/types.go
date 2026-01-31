package vendor

import "github.com/cloudposse/atmos/pkg/schema"

// pkgType represents the type of package source.
type pkgType int

const (
	pkgTypeRemote pkgType = iota
	pkgTypeOci
	pkgTypeLocal
)

// String returns the string representation of the package type.
func (p pkgType) String() string {
	names := [...]string{"remote", "oci", "local"}
	if p < pkgTypeRemote || p > pkgTypeLocal {
		return "unknown"
	}
	return names[p]
}

// pkgVendor represents a unified package for the TUI model.
type pkgVendor struct {
	name             string
	version          string
	atmosPackage     *pkgAtmosVendor
	componentPackage *pkgComponentVendor
}

// pkgAtmosVendor represents an Atmos vendor package.
type pkgAtmosVendor struct {
	uri               string
	name              string
	targetPath        string
	sourceIsLocalFile bool
	pkgType           pkgType
	version           string
	atmosVendorSource schema.AtmosVendorSource
}

// pkgComponentVendor represents a component vendor package.
type pkgComponentVendor struct {
	uri                 string
	name                string
	sourceIsLocalFile   bool
	pkgType             pkgType
	version             string
	vendorComponentSpec *schema.VendorComponentSpec
	componentPath       string
	IsComponent         bool
	IsMixins            bool
	mixinFilename       string
}

// processTargetsParams holds parameters for processing targets.
type processTargetsParams struct {
	AtmosConfig          *schema.AtmosConfiguration
	IndexSource          int
	Source               *schema.AtmosVendorSource
	TemplateData         struct{ Component, Version string }
	VendorConfigFilePath string
	URI                  string
	PkgType              pkgType
	SourceIsLocalFile    bool
}

// executeVendorOptions holds options for executing vendor operations.
type executeVendorOptions struct {
	atmosConfig          *schema.AtmosConfiguration
	vendorConfigFileName string
	atmosVendorSpec      schema.AtmosVendorSpec
	component            string
	tags                 []string
	dryRun               bool
}

// vendorSourceParams holds parameters for processing vendor sources.
type vendorSourceParams struct {
	atmosConfig          *schema.AtmosConfiguration
	sources              []schema.AtmosVendorSource
	component            string
	tags                 []string
	vendorConfigFileName string
	vendorConfigFilePath string
}

// installedPkgMsg is the message sent when a package installation completes.
type installedPkgMsg struct {
	err  error
	name string
}
