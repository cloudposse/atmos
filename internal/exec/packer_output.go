package exec

import (
	"fmt"
	"path/filepath"

	log "github.com/charmbracelet/log"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// ExecutePackerOutput executes `atmos packer output` commands.
func ExecutePackerOutput(
	info *schema.ConfigAndStacksInfo,
) error {
	atmosConfig, err := cfg.InitCliConfig(*info, true)
	if err != nil {
		return err
	}

	*info, err = ProcessStacks(&atmosConfig, *info, true, true, true, nil)
	if err != nil {
		return err
	}

	if len(info.Stack) < 1 {
		return errUtils.ErrMissingStack
	}

	if !info.ComponentIsEnabled {
		log.Info("Component is not enabled and skipped", "component", info.ComponentFromArg)
		return nil
	}

	// Check if the component exists as a Packer component.
	componentPath := filepath.Join(atmosConfig.PackerDirAbsolutePath, info.ComponentFolderPrefix, info.FinalComponent)
	componentPathExists, err := u.IsDirectory(componentPath)
	if err != nil || !componentPathExists {
		return fmt.Errorf("%w: Atmos component `%s` points to the Packer component `%s`, but it does not exist in `%s`",
			errUtils.ErrInvalidComponent,
			info.ComponentFromArg,
			info.FinalComponent,
			filepath.Join(atmosConfig.Components.Packer.BasePath, info.ComponentFolderPrefix),
		)
	}

	// Find Packer manifest file name.
	manifest, err := GetPackerManifestFromVars(&info.ComponentVarsSection)
	if err != nil {
		return err
	}

	if manifest == "" {
		manifest = "manifest.json"
	}

	return nil
}
