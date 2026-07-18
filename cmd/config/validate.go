package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/validation"
)

// configSchemaKey is the well-known `schemas:` registry key that targets the
// atmos.yaml schema — the built-in entry `atmos validate schema` seeds by
// default (see internal/exec).
const configSchemaKey = "config"

var configValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate atmos.yaml against its JSON Schema",
	Long: `Validate the Atmos CLI configuration — atmos.yaml, atmos.d fragments, and
project-local profiles — against the JSON Schema generated from the Atmos
configuration code. This is an alias for ` + "`atmos validate schema config`" + `.`,
	Example: "atmos config validate",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "config.validateRunE")()
		return runConfigValidateCommand(cmd)
	},
}

func init() {
	configValidateCmd.Flags().Bool("affected", false, "Validate only configuration files affected since the Git merge-base")
	configValidateCmd.Flags().String("base", "", "Git base ref or SHA to compare against for affected validation")
	configValidateCmd.Flags().StringSlice("exclude", nil, "Exclude repository paths from validation (glob; can be repeated)")
}

func runConfigValidateCommand(cmd *cobra.Command) error {
	affected, err := cmd.Flags().GetBool("affected")
	if err != nil {
		return err
	}
	if !affected {
		excludes, err := cmd.Flags().GetStringSlice("exclude")
		if err != nil {
			return err
		}
		return runConfigValidate(excludes...)
	}
	base, err := cmd.Flags().GetString("base")
	if err != nil {
		return err
	}
	paths, err := validation.AffectedFiles(base)
	if err != nil {
		return err
	}
	excludes, err := cmd.Flags().GetStringSlice("exclude")
	if err != nil {
		return err
	}
	paths, err = validation.ExcludePaths(paths, excludes)
	if err != nil {
		return err
	}
	configFiles := make([]string, 0, len(paths))
	for _, path := range paths {
		if !validation.IsAtmosConfigPath(path) {
			continue
		}
		if _, err := os.Stat(filepath.FromSlash(path)); err == nil {
			configFiles = append(configFiles, path)
		}
	}
	if len(configFiles) == 0 {
		_, err := fmt.Fprintln(cmd.OutOrStdout(), "No affected Atmos configuration files to validate.")
		return err
	}
	return runConfigValidateForFiles(configFiles)
}

func runConfigValidate(excludes ...string) error {
	if len(excludes) == 0 {
		return runConfigValidateForFiles(nil)
	}
	atmosConfig := atmosConfigPtr
	if atmosConfig == nil {
		atmosConfig = &schema.AtmosConfiguration{}
	}
	return exec.NewAtmosValidatorExecutor(atmosConfig).ExecuteAtmosValidateSchemaCmdExcluding(configSchemaKey, "", excludes)
}

func runConfigValidateForFiles(files []string) error {
	atmosConfig := atmosConfigPtr
	if atmosConfig == nil {
		atmosConfig = &schema.AtmosConfiguration{}
	}

	executor := exec.NewAtmosValidatorExecutor(atmosConfig)
	var err error
	if files == nil {
		err = executor.ExecuteAtmosValidateSchemaCmd(configSchemaKey, "")
	} else {
		err = executor.ExecuteAtmosValidateSchemaCmdForFiles(configSchemaKey, "", files)
	}
	if err != nil {
		if errors.Is(err, exec.ErrInvalidYAML) {
			errUtils.OsExit(1)
		}
		return err
	}
	return nil
}
