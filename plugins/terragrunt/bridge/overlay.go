package atmosbridge

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/spf13/afero"
)

// ErrUnsupportedOverlay reports a unit file that the Atmos preparation contract
// cannot carry faithfully.
var ErrUnsupportedOverlay = errors.New("unit file cannot be exported through the overlay")

// UnitOverlay applies upstream default copy rules without writing to the unit or module.
// It returns file contents and, for files whose permission bits differ from the
// default, the unit file's mode so executables keep their execute bit and
// owner-only or otherwise unreadable-by-others files, such as copied keys, stay
// private. The host ignores modes on Windows, so the exported modes only take
// effect on platforms with POSIX permission bits.
//
//nolint:lintroller // Standalone compatibility module cannot import the host performance tracker.
func UnitOverlay(logger log.Logger, env *Venv, unit string, terraform *config.TerraformConfig) (map[string][]byte, map[string]uint32, error) {
	memory := afero.NewMemMapFs()
	fsys := afero.NewCopyOnWriteFs(afero.NewReadOnlyFs(env.FS), memory)
	destination := filepath.Join(filepath.Dir(unit), ".atmos-unit-overlay")
	if err := copyOverlayContents(logger, fsys, unit, destination, terraform); err != nil {
		return nil, nil, err
	}
	files := map[string][]byte{}
	modes := map[string]uint32{}
	err := afero.Walk(memory, destination, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		name, err := filepath.Rel(destination, path)
		if err != nil {
			return err
		}
		if name == ".terragrunt-module-manifest" {
			return nil
		}
		if err := checkOverlayFile(name); err != nil {
			return err
		}
		content, err := afero.ReadFile(memory, path)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(name)] = content
		perm := restrictByDirectories(env.FS, unit, filepath.Dir(name), info.Mode().Perm())
		if exportsMode(perm) {
			modes[filepath.ToSlash(name)] = uint32(perm)
		}
		return nil
	})
	return files, modes, err
}

// copyOverlayContents applies Terragrunt's configured copy filters to the temporary overlay.
func copyOverlayContents(logger log.Logger, fsys afero.Fs, unit, destination string, terraform *config.TerraformConfig) error {
	include := []string{".tflint.hcl"}
	var exclude []string
	if terraform.IncludeInCopy != nil {
		include = append(include, (*terraform.IncludeInCopy)...)
	}
	if terraform.ExcludeFromCopy != nil {
		exclude = append(exclude, (*terraform.ExcludeFromCopy)...)
	}
	return util.CopyFolderContents(logger, fsys, unit, destination, ".terragrunt-module-manifest",
		util.WithIncludeInCopy(include...), util.WithExcludeFromCopy(exclude...))
}

// restrictByDirectories conservatively narrows a file's permission bits by the
// subdirectories that contain it. The overlay carries file modes only, so a
// readable file inside a private directory (for example 0644 under 0700) would
// otherwise become readable once the directory is recreated with default modes.
// A group or other class that cannot traverse a containing directory loses all
// access to the file. The unit directory itself is not considered because the
// host owns the working directory's mode.
func restrictByDirectories(fsys afero.Fs, unit, dir string, perm os.FileMode) os.FileMode {
	const groupExecute, groupPermissions = 0o010, 0o070
	const otherExecute, otherPermissions = 0o001, 0o007
	for dir != "." && dir != string(filepath.Separator) && dir != "" {
		if info, err := fsys.Stat(filepath.Join(unit, dir)); err == nil {
			mode := info.Mode().Perm()
			if mode&groupExecute == 0 {
				perm &^= groupPermissions
			}
			if mode&otherExecute == 0 {
				perm &^= otherPermissions
			}
		}
		dir = filepath.Dir(dir)
	}
	return perm
}

// exportsMode reports whether a unit file's permission bits differ from the
// default the host applies to files without a mode (0644): it is executable, or
// group or other lacks read access.
func exportsMode(perm os.FileMode) bool {
	const groupAndOtherRead = 0o044
	const executeBits = 0o111
	return perm&executeBits != 0 || perm&groupAndOtherRead != groupAndOtherRead
}

// checkOverlayFile rejects a committed provider lock file. Atmos manages the
// lock per component instance and the host refuses it as a runtime artifact, so
// failing here reports the unit file by name instead of failing later.
func checkOverlayFile(name string) error {
	if filepath.Base(name) == util.TerraformLockFile {
		return fmt.Errorf("%w: %s is a dependency lock file, which Atmos manages per component instance; list it in exclude_from_copy to proceed without the committed pins", ErrUnsupportedOverlay, filepath.ToSlash(name))
	}
	return nil
}
