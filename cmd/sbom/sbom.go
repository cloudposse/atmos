// Package sbom provides lock-file-backed software bill of materials commands.
package sbom

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/internal"
	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
	"github.com/cloudposse/atmos/pkg/sbom"
	"github.com/cloudposse/atmos/pkg/schema"
)

var atmosConfig *schema.AtmosConfiguration

var (
	errSBOMUploadRequiresCI  = errors.New("SBOM upload requires a detected CI provider")
	errSBOMUploadUnsupported = errors.New("CI provider does not support SBOM upload")
)

// SetAtmosConfig provides the initialized project configuration to SBOM commands.
func SetAtmosConfig(config *schema.AtmosConfiguration) {
	atmosConfig = config
}

var sbomCmd = &cobra.Command{
	Use:   "sbom",
	Short: "Generate software bills of materials from Atmos locks",
}

var generateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generate a CycloneDX or SPDX SBOM from project lock files",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		format, err := cmd.Flags().GetString("format")
		if err != nil {
			return err
		}
		output, err := cmd.Flags().GetString("output")
		if err != nil {
			return err
		}
		includeFiles, err := cmd.Flags().GetBool("include-files")
		if err != nil {
			return err
		}
		scope, err := cmd.Flags().GetString("scope")
		if err != nil {
			return err
		}
		mode, err := cmd.Flags().GetString("mode")
		if err != nil {
			return err
		}
		subjectName, _ := cmd.Flags().GetString("subject-name")
		subjectVersion, _ := cmd.Flags().GetString("subject-version")
		subjectSupplier, _ := cmd.Flags().GetString("subject-supplier")
		upload, err := cmd.Flags().GetBool("upload")
		if err != nil {
			return err
		}
		graph, err := sbom.BuildWithOptions(atmosConfig, sbom.Options{IncludeFiles: includeFiles, Scope: scope, Mode: mode, Subject: sbom.Subject{Name: subjectName, Version: subjectVersion, Supplier: subjectSupplier}})
		if err != nil {
			return err
		}
		content, err := sbom.Render(graph, format)
		if err != nil {
			return err
		}
		if output == "" {
			if err := data.Writeln(string(content)); err != nil {
				return err
			}
		} else if err := os.WriteFile(output, content, 0o644); err != nil { // #nosec G304 -- output is explicitly requested by the user.
			return fmt.Errorf("write SBOM: %w", err)
		}
		if upload {
			ciProvider := ci.Detect()
			if ciProvider == nil {
				return errSBOMUploadRequiresCI
			}
			uploader, ok := ciProvider.(ci.SBOMUploader)
			if !ok {
				return fmt.Errorf("%w: %s", errSBOMUploadUnsupported, ciProvider.Name())
			}
			filename := sbomArtifactFilename(format, output)
			if _, err := uploader.UploadSBOM(cmd.Context(), ci.SBOMReport{Filename: filename, Format: format, Content: content}); err != nil {
				return err
			}
		}
		return nil
	},
}

func sbomArtifactFilename(format, output string) string {
	if output != "" {
		return filepath.Base(output)
	}
	if format == sbom.FormatSPDXJSON {
		return "atmos-sbom.spdx.json"
	}
	return "atmos-sbom.cyclonedx.json"
}

func init() {
	generateCmd.Flags().String("format", sbom.FormatCycloneDXJSON, "SBOM format: cyclonedx-json or spdx-json")
	generateCmd.Flags().String("output", "", "Write the SBOM to this file instead of stdout")
	generateCmd.Flags().Bool("upload", false, "Upload the generated SBOM through the detected native CI provider")
	generateCmd.Flags().Bool("include-files", false, "Include vendor lock file entries as file components")
	generateCmd.Flags().String("scope", sbom.ScopeTerraform, "Inventory scope (currently terraform)")
	generateCmd.Flags().String("mode", sbom.ModeProvenance, "SBOM mode: provenance or ntia")
	generateCmd.Flags().String("subject-name", "", "Subject name (required for --mode ntia)")
	generateCmd.Flags().String("subject-version", "", "Subject version (required for --mode ntia)")
	generateCmd.Flags().String("subject-supplier", "", "Subject supplier (required for --mode ntia)")
	sbomCmd.AddCommand(generateCmd)
	internal.Register(&CommandProvider{})
}

// CommandProvider registers the SBOM command with the command registry.
type CommandProvider struct{}

// GetCommand returns the SBOM root command.
func (CommandProvider) GetCommand() *cobra.Command { return sbomCmd }

// GetName returns the command name.
func (CommandProvider) GetName() string { return "sbom" }

// GetGroup returns the command help group.
func (CommandProvider) GetGroup() string { return "Security" }

// GetFlagsBuilder returns no shared flags.
func (CommandProvider) GetFlagsBuilder() flags.Builder { return nil }

// GetPositionalArgsBuilder returns no positional argument builder.
func (CommandProvider) GetPositionalArgsBuilder() *flags.PositionalArgsBuilder { return nil }

// GetCompatibilityFlags returns no compatibility flags.
func (CommandProvider) GetCompatibilityFlags() map[string]compat.CompatibilityFlag { return nil }

// GetAliases returns no aliases.
func (CommandProvider) GetAliases() []internal.CommandAlias { return nil }

// IsExperimental reports that the command is experimental until the lock-file
// contract has completed its first compatibility cycle.
func (CommandProvider) IsExperimental() bool { return true }
