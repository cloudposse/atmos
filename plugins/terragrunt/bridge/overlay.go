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
// default, the unit file's mode so executables keep their execute bit.
func UnitOverlay(logger log.Logger, env *Venv, unit string, terraform *config.TerraformConfig) (map[string][]byte, map[string]uint32, error) {
	memory := afero.NewMemMapFs()
	fsys := afero.NewCopyOnWriteFs(afero.NewReadOnlyFs(env.FS), memory)
	destination := filepath.Join(filepath.Dir(unit), ".atmos-unit-overlay")
	include := []string{".tflint.hcl"}
	var exclude []string
	if terraform.IncludeInCopy != nil {
		include = append(include, (*terraform.IncludeInCopy)...)
	}
	if terraform.ExcludeFromCopy != nil {
		exclude = append(exclude, (*terraform.ExcludeFromCopy)...)
	}
	if err := util.CopyFolderContents(logger, fsys, unit, destination, ".terragrunt-module-manifest",
		util.WithIncludeInCopy(include...), util.WithExcludeFromCopy(exclude...)); err != nil {
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
		// The lock file beside the unit is handed to the host as a unit file copy with its own
		// ownership rules, so the overlay leaves it out rather than rejecting it.
		if filepath.ToSlash(name) == util.TerraformLockFile {
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
		if perm := info.Mode().Perm(); perm&0o111 != 0 {
			modes[filepath.ToSlash(name)] = uint32(perm)
		}
		return nil
	})
	return files, modes, err
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
