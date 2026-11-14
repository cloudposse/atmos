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
func UnitOverlay(logger log.Logger, env *Venv, unit string, terraform *config.TerraformConfig) (map[string][]byte, error) {
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
		return nil, err
	}
	files := map[string][]byte{}
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
		if err := checkOverlayFile(name, info); err != nil {
			return err
		}
		content, err := afero.ReadFile(memory, path)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(name)] = content
		return nil
	})
	return files, err
}

// checkOverlayFile rejects files whose meaning is lost by the contract. The
// contract carries content only: a committed provider lock file belongs to
// Atmos' per-instance lock management and the host refuses it as a runtime
// artifact, and file modes are not carried, so an executable would be written
// without its execute permission. Failing here reports the unit file by name
// instead of failing later or changing behavior silently.
func checkOverlayFile(name string, info os.FileInfo) error {
	if filepath.Base(name) == util.TerraformLockFile {
		return fmt.Errorf("%w: %s is a dependency lock file, which Atmos manages per component instance; list it in exclude_from_copy to proceed without the committed pins", ErrUnsupportedOverlay, filepath.ToSlash(name))
	}
	if info.Mode()&0o111 != 0 {
		return fmt.Errorf("%w: %s is executable, and file modes are not preserved; list it in exclude_from_copy if only parse-time functions use it", ErrUnsupportedOverlay, filepath.ToSlash(name))
	}
	return nil
}
