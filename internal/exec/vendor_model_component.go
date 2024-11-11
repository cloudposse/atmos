package exec

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/hashicorp/go-getter"
	cp "github.com/otiai10/copy"
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

func newModelComponentVendorInternal(pkgs []pkgComponentVendor, dryRun bool, cliConfig schema.CliConfiguration) (modelVendor, error) {
	p := progress.New(
		progress.WithDefaultGradient(),
		progress.WithWidth(30),
		progress.WithoutPercentage(),
	)
	s := spinner.New()
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("63"))
	if len(pkgs) == 0 {
		return modelVendor{}, nil
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
	tty := CheckTTYSupport()
	return modelVendor{
		packages:  vendorPks,
		spinner:   s,
		progress:  p,
		dryRun:    dryRun,
		cliConfig: cliConfig,
		isTTY:     tty,
	}, nil
}

func downloadComponentAndInstall(p *pkgComponentVendor, dryRun bool, cliConfig schema.CliConfiguration) tea.Cmd {
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
			err := installComponent(p, cliConfig)
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
			err := installMixin(p, cliConfig)
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
		u.LogTrace(cliConfig, fmt.Sprintf("Unknown install operation for %s", p.name))
		return installedPkgMsg{
			err:  fmt.Errorf("unknown install operation"),
			name: p.name,
		}
	}
}
func installComponent(p *pkgComponentVendor, cliConfig schema.CliConfiguration) error {

	// Create temp folder
	// We are using a temp folder for the following reasons:
	// 1. 'git' does not clone into an existing folder (and we have the existing component folder with `component.yaml` in it)
	// 2. We have the option to skip some files we don't need and include only the files we need when copying from the temp folder to the destination folder
	tempDir, err := os.MkdirTemp("", strconv.FormatInt(time.Now().Unix(), 10))
	if err != nil {
		u.LogTrace(cliConfig, fmt.Sprintf("Failed to create temp directory %s", err))
		return err
	}

	defer removeTempDir(cliConfig, tempDir)

	switch p.pkgType {
	case pkgTypeRemote:
		tempDir = path.Join(tempDir, filepath.Base(p.uri))

		client := &getter.Client{
			Ctx: context.Background(),
			// Define the destination where the files will be stored. This will create the directory if it doesn't exist
			Dst: tempDir,
			// Source
			Src:  p.uri,
			Mode: getter.ClientModeAny,
		}

		if err = client.Get(); err != nil {
			u.LogTrace(cliConfig, fmt.Sprintf("Failed to download package %s error %s", p.name, err))
			return err
		}

	case pkgTypeOci:
		// Download the Image from the OCI-compatible registry, extract the layers from the tarball, and write to the destination directory
		err = processOciImage(cliConfig, p.uri, tempDir)
		if err != nil {
			u.LogTrace(cliConfig, fmt.Sprintf("Failed to process OCI image %s error %s", p.name, err))
			return err
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
			tempDir2 = path.Join(tempDir, filepath.Base(p.uri))
		}

		if err = cp.Copy(p.uri, tempDir2, copyOptions); err != nil {
			u.LogTrace(cliConfig, fmt.Sprintf("Failed to copy package %s error %s", p.name, err))
			return err
		}
	default:
		u.LogTrace(cliConfig, fmt.Sprintf("Unknown package type %s", p.name))
		return fmt.Errorf("unknown package type")

	}
	if err = copyComponentToDestination(cliConfig, tempDir, p.componentPath, p.vendorComponentSpec, p.sourceIsLocalFile, p.uri); err != nil {
		u.LogTrace(cliConfig, fmt.Sprintf("Failed to copy package %s error %s", p.name, err))
		return err
	}

	return nil

}
func installMixin(p *pkgComponentVendor, cliConfig schema.CliConfiguration) error {
	tempDir, err := os.MkdirTemp("", strconv.FormatInt(time.Now().Unix(), 10))
	if err != nil {
		u.LogTrace(cliConfig, fmt.Sprintf("Failed to create temp directory %s", err))
		return err
	}

	defer removeTempDir(cliConfig, tempDir)
	switch p.pkgType {
	case pkgTypeRemote:
		client := &getter.Client{
			Ctx:  context.Background(),
			Dst:  path.Join(tempDir, p.mixinFilename),
			Src:  p.uri,
			Mode: getter.ClientModeFile,
		}

		if err = client.Get(); err != nil {
			u.LogTrace(cliConfig, fmt.Sprintf("Failed to download package %s error %s", p.name, err))
			return err
		}
	case pkgTypeOci:
		// Download the Image from the OCI-compatible registry, extract the layers from the tarball, and write to the destination directory
		err = processOciImage(cliConfig, p.uri, tempDir)
		if err != nil {
			u.LogTrace(cliConfig, fmt.Sprintf("Failed to process OCI image %s error %s", p.name, err))
			return err
		}
	case pkgTypeLocal:
		return nil

	default:
		u.LogTrace(cliConfig, fmt.Sprintf("Unknown package type %s", p.name))
		return fmt.Errorf("unknown package type")

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
		u.LogTrace(cliConfig, fmt.Sprintf("Failed to copy package %s error %s", p.name, err))
		return err
	}

	return nil
}
