package yaml

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/mikefarah/yq/v4/pkg/yqlib"

	"github.com/cloudposse/atmos/pkg/perf"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// yqEditSilentLevel sits above any real slog level so yq's internal logger
// rejects every message during editing. Mirrors pkg/utils.yqSilentLevel; the
// editor keeps its own copy so it never depends on whatever level the query
// path last set process-wide.
const yqEditSilentLevel = slog.Level(1000)

// errWrapFmt is the format string for wrapping a sentinel error with an
// underlying error.
const errWrapFmt = "%w: %w"

// defaultFileMode is the permission used for newly written files when the
// destination does not already exist.
const defaultFileMode os.FileMode = 0o644

// editPreferences are the yqlib YAML preferences used for all edit operations.
// They favor faithful round-tripping: 2-space indent, no colors, document
// separators preserved, and scalars unwrapped on read.
func editPreferences() yqlib.YamlPreferences {
	return yqlib.YamlPreferences{
		Indent:                      DefaultIndent,
		ColorsEnabled:               false,
		LeadingContentPreProcessing: true,
		PrintDocSeparators:          true,
		UnwrapScalar:                true,
		EvaluateTogether:            false,
	}
}

// evaluate runs a yq expression against raw YAML bytes and returns the full
// resulting document, preserving comments, anchors, and formatting as much as
// the underlying yqlib encoder allows. This is the single choke point through
// which every editing operation passes.
func evaluate(content []byte, expr string) (string, error) {
	// Silence yq's internal diagnostics for the duration of the evaluation.
	yqlib.GetLogger().SetLevel(yqEditSilentLevel)

	pref := editPreferences()
	encoder := yqlib.NewYamlEncoder(pref)
	decoder := yqlib.NewYamlDecoder(pref)

	result, err := yqlib.NewStringEvaluator().Evaluate(expr, string(content), encoder, decoder)
	if err != nil {
		return "", fmt.Errorf("%w: %q: %w", ErrInvalidYAMLExpression, expr, err)
	}
	return result, nil
}

// Eval evaluates an arbitrary yq expression against raw YAML bytes and returns
// the modified document. It is the escape hatch for power users who want full
// yq syntax; the strict anchor guard still applies so a raw expression cannot
// silently rewrite shared anchors.
func Eval(content []byte, expr string) ([]byte, error) {
	defer perf.Track(nil, "yaml.Eval")()

	result, err := evaluate(content, expr)
	if err != nil {
		return nil, err
	}
	out := []byte(result)
	if err := guardAnchors(content, out); err != nil {
		return nil, err
	}
	return out, nil
}

// Query evaluates a read-only yq expression (e.g. a select() filter) against
// raw YAML bytes and returns the trimmed result. Unlike Get, it does not treat
// the input as a dot-path, so callers can use full yq syntax. A "null" result
// is reported as ErrYAMLPathNotFound.
func Query(content []byte, expr string) (string, error) {
	defer perf.Track(nil, "yaml.Query")()

	result, err := evaluate(content, expr)
	if err != nil {
		return "", err
	}
	trimmed := strings.TrimRight(result, "\n")
	if trimmed == "null" || trimmed == "" {
		return "", fmt.Errorf("%w: %s", ErrYAMLPathNotFound, expr)
	}
	return trimmed, nil
}

// EvalFile evaluates a yq expression against a file and writes the result back
// atomically, applying the strict anchor guard.
func EvalFile(filePath, expr string) error {
	defer perf.Track(nil, "yaml.EvalFile")()

	return mutateFile(filePath, func(content []byte) ([]byte, error) {
		return Eval(content, expr)
	})
}

// QueryFile evaluates a read-only yq expression against a file.
func QueryFile(filePath, expr string) (string, error) {
	defer perf.Track(nil, "yaml.QueryFile")()

	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf(errWrapFmt, ErrReadFile, err)
	}
	return Query(content, expr)
}

// Get reads the value at a dot-notation (or raw yq) path and returns it as a
// trimmed string. Missing or null paths return ErrYAMLPathNotFound.
func Get(content []byte, path string) (string, error) {
	defer perf.Track(nil, "yaml.Get")()

	yqPath, err := DotPathToYqPath(path)
	if err != nil {
		return "", err
	}

	result, err := evaluate(content, yqPath)
	if err != nil {
		return "", err
	}
	trimmed := strings.TrimRight(result, "\n")

	// yq emits "null" both for a missing key and for an explicit null value.
	// For addressing purposes the editor treats both as "not present"; callers
	// needing to write a key regardless can use Set directly.
	if trimmed == "null" {
		return "", fmt.Errorf("%w: %s", ErrYAMLPathNotFound, path)
	}
	return trimmed, nil
}

// GetTyped reads the value at a path and decodes it into T.
func GetTyped[T any](content []byte, path string) (T, error) {
	defer perf.Track(nil, "yaml.GetTyped")()

	var zero T
	raw, err := Get(content, path)
	if err != nil {
		return zero, err
	}
	decoded, err := u.UnmarshalYAML[T](raw)
	if err != nil {
		return zero, fmt.Errorf(errWrapFmt, ErrParseYAML, err)
	}
	return decoded, nil
}

// Set assigns a string value at a dot-notation (or raw yq) path and returns the
// modified document, preserving comments/anchors/formatting. The value is
// written as a YAML string scalar. Use SetRaw for non-string (typed) values.
func Set(content []byte, path, value string) ([]byte, error) {
	defer perf.Track(nil, "yaml.Set")()

	return SetRaw(content, path, encodeStringValue(value))
}

// SetRaw assigns a raw yq right-hand-side expression at a path. The rhs is
// inserted verbatim, so callers may pass typed literals (`true`, `42`,
// `[1,2,3]`) or yq expressions. Comments/anchors/formatting are preserved and
// the strict anchor guard applies.
func SetRaw(content []byte, path, rhs string) ([]byte, error) {
	defer perf.Track(nil, "yaml.SetRaw")()

	yqPath, err := DotPathToYqPath(path)
	if err != nil {
		return nil, err
	}

	expr := fmt.Sprintf("%s = %s", yqPath, rhs)
	result, err := evaluate(content, expr)
	if err != nil {
		return nil, fmt.Errorf(errWrapFmt, ErrYAMLUpdateFailed, err)
	}

	out := []byte(result)
	if err := guardAnchors(content, out); err != nil {
		return nil, err
	}
	return out, nil
}

// Delete removes the value at a dot-notation (or raw yq) path via yq's `del`.
func Delete(content []byte, path string) ([]byte, error) {
	defer perf.Track(nil, "yaml.Delete")()

	yqPath, err := DotPathToYqPath(path)
	if err != nil {
		return nil, err
	}

	expr := fmt.Sprintf("del(%s)", yqPath)
	result, err := evaluate(content, expr)
	if err != nil {
		return nil, fmt.Errorf(errWrapFmt, ErrYAMLUpdateFailed, err)
	}

	out := []byte(result)
	if err := guardAnchors(content, out); err != nil {
		return nil, err
	}
	return out, nil
}

// Format normalizes YAML formatting (indentation, spacing) by round-tripping
// the document through the editor's encoder with an identity expression, while
// preserving comments and anchors. This powers a `format`/`fmt` capability.
func Format(content []byte) ([]byte, error) {
	defer perf.Track(nil, "yaml.Format")()

	result, err := evaluate(content, ".")
	if err != nil {
		return nil, err
	}
	out := []byte(result)
	if err := guardAnchors(content, out); err != nil {
		return nil, err
	}
	return out, nil
}

// --- File wrappers -----------------------------------------------------------

// GetFile reads a YAML file and returns the value at path.
func GetFile(filePath, path string) (string, error) {
	defer perf.Track(nil, "yaml.GetFile")()

	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf(errWrapFmt, ErrReadFile, err)
	}
	return Get(content, path)
}

// SetFile sets a string value at path in a YAML file, writing the result back
// atomically while preserving the original file mode.
func SetFile(filePath, path, value string) error {
	defer perf.Track(nil, "yaml.SetFile")()

	return mutateFile(filePath, func(content []byte) ([]byte, error) {
		return Set(content, path, value)
	})
}

// SetFileRaw sets a raw (typed) value at path in a YAML file.
func SetFileRaw(filePath, path, rhs string) error {
	defer perf.Track(nil, "yaml.SetFileRaw")()

	return mutateFile(filePath, func(content []byte) ([]byte, error) {
		return SetRaw(content, path, rhs)
	})
}

// DeleteFile removes the value at path in a YAML file.
func DeleteFile(filePath, path string) error {
	defer perf.Track(nil, "yaml.DeleteFile")()

	return mutateFile(filePath, func(content []byte) ([]byte, error) {
		return Delete(content, path)
	})
}

// FormatFile normalizes a YAML file's formatting in place.
func FormatFile(filePath string) error {
	defer perf.Track(nil, "yaml.FormatFile")()

	return mutateFile(filePath, Format)
}

// mutateFile reads a file, applies fn, and writes the result back atomically
// (temp file + rename) preserving the original file mode.
func mutateFile(filePath string, fn func([]byte) ([]byte, error)) error {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf(errWrapFmt, ErrReadFile, err)
	}

	out, err := fn(content)
	if err != nil {
		return err
	}

	return atomicWrite(filePath, out)
}

// atomicWrite writes data to filePath via a temp file in the same directory
// followed by a rename, preserving the destination's existing permissions when
// it already exists.
func atomicWrite(filePath string, data []byte) error {
	mode := defaultFileMode
	if info, statErr := os.Stat(filePath); statErr == nil {
		mode = info.Mode().Perm()
	}

	dir := filepath.Dir(filePath)
	tmp, err := os.CreateTemp(dir, ".atmos-yaml-*.tmp")
	if err != nil {
		return fmt.Errorf(errWrapFmt, ErrWriteFile, err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // No-op if the rename below succeeds.

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf(errWrapFmt, ErrWriteFile, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf(errWrapFmt, ErrWriteFile, err)
	}
	//nolint:gosec // filePath is the user-specified file being edited; writing to it is the intended behavior.
	if err := os.Chmod(tmpName, mode); err != nil {
		return fmt.Errorf(errWrapFmt, ErrWriteFile, err)
	}
	//nolint:gosec // filePath is the user-specified file being edited; the atomic rename targets it intentionally.
	if err := os.Rename(tmpName, filePath); err != nil {
		return fmt.Errorf(errWrapFmt, ErrWriteFile, err)
	}
	return nil
}
