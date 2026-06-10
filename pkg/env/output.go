// Package env provides unified environment variable formatting across multiple output formats.
package env

import (
	"encoding/json"
	"fmt"
	"os"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

const (
	// DefaultOutputFileMode is the default file mode for output files (0o644).
	DefaultOutputFileMode = 0o644
	// CredentialFileMode is used for files containing credentials (0o600).
	CredentialFileMode = 0o600
)

// OutputOption configures Output behavior.
type OutputOption func(*outputConfig)

type outputConfig struct {
	fileMode    os.FileMode
	atmosConfig *schema.AtmosConfiguration
	formatOpts  []Option // Options to pass to FormatData (e.g., WithExport).
}

// WithFileMode sets the file permission mode for file output.
// Use CredentialFileMode (0o600) for sensitive data.
func WithFileMode(mode os.FileMode) OutputOption {
	defer perf.Track(nil, "env.WithFileMode")()

	return func(c *outputConfig) {
		c.fileMode = mode
	}
}

// WithAtmosConfig provides config for JSON formatting (pretty print settings).
func WithAtmosConfig(cfg *schema.AtmosConfiguration) OutputOption {
	defer perf.Track(nil, "env.WithAtmosConfig")()

	return func(c *outputConfig) {
		c.atmosConfig = cfg
	}
}

// WithFormatOptions passes format-specific options to FormatData.
// Use this to pass options like WithExport, WithUppercase, WithFlatten.
func WithFormatOptions(opts ...Option) OutputOption {
	defer perf.Track(nil, "env.WithFormatOptions")()

	return func(c *outputConfig) {
		c.formatOpts = opts
	}
}

// Output writes environment variables in the specified format to the destination.
// If outputFile is empty, writes to stdout.
// Supported formats: bash, dotenv, env, github, json.
//
// This is the primary entry point for environment variable output. It handles:
//   - Format parsing and validation
//   - Map type conversion (accepts map[string]string for convenience)
//   - Output routing (stdout vs file)
//   - File permissions via WithFileMode option
//   - JSON formatting via WithAtmosConfig option
func Output(data map[string]string, formatStr string, outputFile string, opts ...OutputOption) error {
	defer perf.Track(nil, "env.Output")()

	// Apply options.
	cfg := &outputConfig{
		fileMode: DefaultOutputFileMode,
	}
	for _, opt := range opts {
		opt(cfg)
	}

	// Handle JSON format separately (not part of standard env formats).
	if formatStr == "json" {
		return outputJSON(data, outputFile, cfg)
	}

	// Parse format.
	format, err := ParseFormat(formatStr)
	if err != nil {
		return err
	}

	// Convert map[string]string to map[string]any for FormatData.
	anyData := ConvertMapStringToAny(data)

	// Format the data with any format-specific options.
	formatted, err := FormatData(anyData, format, cfg.formatOpts...)
	if err != nil {
		return err
	}

	// Output to file or stdout.
	if outputFile != "" {
		return writeToFileWithMode(outputFile, formatted, cfg.fileMode)
	}

	// lgtm[go/clear-text-logging] - Intentional stdout output for shell evaluation.
	fmt.Print(formatted)
	return nil
}

// outputJSON handles JSON format output to file or stdout.
func outputJSON(data map[string]string, outputFile string, cfg *outputConfig) error {
	if outputFile != "" {
		return u.WriteToFileAsJSON(outputFile, data, cfg.fileMode)
	}

	// For stdout, use PrintAsJSON if atmosConfig is available for pretty printing,
	// otherwise fall back to standard JSON encoding.
	if cfg.atmosConfig != nil {
		return u.PrintAsJSON(cfg.atmosConfig, data)
	}

	// Fallback: encode JSON directly to stdout.
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(data)
}

// writeToFileWithMode writes content to a file with specified permissions.
// Creates the file if it doesn't exist, appends if it does.
func writeToFileWithMode(filePath string, content string, mode os.FileMode) error {
	f, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, mode)
	if err != nil {
		return errUtils.Build(errUtils.ErrOpenFile).
			WithCause(err).
			WithContext("path", filePath).
			Err()
	}
	defer f.Close()

	if _, err := f.WriteString(content); err != nil {
		return errUtils.Build(errUtils.ErrWriteFile).
			WithCause(err).
			WithContext("path", filePath).
			Err()
	}
	return nil
}
