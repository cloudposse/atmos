package yaml

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mikefarah/yq/v4/pkg/yqlib"

	"github.com/cloudposse/atmos/pkg/filesystem"
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

// errNotFoundFmt is the format string for reporting a not-found path or
// expression alongside its sentinel error.
const errNotFoundFmt = "%w: %s"

// defaultFileMode is the permission used for newly written files when the
// destination does not already exist.
const defaultFileMode os.FileMode = 0o644

// editPreferences are the yqlib YAML preferences used for all edit operations.
// They favor faithful round-tripping: the document's own indent width, no
// colors, document separators preserved, and scalars unwrapped on read.
func editPreferences(indent int) yqlib.YamlPreferences {
	return yqlib.YamlPreferences{
		Indent:                      indent,
		ColorsEnabled:               false,
		LeadingContentPreProcessing: true,
		PrintDocSeparators:          true,
		UnwrapScalar:                true,
		EvaluateTogether:            false,
	}
}

// mergeKeyTagRe matches the explicit `!!merge` tag the yqlib encoder places
// before merge keys when re-encoding a document. The tag is redundant — a
// plain `<<:` key already resolves as a merge in YAML 1.1 — and emitting it
// would churn every merge key in anchor-heavy documents on each edit, so it
// is stripped from encoder output.
var mergeKeyTagRe = regexp.MustCompile(`(?m)^(\s*(?:- )?)!!merge <<:`)

// evaluate runs a yq expression against raw YAML bytes and returns the full
// resulting document, preserving comments, anchors, and formatting as much as
// the underlying yqlib encoder allows. This is the single choke point through
// which every editing operation passes. Multi-document streams are rejected:
// yq would apply the expression to every document in the stream.
func evaluate(content []byte, expr string) (string, error) {
	return evaluateWithOptions(content, expr, defaultEditOptions(content))
}

func evaluateWithOptions(content []byte, expr string, opts editOptions) (string, error) {
	if err := ensureSingleDocument(content); err != nil {
		return "", err
	}

	// Silence yq's internal diagnostics for the duration of the evaluation.
	yqlib.GetLogger().SetLevel(yqEditSilentLevel)

	pref := editPreferences(opts.indent)
	encoder := yqlib.NewYamlEncoder(pref)
	decoder := yqlib.NewYamlDecoder(pref)

	result, err := yqlib.NewStringEvaluator().Evaluate(expr, string(content), encoder, decoder)
	if err != nil {
		return "", fmt.Errorf("%w: %q: %w", ErrInvalidYAMLExpression, expr, err)
	}
	return mergeKeyTagRe.ReplaceAllString(result, "${1}<<:"), nil
}

// Eval evaluates an arbitrary yq expression against raw YAML bytes and returns
// the modified document. It is the escape hatch for power users who want full
// yq syntax; the strict anchor guard still applies so a raw expression cannot
// silently rewrite shared anchors.
func Eval(content []byte, expr string) ([]byte, error) {
	defer perf.Track(nil, "yaml.Eval")()

	return evalWithOptions(content, expr, defaultEditOptions(editBase(content)))
}

func evalWithOptions(content []byte, expr string, opts editOptions) ([]byte, error) {
	base := editBase(content)
	result, err := evaluateWithOptions(base, expr, opts)
	if err != nil {
		return nil, err
	}
	out := []byte(result)
	if err := guardAnchors(base, out); err != nil {
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
	// An empty pre-trimmed result means yq produced no output at all (no match).
	// A legitimate empty scalar prints as a blank line ("\n"), which trims to ""
	// but must still be returned as a valid value.
	if result == "" {
		return "", fmt.Errorf(errNotFoundFmt, ErrYAMLPathNotFound, expr)
	}
	trimmed := strings.TrimRight(result, "\n")
	if trimmed == "null" && !resultIsStringScalar(content, expr) {
		return "", fmt.Errorf(errNotFoundFmt, ErrYAMLPathNotFound, expr)
	}
	return trimmed, nil
}

// resultIsStringScalar reports whether expr resolves to a string scalar. It
// disambiguates the unwrapped output "null", which yq prints both for a true
// YAML null (missing key or explicit null) and for the literal string "null".
func resultIsStringScalar(content []byte, expr string) bool {
	tag, err := evaluate(content, "("+expr+") | tag")
	if err != nil {
		return false
	}
	return strings.TrimRight(tag, "\n") == "!!str"
}

// EvalFile evaluates a yq expression against a file and writes the result back
// atomically, applying the strict anchor guard.
func EvalFile(filePath, expr string) error {
	defer perf.Track(nil, "yaml.EvalFile")()

	return mutateFile(filePath, fileMutationPreserve, func(content []byte, opts editOptions) ([]byte, error) {
		return evalWithOptions(content, expr, opts)
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
	if isEmptyDocument(content) {
		return "", fmt.Errorf(errNotFoundFmt, ErrYAMLPathNotFound, path)
	}

	result, err := evaluate(content, yqPath)
	if err != nil {
		return "", err
	}
	trimmed := strings.TrimRight(result, "\n")

	// yq emits "null" both for a missing key and for an explicit null value.
	// For addressing purposes the editor treats both as "not present"; callers
	// needing to write a key regardless can use Set directly. The literal
	// string "null" prints identically, so it is disambiguated by tag.
	if trimmed == "null" && !resultIsStringScalar(content, yqPath) {
		return "", fmt.Errorf(errNotFoundFmt, ErrYAMLPathNotFound, path)
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

	return setRawWithOptions(content, path, rhs, defaultEditOptions(editBase(content)))
}

func setRawWithOptions(content []byte, path, rhs string, opts editOptions) ([]byte, error) {
	yqPath, err := DotPathToYqPath(path)
	if err != nil {
		return nil, err
	}

	// Seed an empty document with null so the assignment has a document to
	// create keys in; yq evaluates expressions zero times on empty input,
	// which would make Set on a new/empty file a silent no-op.
	base := editBase(content)
	expr := fmt.Sprintf("%s = %s", yqPath, rhs)
	result, err := evaluateWithOptions(base, expr, opts)
	if err != nil {
		return nil, fmt.Errorf(errWrapFmt, ErrYAMLUpdateFailed, err)
	}

	out := []byte(result)
	if err := guardAnchors(base, out); err != nil {
		return nil, err
	}
	return out, nil
}

// Delete removes the value at a dot-notation (or raw yq) path via yq's `del`.
// Deleting from an empty document is a no-op, matching yq's del semantics for
// a missing path.
func Delete(content []byte, path string) ([]byte, error) {
	defer perf.Track(nil, "yaml.Delete")()

	return deleteWithOptions(content, path, defaultEditOptions(content))
}

func deleteWithOptions(content []byte, path string, opts editOptions) ([]byte, error) {
	yqPath, err := DotPathToYqPath(path)
	if err != nil {
		return nil, err
	}
	if isEmptyDocument(content) {
		return content, nil
	}

	expr := fmt.Sprintf("del(%s)", yqPath)
	result, err := evaluateWithOptions(content, expr, opts)
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

	return formatWithOptions(content, defaultEditOptions(content))
}

func formatWithOptions(content []byte, opts editOptions) ([]byte, error) {
	// Nothing to normalize in an empty document; formatting must not
	// materialize a "null" scalar into a previously empty file.
	if isEmptyDocument(content) {
		return content, nil
	}

	result, err := evaluateWithOptions(content, ".", opts)
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

	return mutateFile(filePath, fileMutationPreserve, func(content []byte, opts editOptions) ([]byte, error) {
		return setRawWithOptions(content, path, encodeStringValue(value), opts)
	})
}

// SetFileRaw sets a raw (typed) value at path in a YAML file.
func SetFileRaw(filePath, path, rhs string) error {
	defer perf.Track(nil, "yaml.SetFileRaw")()

	return mutateFile(filePath, fileMutationPreserve, func(content []byte, opts editOptions) ([]byte, error) {
		return setRawWithOptions(content, path, rhs, opts)
	})
}

// DeleteFile removes the value at path in a YAML file.
func DeleteFile(filePath, path string) error {
	defer perf.Track(nil, "yaml.DeleteFile")()

	return mutateFile(filePath, fileMutationPreserve, func(content []byte, opts editOptions) ([]byte, error) {
		return deleteWithOptions(content, path, opts)
	})
}

// FormatFile normalizes a YAML file's formatting in place.
func FormatFile(filePath string) error {
	defer perf.Track(nil, "yaml.FormatFile")()

	return mutateFile(filePath, fileMutationFormat, formatWithOptions)
}

// mutateFile reads a file, applies fn, and writes the result back atomically
// (temp file + rename) preserving the original file mode. Symlinks are
// resolved first so editing a symlinked config rewrites the target file
// instead of replacing the link with a regular file. Regular edits preserve
// detected file style, while format operations normalize to EditorConfig when
// it declares a style.
func mutateFile(filePath string, mode fileMutationMode, fn func([]byte, editOptions) ([]byte, error)) error {
	if resolved, err := filepath.EvalSymlinks(filePath); err == nil {
		filePath = resolved
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf(errWrapFmt, ErrReadFile, err)
	}

	style := resolveEditorConfigStyle(filePath)
	out, err := fn(content, editOptionsForFile(content, style, mode))
	if err != nil {
		return err
	}

	return atomicWrite(filePath, applyFileStyle(content, out, style, mode))
}

// atomicWrite writes data to filePath via the shared cross-platform atomic
// writer (temp file + rename on Unix, ReplaceFile semantics on Windows so an
// existing file can be replaced), preserving the destination's existing
// permissions when it already exists.
func atomicWrite(filePath string, data []byte) error {
	mode := defaultFileMode
	if info, statErr := os.Stat(filePath); statErr == nil {
		mode = info.Mode().Perm()
	}

	if err := filesystem.NewOSFileSystem().WriteFileAtomic(filePath, data, mode); err != nil {
		return fmt.Errorf(errWrapFmt, ErrWriteFile, err)
	}
	return nil
}
