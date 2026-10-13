package imports

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// templateMarker is the Go template action opener used to detect templated import paths.
const templateMarker = "{{"

// globChars are the characters that turn an import path into a glob pattern.
const globChars = "*?[{"

// IsTemplatedPath reports whether an import path contains a Go template expression.
// Templated import paths can only be resolved at stack-processing time (they depend on
// the `settings`/`vars`/`env` sections of earlier imports), so static resolvers skip them.
func IsTemplatedPath(importPath string) bool {
	defer perf.Track(nil, "imports.IsTemplatedPath")()

	return strings.Contains(importPath, templateMarker)
}

// IsTemplateFile reports whether a resolved import file is a Go template file
// (`.yaml.tmpl` or `.yml.tmpl`) that must be rendered before it can be parsed as YAML.
func IsTemplateFile(filePath string) bool {
	defer perf.Track(nil, "imports.IsTemplateFile")()

	return u.IsTemplateFile(filePath)
}

// ResolveLocal resolves a single local stack import to the files on disk that the
// stack processor would load for it. It has no side effects, never reads file
// contents, and never renders templates.
//
// Semantics mirror the runtime stack processor (internal/exec ProcessImportSection
// and processYAMLConfigFileWithContextInternal):
//
//   - `./` and `../` import paths are relative to the importing file's directory;
//     every other path is relative to stacksBasePath.
//   - An import with no extension tries `.yaml`, `.yml`, `.yaml.tmpl`, `.yml.tmpl`
//     in that order and falls back to `.yaml`.
//   - An import ending in `.yaml` or `.yml` is upgraded to the `.tmpl` sibling when
//     that file exists.
//   - Glob patterns (including doublestar `**`) are expanded.
//
// Remote imports (see IsRemote) and templated paths (see IsTemplatedPath) are not
// handled here; callers must filter them out first.
//
// It returns errUtils.ErrStackImportNotFound (wrapped) when nothing matches, and
// errUtils.ErrInvalidImport (wrapped) when the path is empty or a non-relative path
// is given without a stacksBasePath.
func ResolveLocal(importPath, importingFile, stacksBasePath string) ([]string, error) {
	defer perf.Track(nil, "imports.ResolveLocal")()

	if importPath == "" {
		return nil, fmt.Errorf("%w: empty import path", errUtils.ErrInvalidImport)
	}

	baseDir, err := localImportBaseDir(importPath, importingFile, stacksBasePath)
	if err != nil {
		return nil, err
	}

	rel := filepath.FromSlash(importPath)
	pattern := filepath.Join(baseDir, withImportExtension(baseDir, rel))

	matches, err := expandImportPattern(pattern)
	if err != nil {
		return nil, fmt.Errorf("%w: import '%s' in file '%s': %w", errUtils.ErrStackImportNotFound, importPath, importingFile, err)
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("%w: import '%s' in file '%s'", errUtils.ErrStackImportNotFound, importPath, importingFile)
	}
	return matches, nil
}

// localImportBaseDir returns the directory a local import path is relative to.
func localImportBaseDir(importPath, importingFile, stacksBasePath string) (string, error) {
	if strings.HasPrefix(importPath, "./") || strings.HasPrefix(importPath, "../") ||
		importPath == "." || importPath == ".." {
		return filepath.Dir(importingFile), nil
	}
	if stacksBasePath == "" {
		return "", fmt.Errorf("%w: import '%s' requires the stacks base path", errUtils.ErrInvalidImport, importPath)
	}
	return stacksBasePath, nil
}

// withImportExtension applies the runtime's extension resolution to a relative import path.
func withImportExtension(baseDir, rel string) string {
	switch filepath.Ext(rel) {
	case "":
		for _, extension := range []string{
			u.YamlFileExtension,
			u.YmlFileExtension,
			u.YamlTemplateExtension,
			u.YmlTemplateExtension,
		} {
			if _, err := os.Stat(filepath.Join(baseDir, rel+extension)); err == nil {
				return rel + extension
			}
		}
		// Default to .yaml when no file is found.
		return rel + u.DefaultStackConfigFileExtension
	case u.YamlFileExtension, u.YmlFileExtension:
		// Prefer the template version of the file when it exists.
		if _, err := os.Stat(filepath.Join(baseDir, rel+u.TemplateExtension)); err == nil {
			return rel + u.TemplateExtension
		}
	}
	return rel
}

// expandImportPattern expands a (possibly glob) pattern into existing paths.
// Unlike utils.GetGlobMatches it does not memoize results, so callers always see current disk state.
func expandImportPattern(pattern string) ([]string, error) {
	if !strings.ContainsAny(pattern, globChars) {
		if _, err := os.Stat(pattern); err == nil {
			return []string{pattern}, nil
		}
		return nil, nil
	}

	slashPattern := filepath.ToSlash(pattern)
	base, cleanPattern := doublestar.SplitPattern(slashPattern)
	matches, err := doublestar.Glob(os.DirFS(base), cleanPattern)
	if err != nil {
		return nil, err
	}

	result := make([]string, 0, len(matches))
	for _, match := range matches {
		result = append(result, filepath.Join(filepath.FromSlash(base), match))
	}
	return result, nil
}
