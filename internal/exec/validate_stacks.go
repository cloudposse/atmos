package exec

import (
	"fmt"
	"path"
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	cfg "github.com/cloudposse/atmos/pkg/config"
	s "github.com/cloudposse/atmos/pkg/stack"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// ExecuteValidateStacksCmd executes `validate stacks` command
func ExecuteValidateStacksCmd(cmd *cobra.Command, args []string) error {
	info, err := processCommandLineArgs("", cmd, args, nil)
	if err != nil {
		return err
	}

	cliConfig, err := cfg.InitCliConfig(info, true)
	if err != nil {
		return err
	}

	flags := cmd.Flags()

	schemasAtmosManifestFlag, err := flags.GetString("schemas-atmos-manifest")
	if err != nil {
		return err
	}

	if schemasAtmosManifestFlag != "" {
		cliConfig.Schemas.Atmos.Manifest = schemasAtmosManifestFlag
	}

	// Check if the Atmos manifest JSON Schema is configured and the file exists
	// The path to the Atmos manifest JSON Schema can be absolute path or a path relative to the `base_path` setting in `atmos.yaml`
	var atmosManifestJsonSchemaFilePath string

	if cliConfig.Schemas.Atmos.Manifest != "" {
		atmosManifestJsonSchemaFileAbsPath := path.Join(cliConfig.BasePath, cliConfig.Schemas.Atmos.Manifest)

		if u.FileExists(cliConfig.Schemas.Atmos.Manifest) {
			atmosManifestJsonSchemaFilePath = cliConfig.Schemas.Atmos.Manifest
		} else if u.FileExists(atmosManifestJsonSchemaFileAbsPath) {
			atmosManifestJsonSchemaFilePath = atmosManifestJsonSchemaFileAbsPath
		} else {
			return fmt.Errorf("the Atmos JSON Schema file '%s' does not exist.\n"+
				"It can be configured in the 'schemas.atmos.manifest' section in 'atmos.yaml', or provided using the 'ATMOS_SCHEMAS_ATMOS_MANIFEST' "+
				"ENV variable or '--schemas-atmos-manifest' command line argument.\n"+
				"The path to the schema file should be an absolute path or a path relative to the 'base_path' setting in 'atmos.yaml'.",
				cliConfig.Schemas.Atmos.Manifest)
		}
	}

	// Include (process and validate) all YAML files in the `stacks` folder in all subfolders
	includedPaths := []string{"**/*"}
	// Don't exclude any YAML files for validation
	excludedPaths := []string{}
	includeStackAbsPaths, err := u.JoinAbsolutePathWithPaths(cliConfig.StacksBaseAbsolutePath, includedPaths)
	if err != nil {
		return err
	}

	stackConfigFilesAbsolutePaths, _, err := cfg.FindAllStackConfigsInPaths(cliConfig, includeStackAbsPaths, excludedPaths)
	if err != nil {
		return err
	}

	u.LogDebug(cliConfig, fmt.Sprintf("Validating all YAML files in the '%s' folder and all subfolders\n",
		path.Join(cliConfig.BasePath, cliConfig.Stacks.BasePath)))

	var errorMessages []string

	for _, filePath := range stackConfigFilesAbsolutePaths {
		stackConfig, importsConfig, _, err := s.ProcessYAMLConfigFile(
			cliConfig,
			cliConfig.StacksBaseAbsolutePath,
			filePath,
			map[string]map[any]any{},
			nil,
			false,
			false,
			false,
			false,
			map[any]any{},
			map[any]any{},
			atmosManifestJsonSchemaFilePath,
		)
		if err != nil {
			errorMessages = append(errorMessages, err.Error())
		}

		// Process and validate the stack manifest
		componentStackMap := map[string]map[string][]string{}
		_, err = s.ProcessStackConfig(
			cliConfig.StacksBaseAbsolutePath,
			cliConfig.TerraformDirAbsolutePath,
			cliConfig.HelmfileDirAbsolutePath,
			filePath,
			stackConfig,
			false,
			true,
			"",
			componentStackMap,
			importsConfig,
			false)
		if err != nil {
			errorMessages = append(errorMessages, err.Error())
		}
	}

	if len(errorMessages) > 0 {
		return errors.New(strings.Join(errorMessages, "\n\n"))
	}

	return nil
}
