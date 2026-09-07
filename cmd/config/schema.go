package config

import (
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/datafetcher"
	"github.com/cloudposse/atmos/pkg/perf"
)

// configSchemaSource is the embedded atmos.yaml JSON Schema — generated from the
// Go configuration structs (see pkg/config/schema) and the same one
// `atmos validate schema` uses for atmos.yaml by default.
const configSchemaSource = "atmos://schema/atmos/config/1.0"

// schemaOutputDirPerm and schemaOutputFilePerm are the permissions used when
// writing the schema to output-path: a non-sensitive project file (JSON Schema
// document), safe to be world-readable.
const (
	schemaOutputDirPerm  = 0o755
	schemaOutputFilePerm = 0o644
)

var configSchemaCmd = &cobra.Command{
	Use:   "schema [output-path]",
	Short: "Print the atmos.yaml JSON Schema",
	Long: `Print the embedded JSON Schema for the Atmos CLI configuration (atmos.yaml) — the
same schema Atmos uses to validate atmos.yaml, generated from the configuration
code itself so it is always current. Prints to stdout, or writes to output-path
if given.`,
	Example: "atmos config schema\natmos config schema website/static/schemas/atmos/atmos-config/1.0/atmos-config.json",
	Args:    cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "config.schemaRunE")()
		return runConfigSchema(args)
	},
}

func runConfigSchema(args []string) error {
	schemaBytes, err := datafetcher.NewDataFetcher(nil).GetData(configSchemaSource)
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
