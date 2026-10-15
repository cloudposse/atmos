package validation

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	ecconfig "github.com/editorconfig-checker/editorconfig-checker/v3/pkg/config"
	ecfiles "github.com/editorconfig-checker/editorconfig-checker/v3/pkg/files"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// EditorConfigDisableOverride pairs a --disable-* flag name with the atmos.yaml source
// field and the editorconfig-checker target field it feeds.
type EditorConfigDisableOverride struct {
	FlagName string
	Source   *bool
	Target   *bool
}

// EditorConfigDisableOverrides lists every disable_* setting of validate.editorconfig together
// with the editorconfig-checker field it controls.
func EditorConfigDisableOverrides(settings *schema.EditorConfig, target *ecconfig.Config) []EditorConfigDisableOverride {
	defer perf.Track(nil, "validation.EditorConfigDisableOverrides")()

	return []EditorConfigDisableOverride{
		{"disable-trim-trailing-whitespace", &settings.DisableTrimTrailingWhitespace, &target.Disable.TrimTrailingWhitespace},
		{"disable-end-of-line", &settings.DisableEndOfLine, &target.Disable.EndOfLine},
		{"disable-insert-final-newline", &settings.DisableInsertFinalNewline, &target.Disable.InsertFinalNewline},
		{"disable-indentation", &settings.DisableIndentation, &target.Disable.Indentation},
		{"disable-indent-size", &settings.DisableIndentSize, &target.Disable.IndentSize},
		{"disable-max-line-length", &settings.DisableMaxLineLength, &target.Disable.MaxLineLength},
	}
}

// ApplyEditorConfigSettings applies the validate.editorconfig ignore_defaults and disable_*
// settings to a checker configuration. A setting only ever turns a behavior on, exactly like
// the standalone command when its flag is not set.
func ApplyEditorConfigSettings(cfg *ecconfig.Config, settings *schema.EditorConfig) {
	defer perf.Track(nil, "validation.ApplyEditorConfigSettings")()

	if settings.IgnoreDefaults {
		cfg.IgnoreDefaults = true
	}
	for _, override := range EditorConfigDisableOverrides(settings, cfg) {
		if *override.Source {
			*override.Target = true
		}
	}
}

// EditorConfigSettingsFromMap decodes the validate.editorconfig section of a raw
// configuration document. A missing or absent section yields zero settings.
func EditorConfigSettingsFromMap(config map[string]any) (schema.EditorConfig, error) {
	defer perf.Track(nil, "validation.EditorConfigSettingsFromMap")()

	var settings schema.EditorConfig
	validate, _ := config["validate"].(map[string]any)
	section, ok := validate["editorconfig"]
	if !ok || section == nil {
		return settings, nil
	}
	encoded, err := json.Marshal(section)
	if err != nil {
		return settings, fmt.Errorf("encode validate.editorconfig: %w", err)
	}
	if err := json.Unmarshal(encoded, &settings); err != nil {
		return settings, fmt.Errorf("decode validate.editorconfig: %w", err)
	}
	return settings, nil
}

// EditorConfigFiles returns the files EditorConfig Checker should validate under root.
//
// Inside a Git work tree the candidates are exactly what Git reports with
// "ls-files --cached --others --exclude-standard", so ignored paths such as vendored components
// and workdirs are never checked. Outside Git the directory is walked. Candidates are then
// filtered by the glob patterns (validate.editorconfig.exclude), by the checker's own exclude
// regular expressions (honoring ignore_defaults), and by allowed content type. Both kinds of
// pattern match root-relative paths, so the result does not depend on the working directory.
//
// Returned paths are root joined with the root-relative path, so a relative root yields relative
// paths and an absolute root yields absolute ones.
func EditorConfigFiles(root string, cfg *ecconfig.Config, globExcludes []string) ([]string, error) {
	defer perf.Track(nil, "validation.EditorConfigFiles")()

	if root == "" {
		root = "."
	}
	candidates, err := editorConfigCandidates(root, cfg)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrEditorConfigGetFiles, err)
	}
	candidates, err = ExcludePaths(candidates, globExcludes)
	if err != nil {
		return nil, err
	}
	excluded, err := editorConfigExcluder(cfg)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrEditorConfigGetFiles, err)
	}
	result := make([]string, 0, len(candidates))
	for _, relative := range candidates {
		if excluded(relative) {
			continue
		}
		path := filepath.Join(root, filepath.FromSlash(relative))
		if editorConfigAllowed(path, cfg) {
			result = append(result, path)
		}
	}
	return result, nil
}

// SelectEditorConfigFiles keeps only the paths that are also in selected, comparing absolute
// cleaned paths. A nil selection keeps everything.
func SelectEditorConfigFiles(paths, selected []string) []string {
	defer perf.Track(nil, "validation.SelectEditorConfigFiles")()

	if selected == nil {
		return paths
	}
	wanted := make(map[string]struct{}, len(selected))
	for _, file := range selected {
		if absolute, err := filepath.Abs(filepath.FromSlash(file)); err == nil {
			wanted[filepath.Clean(absolute)] = struct{}{}
		}
	}
	kept := make([]string, 0, len(paths))
	for _, path := range paths {
		absolute, err := filepath.Abs(path)
		if err != nil {
			continue
		}
		if _, ok := wanted[filepath.Clean(absolute)]; ok {
			kept = append(kept, path)
		}
	}
	return kept
}

// editorConfigExcluder returns a predicate for the checker's exclude regular expression,
// mirroring files.IsExcluded but matching the root-relative path it is given.
func editorConfigExcluder(cfg *ecconfig.Config) (func(string) bool, error) {
	if len(cfg.Exclude) == 0 && cfg.IgnoreDefaults {
		return func(string) bool { return false }, nil
	}
	re, err := cfg.CachedExcludesAsRegexp()
	if err != nil {
		return nil, err
	}
	return re.MatchString, nil
}

// editorConfigAllowed reports whether a candidate is a regular file with a content type the
// checker validates. Files whose type cannot be determined are kept, as the checker does.
func editorConfigAllowed(path string, cfg *ecconfig.Config) bool {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		// A broken symlink or a deleted-but-cached file is not checked.
		return false
	}
	// The checker's content-type sniffing panics when a file cannot be opened, so keep such
	// files and let validation report the read failure.
	file, openErr := os.Open(path)
	if openErr != nil {
		return true
	}
	_ = file.Close()
	contentType, err := ecfiles.GetContentType(path)
	if err != nil {
		return true
	}
	return ecfiles.IsAllowedContentType(contentType, *cfg)
}

// editorConfigCandidates lists root-relative, slash-separated candidate files.
func editorConfigCandidates(root string, cfg *ecconfig.Config) ([]string, error) {
	if listed, ok := gitListedFiles(root); ok {
		return listed, nil
	}
	return walkedFiles(root, cfg)
}

// gitListedFiles asks Git for tracked and untracked-but-not-ignored files under root.
func gitListedFiles(root string) ([]string, bool) {
	command := exec.Command("git", "-C", root, "ls-files", "--cached", "--others", "--exclude-standard", nullSeparatedFlag)
	var stdout bytes.Buffer
	command.Stdout = &stdout
	if err := command.Run(); err != nil {
		return nil, false
	}
	var listed []string
	for _, entry := range strings.Split(stdout.String(), "\x00") {
		if entry != "" {
			listed = append(listed, filepath.ToSlash(entry))
		}
	}
	return listed, true
}

// walkedFiles walks root for regular files, pruning directories the checker would exclude.
func walkedFiles(root string, cfg *ecconfig.Config) ([]string, error) {
	excluded, err := editorConfigExcluder(cfg)
	if err != nil {
		return nil, err
	}
	var listed []string
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		relative = filepath.ToSlash(relative)
		if entry.IsDir() {
			if relative != "." && excluded(relative) {
				return fs.SkipDir
			}
			return nil
		}
		if entry.Type().IsRegular() {
			listed = append(listed, relative)
		}
		return nil
	})
	return listed, err
}
