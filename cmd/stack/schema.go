package stack

import (
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/datafetcher"
	"github.com/cloudposse/atmos/pkg/perf"
)

// manifestSchemaSource is the embedded atmos-manifest JSON Schema — the same one
// `atmos validate stacks` falls back to when no `schemas.atmos.manifest` override is set.
const manifestSchemaSource = "atmos://schema/atmos/manifest/1.0"

// schemaOutputDirPerm and schemaOutputFilePerm are the permissions used when writing
// the schema to output-path: a non-sensitive project file (JSON Schema document), safe
// to be world-readable.
const (
	schemaOutputDirPerm  = 0o755
	schemaOutputFilePerm = 0o644
)

var stackSchemaCmd = &cobra.Command{
	Use:   "schema [output-path]",
	Short: "Print the atmos-manifest JSON Schema",
	Long: `Print the embedded atmos-manifest JSON Schema — the same schema Atmos uses by
default to validate stack manifests. Prints to stdout, or writes to output-path if given.`,
	Example: "atmos stack schema\natmos stack schema website/static/schemas/atmos/atmos-manifest/1.0/atmos-manifest.json",
	Args:    cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "stack.schemaRunE")()
		return runStackSchema(args)
	},
}

func runStackSchema(args []string) error {
	schemaBytes, err := datafetcher.NewDataFetcher(nil).GetData(manifestSchemaSource)
	if err != nil {
		return err
	}

	if len(args) == 0 {
		return data.Write(string(schemaBytes))
	}

	outputPath := args[0]
	if err := os.MkdirAll(filepath.Dir(outputPath), schemaOutputDirPerm); err != nil {
		return errUtils.Build(errUtils.ErrCreateDirectory).
			WithCause(err).
			WithContext("path", filepath.Dir(outputPath)).
			Err()
	}
	if err := os.WriteFile(outputPath, schemaBytes, schemaOutputFilePerm); err != nil { // #nosec G306 -- the JSON Schema is a non-sensitive project file.
		return errUtils.Build(errUtils.ErrWriteFile).
			WithCause(err).
			WithContext("path", outputPath).
			Err()
	}
	return nil
}
