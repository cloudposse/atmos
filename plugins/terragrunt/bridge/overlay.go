package atmosbridge

import (
	"os"
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/spf13/afero"
)

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
		content, err := afero.ReadFile(memory, path)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(name)] = content
		return nil
	})
	return files, err
}
