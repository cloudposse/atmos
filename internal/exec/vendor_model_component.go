package exec

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/hashicorp/go-getter"
	cp "github.com/otiai10/copy"

	"github.com/cloudposse/atmos/internal/tui/templates/term"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

// pkgComponentVendor defines a vendor package.
type pkgComponentVendor struct {
	uri                 string
	name                string
	sourceIsLocalFile   bool
	pkgType             pkgType
	version             string
	vendorComponentSpec schema.VendorComponentSpec
	componentPath       string
	IsComponent         bool
	IsMixins            bool
	mixinFilename       string
}

// newModelComponentVendorInternal creates a new vendor model.
func newModelComponentVendorInternal(pkgs []pkgComponentVendor, dryRun bool, atmosConfig *schema.AtmosConfiguration) modelVendor {
	p := progress.New(
		progress.WithDefaultGradient(),
		progress.WithWidth(progressWidth),
		progress.WithoutPercentage(),
	)
	s := spinner.New()
	s.Style = theme.Styles.Link
	if len(pkgs) == 0 {
		return modelVendor{
			packages:    nil,
			spinner:     s,
			progress:    p,
			dryRun:      dryRun,
			atmosConfig: atmosConfig,
			isTTY:       term.IsTTYSupportForStdout(),
		}
	}
	vendorPks := []pkgVendor{}
	for i := range pkgs {
		vendorPkg := pkgVendor{
			name:             pkgs[i].name,
			version:          pkgs[i].version,
			componentPackage: &pkgs[i],
		}
		vendorPks = append(vendorPks, vendorPkg)
	}
	tty := term.IsTTYSupportForStdout()
	return modelVendor{
		packages:    vendorPks,
		spinner:     s,
		progress:    p,
		dryRun:      dryRun,
		atmosConfig: atmosConfig,
		isTTY:       tty,
	}
}

// downloadComponentAndInstall returns a command to download and install a component.
func downloadComponentAndInstall(p *pkgComponentVendor, dryRun bool, atmosConfig *schema.AtmosConfiguration) tea.Cmd {
	return func() tea.Msg {
		if dryRun {
			time.Sleep(100 * time.Millisecond)
			return installedPkgMsg{
				err:  nil,
				name: p.name,
			}
		}
		if p.IsComponent {
			err := installComponent(p, atmosConfig)
			if err != nil {
				return installedPkgMsg{
					err:  err,
					name: p.name,
				}
			}
			return installedPkgMsg{
				err:  nil,
				name: p.name,
			}
		}
		if p.IsMixins {
			err := installMixin(p, atmosConfig)
			if err != nil {
				return installedPkgMsg{
					err:  err,
					name: p.name,
				}
			}
			return installedPkgMsg{
				err:  nil,
				name: p.name,
			}
		}
		return installedPkgMsg{
			err:  fmt.Errorf("%w", ErrUnknownPackageType),
			name: p.name,
		}
	}
}

// installComponent downloads and installs a component.
func installComponent(p *pkgComponentVendor, atmosConfig *schema.AtmosConfiguration) error {
	tempDir, err := os.MkdirTemp("", fmt.Sprintf("atmos-vendor-%d-*", time.Now().Unix()))
	if err != nil {
		return fmt.Errorf(wrapErrFmtWithDetails, ErrCreateTempDir, err)
	}

	if err := os.Chmod(tempDir, componentTempDirPermissions); err != nil {
		return fmt.Errorf("failed to set temp directory permissions: %w", err)
	}

	defer removeTempDir(*atmosConfig, tempDir)

	switch p.pkgType {
	case pkgTypeRemote:
		tempDir = filepath.Join(tempDir, SanitizeFileName(p.uri))
		if err = GoGetterGet(atmosConfig, p.uri, tempDir, getter.ClientModeAny, getterTimeout); err != nil {
			return fmt.Errorf(wrapErrFmtWithDetails, ErrDownloadPackage, err)
		}
	case pkgTypeOci:
		err = processOciImage(*atmosConfig, p.uri, tempDir)
		if err != nil {
			return fmt.Errorf(wrapErrFmtWithDetails, ErrProcessOCIImage, err)
		}
	case pkgTypeLocal:
		copyOptions := cp.Options{
			PreserveTimes: false,
			PreserveOwner: false,
			OnSymlink: func(src string) cp.SymlinkAction {
				return cp.Deep
			},
		}

		tempDir2 := tempDir
		if p.sourceIsLocalFile {
			tempDir2 = filepath.Join(tempDir, SanitizeFileName(p.uri))
		}

		if err = cp.Copy(p.uri, tempDir2, copyOptions); err != nil {
			return fmt.Errorf(wrapErrFmtWithDetails, ErrCopyPackage, err)
		}
	default:
		return fmt.Errorf("%w", ErrUnknownPackageType)
	}
	if err = copyComponentToDestination(*atmosConfig, tempDir, p.componentPath, p.vendorComponentSpec, p.sourceIsLocalFile, p.uri); err != nil {
		return fmt.Errorf(wrapErrFmtWithDetails, ErrCopyPackage, err)
	}

	return nil
}

// installMixin downloads and installs a mixin.
func installMixin(p *pkgComponentVendor, atmosConfig *schema.AtmosConfiguration) error {
	tempDir, err := os.MkdirTemp("", strconv.FormatInt(time.Now().Unix(), timeFormatBase))
	if err != nil {
		return fmt.Errorf(wrapErrFmtWithDetails, ErrCreateTempDir, err)
	}

	defer removeTempDir(*atmosConfig, tempDir)

	switch p.pkgType {
	case pkgTypeRemote:
		if err = GoGetterGet(atmosConfig, p.uri, filepath.Join(tempDir, p.mixinFilename), getter.ClientModeFile, getterTimeout); err != nil {
			return fmt.Errorf(wrapErrFmtWithDetails, ErrDownloadPackage, err)
		}
	case pkgTypeOci:
		err = processOciImage(*atmosConfig, p.uri, tempDir)
		if err != nil {
			return fmt.Errorf(wrapErrFmtWithDetails, ErrProcessOCIImage, err)
		}
	case pkgTypeLocal:
		if p.uri == "" {
			return ErrLocalMixinURICannotBeEmpty
		}
		return ErrLocalMixinInstallationNotImplemented
	default:
		return fmt.Errorf("%w", ErrUnknownPackageType)
	}

	copyOptions := cp.Options{
		PreserveTimes: false,
		PreserveOwner: false,
		OnSymlink: func(src string) cp.SymlinkAction {
			return cp.Deep
		},
	}

	if err = cp.Copy(tempDir, p.componentPath, copyOptions); err != nil {
		return fmt.Errorf(wrapErrFmtWithDetails, ErrCopyPackage, err)
	}

	return nil
}
