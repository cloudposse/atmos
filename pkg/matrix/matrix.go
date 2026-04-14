package matrix

import (
	"encoding/json"
	"fmt"
	"os"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/data"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

// File permissions for matrix output files.
// Matrix output contains only stack/component names, not secrets.
const defaultFilePermissions = 0o644

// Output represents the GitHub Actions matrix strategy format.
type Output struct {
	Include []Entry `json:"include"`
}

// Entry represents a single entry in the matrix include array.
type Entry struct {
	Stack         string `json:"stack"`
	Component     string `json:"component"`
	ComponentPath string `json:"component_path"`
	ComponentType string `json:"component_type"`
}

// Marshal serializes matrix entries to compact JSON.
// Nil entries are normalized to an empty slice to produce {"include":[]} instead of {"include":null}.
func Marshal(entries []Entry) ([]byte, error) {
	defer perf.Track(nil, "matrix.Marshal")()

	include := entries
	if include == nil {
		include = []Entry{}
	}
	output := Output{
		Include: include,
	}
	return json.Marshal(output)
}

// WriteOutput writes the matrix output to stdout or a file.
// If outputFile is specified (for $GITHUB_OUTPUT), writes in key=value format.
// Otherwise, writes JSON to stdout.
func WriteOutput(entries []Entry, outputFile string) error {
	defer perf.Track(nil, "matrix.WriteOutput")()

	matrixJSON, err := Marshal(entries)
	if err != nil {
		return fmt.Errorf("%w: matrix output: %w", errUtils.ErrFailedToMarshalPayload, err)
	}

	if outputFile != "" {
		return writeToFile(matrixJSON, len(entries), outputFile)
	}

	// Write to stdout.
	if err := data.Writeln(string(matrixJSON)); err != nil {
		return fmt.Errorf("%w: matrix output to stdout: %w", errUtils.ErrWriteOutput, err)
	}
	return nil
}

// writeToFile writes matrix output to a file in key=value format for $GITHUB_OUTPUT.
func writeToFile(matrixJSON []byte, count int, outputFile string) error {
	defer perf.Track(nil, "matrix.writeToFile")()

	f, err := os.OpenFile(outputFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, defaultFilePermissions)
	if err != nil {
		return fmt.Errorf("%w: output file %s: %w", errUtils.ErrOpenFile, outputFile, err)
	}
	defer f.Close()

	// Write matrix=<json> format.
	if _, err := fmt.Fprintf(f, "matrix=%s\n", string(matrixJSON)); err != nil {
		return fmt.Errorf("%w: output file %s: %w", errUtils.ErrWriteFile, outputFile, err)
	}
	// Also write count for convenience (generic name since this is shared by both
	// describe affected and list instances).
	if _, err := fmt.Fprintf(f, "count=%d\n", count); err != nil {
		return fmt.Errorf("%w: count to output file %s: %w", errUtils.ErrWriteFile, outputFile, err)
	}
	log.Debug("Wrote matrix output to file", "file", outputFile, "count", count)
	return nil
}
