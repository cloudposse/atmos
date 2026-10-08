package initcmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"al.essio.dev/pkg/shellescape"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	gen "github.com/cloudposse/atmos/pkg/generator"
	"github.com/cloudposse/atmos/pkg/generator/directory"
	"github.com/cloudposse/atmos/pkg/generator/source"
	"github.com/cloudposse/atmos/pkg/generator/templates"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/vendor"
)

type preparedInitSource struct {
	directory *source.Directory
	name      string
	copy      bool
	cleanup   func()
}

// explicitScaffoldFlags catches explicitly supplied scaffold-only settings,
// including values equal to their defaults and environment overrides.
func explicitScaffoldFlags(cmd *cobra.Command, v *viper.Viper) []string {
	var result []string
	for _, name := range []string{"update", "set", "base-ref", "update-strategy", "merge-driver", "merge-strategy", "max-changes", "recreate-deleted", "skip-hooks"} {
		_, envSet := os.LookupEnv("ATMOS_INIT_" + strings.ToUpper(strings.ReplaceAll(name, "-", "_")))
		if cmd.Flags().Changed(name) || envSet || v.InConfig(name) {
			result = append(result, "--"+name)
		}
	}
	return result
}

func normalizeInitArgument(opts *initOptions, configs map[string]templates.Configuration) error {
	if opts.templateName == "" {
		return nil
	}
	if _, exists := configs[opts.templateName]; exists {
		return nil
	}
	// Bare existing directories are accepted after named templates, which keep
	// precedence. Use ./ to disambiguate a local directory with a template name.
	if !strings.HasPrefix(opts.templateName, "examples/") && !source.IsTemplateSource(opts.templateName) {
		if info, err := os.Stat(opts.templateName); err == nil && info.IsDir() {
			opts.templateName = resolveTargetDir(opts.templateName)
		}
	}
	if err := expandInitRepository(opts); err != nil {
		return err
	}
	normalized, err := source.NormalizeInitSource(opts.templateName, opts.ref)
	if err != nil {
		return err
	}
	opts.templateName = normalized.Source
	opts.copy = opts.copy || normalized.Copy
	return nil
}

func prepareInitSource(opts *initOptions, selected *templates.Configuration, configs map[string]templates.Configuration) (*preparedInitSource, error) {
	prepared := &preparedInitSource{cleanup: func() {}}
	_, catalog := configs[opts.templateName]
	if !opts.copy && (opts.templateName == "" || catalog) {
		cleanup, err := source.Hydrate(selected, opts.sourceOverride)
		prepared.cleanup = cleanup
		return prepared, err
	}
	if len(selected.Files) != 0 || selected.Source == "" {
		return nil, copyOptionError("--copy requires a source directory; embedded templates cannot be copied")
	}
	normalized, err := source.NormalizeInitSource(selected.Source, opts.ref)
	if err != nil {
		return nil, err
	}
	return prepareInitDirectory(opts, selected, normalized)
}

func prepareInitDirectory(opts *initOptions, selected *templates.Configuration, normalized source.InitSource) (*preparedInitSource, error) {
	atmosConfig, err := initSourceConfig(normalized.Source)
	if err != nil {
		return nil, err
	}
	dir, cleanup, err := source.FetchDirectory(&atmosConfig, normalized.Name, normalized.Source, source.DefaultFetchTimeout)
	if err != nil {
		cleanup()
		return nil, err
	}
	prepared := &preparedInitSource{directory: dir, cleanup: cleanup, name: normalized.Name}
	_, manifestErr := os.Lstat(filepath.Join(dir.Path, "scaffold.yaml"))
	if manifestErr != nil && !errors.Is(manifestErr, os.ErrNotExist) {
		cleanup()
		return nil, manifestErr
	}
	prepared.copy = opts.copy || errors.Is(manifestErr, os.ErrNotExist)
	if prepared.copy {
		return prepared, nil
	}
	loaded, err := dir.LoadScaffold(selected.Name, normalized.Source)
	if err != nil {
		cleanup()
		return nil, err
	}
	*selected = *loaded
	return prepared, nil
}

func copyOptionError(message string) error {
	return errUtils.Build(errUtils.ErrInvalidFlagValue).WithExplanation(message).WithExitCode(2).Err()
}

func validateCopyOptions(opts *initOptions) error {
	if len(opts.copyUnsupported) > 0 {
		return copyOptionError("Copy initialization does not support " + strings.Join(opts.copyUnsupported, ", "))
	}
	if opts.update || len(opts.templateVars) > 0 || opts.baseRef != "" || opts.mergeStrategy != "" || opts.recreateDeleted {
		return copyOptionError("Copy initialization does not support scaffold answers or merge updates")
	}
	return validateCopyStrategies(opts)
}

func validateCopyStrategies(opts *initOptions) error {
	if opts.updateStrategy != "" && opts.updateStrategy != "tracked" {
		return copyOptionError("Copy initialization does not support --update-strategy")
	}
	if opts.mergeDriver != "" && opts.mergeDriver != "auto" {
		return copyOptionError("Copy initialization does not support --merge-driver")
	}
	return nil
}

func resolveCopyTarget(opts *initOptions, name string) error {
	if opts.targetDir != "" {
		return nil
	}
	if name == "" || name == "." || name == string(filepath.Separator) {
		return copyOptionError("Cannot infer a directory name from this source; supply a target directory")
	}
	opts.targetDir = resolveTargetDir(name)
	return nil
}

func runCopyInit(ctx context.Context, opts *initOptions, prepared *preparedInitSource) error {
	if err := validateCopyOptions(opts); err != nil {
		return err
	}
	if err := resolveCopyTarget(opts, prepared.name); err != nil {
		return err
	}
	if err := directory.Copy(ctx, prepared.directory.Path, opts.targetDir, opts.force); err != nil {
		return err
	}
	if opts.git {
		if _, _, err := gen.InitGitRepository(gen.InitGitOptions{
			TargetPath: opts.targetDir, TemplateName: prepared.name, AllowEmptyCommit: true,
		}); err != nil {
			return err
		}
	}
	return displayCopiedProject(prepared, opts.targetDir)
}

func displayCopiedProject(prepared *preparedInitSource, targetDir string) error {
	ui.Success(fmt.Sprintf("Initialized %s in %s", prepared.name, targetDir))
	readme, err := os.ReadFile(filepath.Join(prepared.directory.Path, "README.md"))
	if err == nil && len(readme) > 0 {
		ui.MarkdownMessagef("%s", string(readme))
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	ui.Writef("\nNext: cd %s\n", shellescape.Quote(targetDir))
	return nil
}

// initSourceConfig avoids configuration loading for explicit local paths.
func initSourceConfig(src string) (schema.AtmosConfiguration, error) {
	if vendor.IsLocalPath(src) || vendor.IsFileURI(src) {
		return schema.AtmosConfiguration{}, nil
	}
	return cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
}
