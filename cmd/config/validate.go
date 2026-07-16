package config

import (
	"errors"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
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
		return runConfigValidate()
	},
}

func runConfigValidate() error {
	atmosConfig := atmosConfigPtr
	if atmosConfig == nil {
		atmosConfig = &schema.AtmosConfiguration{}
	}

	if err := exec.NewAtmosValidatorExecutor(atmosConfig).ExecuteAtmosValidateSchemaCmd(configSchemaKey, ""); err != nil {
		if errors.Is(err, exec.ErrInvalidYAML) {
			errUtils.OsExit(1)
		}
		return err
	}
	return nil
}
