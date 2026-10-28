package source

import (
	"fmt"
	"os"
	"path/filepath"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/edition"
	"github.com/cloudposse/atmos/pkg/schema"
)

const workingDirectoryPrecedence = "components.source.working_directory_precedence"

func enforceSourceWorkdirOwner(config *schema.AtmosConfiguration, section map[string]any, isWorkdir bool) (bool, error) {
	if !isWorkdir || getWorkingDirectoryOverride(section) == "" {
		return false, nil
	}
	return edition.BehaviorChanged(config.Edition, workingDirectoryPrecedence)
}

// sourceWorkdirOverride preserves edition-pinned placement and refuses an implicit migration.
func sourceWorkdirOverride(config *schema.AtmosConfiguration, section map[string]any, legacy string) (string, error) {
	override := getWorkingDirectoryOverride(section)
	if override == "" {
		return legacy, nil
	}
	changed, err := edition.BehaviorChanged(config.Edition, workingDirectoryPrecedence)
	if err != nil {
		return "", err
	}
	if !changed {
		return legacy, nil
	}
	if !filepath.IsAbs(override) {
		override = filepath.Join(config.BasePath, override)
	}
	override, err = filepath.Abs(override)
	if err != nil {
		return "", fmt.Errorf("resolve source working_directory: %w", err)
	}
	old, err := filepath.Abs(legacy)
	if err != nil {
		return "", err
	}
	if override == old {
		return override, nil
	}
	if err := checkSourceWorkdirMigration(old, override); err != nil {
		return "", err
	}
	return override, nil
}

// checkSourceWorkdirMigration prevents switching away from an existing runtime directory.
func checkSourceWorkdirMigration(old, override string) error {
	entries, err := os.ReadDir(old)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("%w: inspect existing source workdir: %w", errUtils.ErrWorkdirProvision, err)
	}
	if len(entries) > 0 {
		return errUtils.Build(errUtils.ErrWorkdirProvision).
			WithExplanation("working_directory now takes precedence, but the previous workdir still contains files").
			WithContext("previous_workdir", old).
			WithContext("requested_workdir", override).
			WithHint("Keep an earlier edition pin to retain the previous path, or explicitly migrate the entire workdir and its Terraform state before removing the old directory. Atmos will not move or discard state automatically.").
			Err()
	}
	return nil
}
