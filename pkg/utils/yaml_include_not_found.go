package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// includeNotFoundError explains where a missing local include file was looked for. It mirrors
// findLocalFile, whose path kinds do not overlap: `./` and `../` paths are relative to the
// including file only, other relative paths are relative to base_path and then the working
// directory, and absolute paths are used as-is. The args parameter is the tag's full argument
// text, so a suggested replacement keeps its YQ query and `| eval` option.
func includeNotFoundError(tag, args, includeFile, manifestFile string, atmosConfig *schema.AtmosConfiguration) error {
	basePath := atmosConfig.BasePathAbsolute
	if basePath == "" {
		basePath = atmosConfig.BasePath
	}

	switch {
	case filepath.IsAbs(includeFile):
		return fmt.Errorf("%w: could not find local file '%s'", ErrIncludeYamlFunctionInvalidFile, includeFile)

	case isManifestRelativePath(includeFile):
		resolved := ResolveRelativePath(includeFile, manifestFile)
		builder := errUtils.Build(fmt.Errorf(
			"%w: could not find local file '%s' at '%s' (paths starting with './' or '../' are resolved relative to the including file '%s' only)",
			ErrIncludeYamlFunctionInvalidFile, includeFile, resolved, manifestFile,
		)).WithHint("Paths starting with `./` or `../` are relative to the file that contains the `" + tag +
			"`. Paths without that prefix are relative to `base_path`.")
		if suggestion := baseRelativeIncludeSuggestion(includeFile, basePath); suggestion != "" {
			builder = builder.WithHintf("`%s` exists under `base_path`. Did you mean `%s %s`?",
				suggestion, tag, replaceIncludePath(args, includeFile, suggestion))
		}
		return builder.Err()

	default:
		workingDir, err := os.Getwd()
		if err != nil {
			workingDir = "."
		}
		return errUtils.Build(fmt.Errorf(
			"%w: could not find local file '%s' (tried relative to base path '%s' and the working directory '%s')",
			ErrIncludeYamlFunctionInvalidFile, includeFile, basePath, workingDir,
		)).WithHint("Paths without a `./` or `../` prefix are relative to `base_path`. Start the path with `./` or `../` " +
			"to resolve it relative to the file that contains the `" + tag + "`.").Err()
	}
}

// replaceIncludePath swaps the path at the start of an include's argument text for another path,
// keeping its quoting and everything after it (the YQ query and `| eval`) as written. When the
// path cannot be located as written, only the new path is returned.
func replaceIncludePath(args, includeFile, path string) string {
	trimmed := strings.TrimSpace(args)
	for _, quote := range []string{"", `"`, "'"} {
		written := quote + includeFile + quote
		if strings.HasPrefix(trimmed, written) {
			return quote + path + quote + trimmed[len(written):]
		}
	}
	return path
}

// baseRelativeIncludeSuggestion returns the base_path-relative form of a `./` include path when a
// file exists there, so the error can suggest dropping the prefix. Paths that leave the directory
// they start in (`../`) have no base_path-relative equivalent and get no suggestion.
func baseRelativeIncludeSuggestion(includeFile, basePath string) string {
	if basePath == "" {
		return ""
	}
	cleaned := filepath.Clean(filepath.FromSlash(includeFile))
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return ""
	}
	info, err := os.Stat(filepath.Join(basePath, cleaned))
	if err != nil || info.IsDir() {
		return ""
	}
	return filepath.ToSlash(cleaned)
}
