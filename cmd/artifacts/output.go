package artifacts

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	sigsyaml "sigs.k8s.io/yaml"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/invoke"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/terminal"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

// result is the structured result of one operation. The type is shared with the artifact step and
// hooks (see pkg/artifact/invoke), so every entry point reports identical field names.
type result = invoke.Result

// joinValues renders a format list for flag help and errors.
func joinValues(values []string) string {
	return strings.Join(values, ", ")
}

// validateFormat rejects a format a command does not support before any work is done.
func validateFormat(value string, valid []string) error {
	if slices.Contains(valid, value) {
		return nil
	}
	return errUtils.Build(fmt.Errorf("%w: unsupported format %q; supported formats: %s", errUtils.ErrInvalidFlag, value, joinValues(valid))).
		WithHintf("Use `--format=%s`.", valid[0]).
		WithExitCode(2).
		Err()
}

// isStructured reports whether the format is machine-readable.
func isStructured(format string) bool {
	return format == formatJSON || format == formatYAML
}

// writeStructured writes a value as JSON or YAML to the data channel (stdout). YAML is derived from
// the JSON encoding, so both share the same field names and number formatting.
func writeStructured(format string, value any) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encode %s output: %w", format, err)
	}
	if format == formatYAML {
		converted, err := sigsyaml.JSONToYAML(raw)
		if err != nil {
			return fmt.Errorf("encode yaml output: %w", err)
		}
		return data.Write(string(converted))
	}
	return data.Writeln(string(raw))
}

// keyValue is one line of the text rendering.
type keyValue struct {
	key   string
	value string
}

// writeText renders terminal details as a table and keeps redirected text in key: value format.
func writeText(pairs []keyValue) error {
	if terminal.New().IsTTY(terminal.Stdout) {
		rows := make([][]string, 0, len(pairs))
		for _, pair := range pairs {
			if pair.value != "" {
				rows = append(rows, []string{detailLabel(pair.key), pair.value})
			}
		}
		if len(rows) == 0 {
			return nil
		}
		return data.Writeln(theme.CreateMinimalTable([]string{"Property", "Value"}, rows))
	}

	width := 0
	for _, pair := range pairs {
		if pair.value != "" && len(pair.key) > width {
			width = len(pair.key)
		}
	}
	var builder strings.Builder
	for _, pair := range pairs {
		if pair.value == "" {
			continue
		}
		fmt.Fprintf(&builder, "%-*s %s\n", width+1, pair.key+":", pair.value)
	}
	return data.Write(builder.String())
}

// detailLabel converts internal field names into labels for the terminal table.
func detailLabel(key string) string {
	switch key {
	case invoke.OutputRef:
		return "Reference"
	case invoke.OutputSize:
		return "Size (bytes)"
	case "readme":
		return "README"
	case "changelog":
		return "CHANGELOG"
	default:
		label := strings.ReplaceAll(key, "_", " ")
		return strings.ToUpper(label[:1]) + label[1:]
	}
}

// operationPairs are the named outputs shared by every operation, in a stable order, followed by
// the lock scope. Human-readable locations inside the working directory use relative paths.
func operationPairs(op *artifact.OperationResult) []keyValue {
	named := invoke.OutputPairs(op)
	pairs := make([]keyValue, 0, len(named)+1)
	for _, pair := range named {
		if pair.Name == invoke.OutputLocation {
			pair.Value = displayLocation(pair.Value)
		}
		pairs = append(pairs, keyValue{pair.Name, pair.Value})
	}
	return append(pairs, keyValue{"lock_scope", string(op.LockScope)})
}

func displayLocation(location string) string {
	if !filepath.IsAbs(location) {
		return location
	}
	cwd, err := os.Getwd()
	if err != nil {
		return location
	}
	rel, err := filepath.Rel(cwd, location)
	if err != nil || !filepath.IsLocal(rel) {
		return location
	}
	return rel
}

// reportDiagnostics sends the non-fatal messages of an operation to the UI channel (stderr).
func reportDiagnostics(diagnostics []artifact.Diagnostic) {
	for index := range diagnostics {
		diagnostic := diagnostics[index]
		switch diagnostic.Severity {
		case artifact.SeverityWarning:
			ui.Warning(diagnostic.Message)
		case artifact.SeverityInfo:
			ui.Info(diagnostic.Message)
		case artifact.SeverityError:
			// The error itself is returned and rendered once by the error path.
		}
	}
}

// emit renders a result. Structured formats always render, including on failure; the text format
// renders only a successful result, because a failure is reported by the returned error.
func emit(format string, res *result, failed bool, text func(*result) error) error {
	if isStructured(format) {
		return writeStructured(format, res)
	}
	if failed {
		return nil
	}
	reportDiagnostics(res.Diagnostics)
	return text(res)
}

// textOperation is the default rendering of a plain operation result.
func textOperation(res *result) error {
	return writeText(operationPairs(&res.OperationResult))
}
