package stack

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/validation"
)

var stackValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate stack manifest configurations",
	Long: `Validate the configuration of all stack manifests against the atmos-manifest
JSON Schema — the same one ` + "`atmos stack schema`" + ` prints. This is an alias for
` + "`atmos validate stacks`" + `.`,
	Example: "atmos stack validate",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "stack.validateRunE")()
		affected := false
		if cmd.Flags().Lookup("affected") != nil {
			var err error
			affected, err = cmd.Flags().GetBool("affected")
			if err != nil {
				return err
			}
		}
		if affected {
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
			if !affectedStackValidationApplicable(paths, atmosConfigPtr) {
				_, err := fmt.Fprintln(cmd.OutOrStdout(), "No affected stack manifest files to validate.")
				return err
			}
		}
		excludes, err := cmd.Flags().GetStringSlice("exclude")
		if err != nil {
			return err
		}
		format, _ := cmd.Flags().GetString("format")
		format = strings.ToLower(strings.TrimSpace(format))
		if format != "" && format != "text" && format != "rich" {
			return fmt.Errorf("unsupported validation format %q: expected text or rich", format)
		}
		if format == "rich" {
			stackConfig := withExcludedStackPaths(atmosConfigPtr, excludes)
			err := exec.ValidateStacks(stackConfig)
			if err == nil {
				message := "All stacks validated successfully"
				if len(stackConfig.StackConfigFilesAbsolutePaths) == 0 {
					message = "No stack manifests found to validate"
				}
				ui.Success(message)
				return nil
			}
			root := stackConfig.StacksBaseAbsolutePath
			if root == "" {
				var rootErr error
				root, rootErr = os.Getwd()
				if rootErr != nil {
					return rootErr
				}
			}
			ui.Writeln(validation.Rich(validation.FromGCCText("stacks", err.Error()), validation.DefaultRichOptions(root)))
			return errUtils.ExitCodeError{Code: 1, Silent: true}
		}
		if len(excludes) > 0 {
			return exec.ValidateStacks(withExcludedStackPaths(atmosConfigPtr, excludes))
		}
		return exec.ExecuteValidateStacksCmd(cmd, args)
	},
}

func init() {
	stackValidateCmd.PersistentFlags().String("schemas-atmos-manifest", "", "Specifies the path to a JSON schema file used to validate the structure and content of the Atmos manifest file")
	stackValidateCmd.PersistentFlags().String("format", "", "Output format: text, rich")
	stackValidateCmd.PersistentFlags().Bool("affected", false, "Validate stack manifests affected since the Git merge-base")
	stackValidateCmd.PersistentFlags().String("base", "", "Git base ref or SHA to compare against for affected validation")
	stackValidateCmd.PersistentFlags().StringSlice("exclude", nil, "Exclude repository paths from validation (glob; can be repeated)")
}

func withExcludedStackPaths(atmosConfig *schema.AtmosConfiguration, excludes []string) *schema.AtmosConfiguration {
	if atmosConfig == nil || len(excludes) == 0 {
		return atmosConfig
	}
	copy := *atmosConfig
	copy.Stacks.ExcludedPaths = append([]string{}, atmosConfig.Stacks.ExcludedPaths...)
	stackBase := filepath.ToSlash(filepath.Clean(atmosConfig.Stacks.BasePath))
	for _, exclude := range excludes {
		exclude = filepath.ToSlash(filepath.Clean(exclude))
		if stackBase != "." && strings.HasPrefix(exclude, stackBase+"/") {
			exclude = strings.TrimPrefix(exclude, stackBase+"/")
		}
		copy.Stacks.ExcludedPaths = append(copy.Stacks.ExcludedPaths, exclude)
	}
	return &copy
}

func affectedStackValidationApplicable(paths []string, atmosConfig *schema.AtmosConfiguration) bool {
	if atmosConfig == nil {
		return false
	}
	for _, path := range paths {
		if validation.IsAtmosConfigPath(path) {
			return true
		}
		absolute, err := filepath.Abs(filepath.FromSlash(path))
		if err != nil || atmosConfig.StacksBaseAbsolutePath == "" {
			continue
		}
		relative, err := filepath.Rel(atmosConfig.StacksBaseAbsolutePath, absolute)
		if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
			return true
		}
	}
	return false
}
