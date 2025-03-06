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

func newModelComponentVendorInternal(pkgs []pkgComponentVendor, dryRun bool, atmosConfig schema.AtmosConfiguration) (modelVendor, error) {
	p := progress.New(
		progress.WithDefaultGradient(),
		progress.WithWidth(30),
		progress.WithoutPercentage(),
	)
	s := spinner.New()
	s.Style = theme.Styles.Link
	if len(pkgs) == 0 {
		return modelVendor{done: true}, nil
	}
	vendorPks := []pkgVendor{}
	for _, pkg := range pkgs {
		vendorPkg := pkgVendor{
			name:             pkg.name,
			version:          pkg.version,
			componentPackage: &pkg,
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
	}, nil
}

func downloadComponentAndInstall(p *pkgComponentVendor, dryRun bool, atmosConfig schema.AtmosConfiguration) tea.Cmd {
	return func() tea.Msg {
		if dryRun {
			// Simulate the action
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
			err:  fmt.Errorf("unknown package type %s package %s", p.pkgType.String(), p.name),
			name: p.name,
		}
	}
}

func installComponent(p *pkgComponentVendor, atmosConfig schema.AtmosConfiguration) error {
	// Create temp folder
	// We are using a temp folder for the following reasons:
	// 1. 'git' does not clone into an existing folder (and we have the existing component folder with `component.yaml` in it)
	// 2. We have the option to skip some files we don't need and include only the files we need when copying from the temp folder to the destination folder
	tempDir, err := os.MkdirTemp("", fmt.Sprintf("atmos-vendor-%d-*", time.Now().Unix()))
	if err != nil {
		return fmt.Errorf("Failed to create temp directory %s", err)
	}

	// Ensure directory permissions are restricted
	if err := os.Chmod(tempDir, 0o700); err != nil {
		return fmt.Errorf("failed to set temp directory permissions: %w", err)
	}

	defer removeTempDir(atmosConfig, tempDir)

	switch p.pkgType {
	case pkgTypeRemote:
		tempDir = filepath.Join(tempDir, SanitizeFileName(p.uri))

		if err = GoGetterGet(&atmosConfig, p.uri, tempDir, getter.ClientModeAny, 10*time.Minute); err != nil {
			return fmt.Errorf("failed to download package %s: %w", p.name, err) //nolint:err113
		}

	case pkgTypeOci:
		// Download the Image from the OCI-compatible registry, extract the layers from the tarball, and write to the destination directory
		err = processOciImage(atmosConfig, p.uri, tempDir)
		if err != nil {
			return fmt.Errorf("Failed to process OCI image %s error %s", p.name, err) //nolint:err113
		}

	case pkgTypeLocal:
		copyOptions := cp.Options{
			PreserveTimes: false,
			PreserveOwner: false,
			// OnSymlink specifies what to do on symlink
			// Override the destination file if it already exists
			OnSymlink: func(src string) cp.SymlinkAction {
				return cp.Deep
			},
		}

		tempDir2 := tempDir
		if p.sourceIsLocalFile {
			tempDir2 = filepath.Join(tempDir, SanitizeFileName(p.uri))
		}

		if err = cp.Copy(p.uri, tempDir2, copyOptions); err != nil {
			return fmt.Errorf("failed to copy package %s: %w", p.name, err)
		}
	default:
		return fmt.Errorf("unknown package type %s package %s", p.pkgType.String(), p.name) //nolint:err113
	}
	if err = copyComponentToDestination(atmosConfig, tempDir, p.componentPath, p.vendorComponentSpec, p.sourceIsLocalFile, p.uri); err != nil {
		return fmt.Errorf("failed to copy package %s error %s", p.name, err) //nolint:err113
	}

	return nil
}

func installMixin(p *pkgComponentVendor, atmosConfig schema.AtmosConfiguration) error {
	tempDir, err := os.MkdirTemp("", strconv.FormatInt(time.Now().Unix(), 10))
	if err != nil {
		return fmt.Errorf("Failed to create temp directory %s", err)
	}

	defer removeTempDir(atmosConfig, tempDir)

	switch p.pkgType {
	case pkgTypeRemote:
		if err = GoGetterGet(&atmosConfig, p.uri, filepath.Join(tempDir, p.mixinFilename), getter.ClientModeFile, 10*time.Minute); err != nil {
			return fmt.Errorf("failed to download package %s error %s", p.name, err) //nolint:err113
		}

	case pkgTypeOci:
		// Download the Image from the OCI-compatible registry, extract the layers from the tarball, and write to the destination directory
		err = processOciImage(atmosConfig, p.uri, tempDir)
		if err != nil {
			return fmt.Errorf("failed to process OCI image %s error %s", p.name, err)
		}

	case pkgTypeLocal:
		if p.uri == "" {
			return fmt.Errorf("local mixin URI cannot be empty")
		}
		// Implement local mixin installation logic
		return fmt.Errorf("local mixin installation not implemented")

	default:
		return fmt.Errorf("unknown package type %s package %s", p.pkgType.String(), p.name)
	}

	// Copy from the temp folder to the destination folder
	copyOptions := cp.Options{
		// Preserve the atime and the mtime of the entries
		PreserveTimes: false,

		// Preserve the uid and the gid of all entries
		PreserveOwner: false,

		// OnSymlink specifies what to do on symlink
		// Override the destination file if it already exists
		// Prevent the error:
		// symlink components/terraform/mixins/context.tf components/terraform/infra/vpc-flow-logs-bucket/context.tf: file exists
		OnSymlink: func(src string) cp.SymlinkAction {
			return cp.Deep
		},
	}

	if err = cp.Copy(tempDir, p.componentPath, copyOptions); err != nil {
		return fmt.Errorf("Failed to copy package %s error %s", p.name, err)
	}

	return nil
}
