package ui

import (
	"errors"
	"fmt"
	"os"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/generator/engine"
	tmpl "github.com/cloudposse/atmos/pkg/generator/templates"
	"github.com/cloudposse/atmos/pkg/project/config"
)

// SetUpdateStrategy selects where a 3-way merge's base content comes from
// during --update (engine.UpdateStrategyTracked, the default: the target's
// own git history; engine.UpdateStrategyRendered: a pristine template
// re-render, see SetRenderedBaseSource).
func (ui *InitUI) SetUpdateStrategy(strategy engine.UpdateStrategy) {
	ui.updateStrategy = strategy
}

// SetRenderedBaseSource supplies the pristine "old ref" template
// configuration and its originally-recorded answers that
// engine.UpdateStrategyRendered re-renders as the merge base. Both must come
// from the same generation (see pkg/project/config's project-record
// Spec.Values/Spec.BaseRef) -- rendering the old config's own scaffold.yaml
// against these old values keeps base self-consistent, unlike applying this
// run's new answers to the old schema.
func (ui *InitUI) SetRenderedBaseSource(cfg *tmpl.Configuration, values map[string]interface{}) {
	ui.renderedBaseConfig = cfg
	ui.renderedBaseValues = values
}

// loadOldScaffoldConfig finds and loads oldConfig's own scaffold.yaml, so
// renderPristineBase can render it against the old ref's own schema rather
// than the current run's. The returned []string is every path some
// !include tag in that scaffold.yaml consumed (see config.WithIncludedPaths)
// -- renderPristineBaseFiles excludes those from the render the same way
// executeWithSetup's own generation loop does, so the pristine base and the
// real target always agree on whether such a file exists.
func loadOldScaffoldConfig(oldConfig *tmpl.Configuration) (*config.ScaffoldConfig, []string, error) {
	var oldScaffoldConfigFile *tmpl.File
	for i := range oldConfig.Files {
		if oldConfig.Files[i].Path == config.ScaffoldConfigFileName {
			oldScaffoldConfigFile = &oldConfig.Files[i]
			break
		}
	}
	if oldScaffoldConfigFile == nil {
		return nil, nil, errUtils.Build(errUtils.ErrScaffoldConfigMissing).
			WithExplanationf("%s not found in the old ref's rendered configuration", config.ScaffoldConfigFileName).
			WithHint("--update-strategy=rendered requires the template to carry a scaffold.yaml at every ref it's updated across").
			Err()
	}

	var includedPaths []string
	oldScaffoldConfig, err := config.LoadScaffoldConfigFromContent(
		oldScaffoldConfigFile.Content, config.WithSourceDir(oldConfig.Source), config.WithIncludedPaths(&includedPaths),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load the old ref's scaffold configuration: %w", err)
	}
	return oldScaffoldConfig, includedPaths, nil
}

// renderPristineBase renders oldConfig -- a template Configuration fetched at
// the ref that produced what's currently on disk -- into a fresh temp
// directory using oldValues (that generation's own recorded answers), with
// no hooks, no UI output, and no project-record write. It exists purely to
// give engine.UpdateStrategyRendered's 3-way merge a base to read from (see
// engine.Processor.SetupRenderedBaseStorage); the caller is responsible for
// invoking the returned cleanup once the merge that consumes it is done.
//
// It reuses ui.processFileEntry -- the same per-file loop executeWithSetup
// uses for a real generation, including matrix expansion -- with
// force=true, update=false so every file is a plain overwrite into an
// otherwise-empty directory, never touching merge/hooks itself.
func (ui *InitUI) renderPristineBase(oldConfig *tmpl.Configuration, oldValues map[string]interface{}, delimiters []string) (tempDir string, cleanup func(), err error) {
	oldScaffoldConfig, includedPaths, err := loadOldScaffoldConfig(oldConfig)
	if err != nil {
		return "", nil, err
	}
	mergedOldValues := config.DeepMerge(oldScaffoldConfig, oldValues)

	tempDir, err = os.MkdirTemp("", "atmos-rendered-base-")
	if err != nil {
		return "", nil, fmt.Errorf("failed to create a temp directory for the rendered base: %w", err)
	}
	cleanup = func() { _ = os.RemoveAll(tempDir) }

	// Discard UI output produced by the reused per-file loop below. This
	// render is a purely internal step; its progress lines must never
	// interleave with the real run's own output buffer. strings.Builder must
	// never be copied by value once used (a copy's internal address check
	// panics on the next write), so save/restore its string content rather
	// than the Builder itself.
	savedOutput := ui.output.String()
	ui.output.Reset()
	defer func() {
		ui.output.Reset()
		_, _ = ui.output.WriteString(savedOutput)
	}()

	// This internal render must always actually write oldConfig's files to
	// tempDir, even when the outer run is a --dry-run preview:
	// engine.Processor.ProcessFile skips the actual disk write whenever
	// Processor.DryRun is set, and this render shares ui.processor with the
	// real run (see ui.processFileEntry -> ui.writeOneOutput ->
	// ui.processor.ProcessFile). Left as-is, SetupRenderedBaseStorage below
	// would be pointed at an empty tempDir and a --dry-run preview under
	// --update-strategy=rendered would have no base to diff against. Save
	// and restore rather than leaving it disabled, since the real run
	// continuing after this function returns still needs its own DryRun
	// behavior intact.
	dryRun := ui.processor.DryRun
	ui.processor.SetDryRun(false)
	defer ui.processor.SetDryRun(dryRun)

	if err := ui.renderPristineBaseFiles(&renderPristineBaseFilesArgs{
		oldConfig:         oldConfig,
		oldScaffoldConfig: oldScaffoldConfig,
		mergedOldValues:   mergedOldValues,
		tempDir:           tempDir,
		delimiters:        delimiters,
		includedPaths:     includedPaths,
	}); err != nil {
		cleanup()
		return "", nil, err
	}

	return tempDir, cleanup, nil
}

// renderPristineBaseFilesArgs bundles renderPristineBaseFiles's parameters
// (grouped into a struct, rather than six separate parameters, to stay
// under revive's argument-limit).
type renderPristineBaseFilesArgs struct {
	oldConfig         *tmpl.Configuration
	oldScaffoldConfig *config.ScaffoldConfig
	mergedOldValues   map[string]interface{}
	tempDir           string
	delimiters        []string
	includedPaths     []string
}

// renderPristineBaseFiles loops oldConfig's files (skipping scaffold.yaml,
// directory entries, and !include-consumed files -- see
// loadOldScaffoldConfig) and renders each into tempDir via
// ui.processFileEntry, joining any per-file failures into a single error.
func (ui *InitUI) renderPristineBaseFiles(args *renderPristineBaseFilesArgs) error {
	activeDelimiters := ResolveDelimiters(args.delimiters, args.oldScaffoldConfig)
	fileSpecs := FileSpecByPath(args.oldScaffoldConfig, args.oldConfig.Files)
	seenRenderedPaths := make(map[string]string)
	matrixExpansions := make(map[string]matrixExpansionResult)
	includedSet := includedPathSet(args.includedPaths)

	var failureErrs []error
	for _, file := range args.oldConfig.Files {
		if file.Path == config.ScaffoldConfigFileName || file.IsDirectory || includedSet[file.Path] {
			continue
		}

		spec := fileSpecs[file.Path]
		_, _, _, entryErr := ui.processFileEntry(file, spec, args.tempDir, true, false, args.oldScaffoldConfig, args.mergedOldValues, activeDelimiters, seenRenderedPaths, matrixExpansions)
		if entryErr != nil {
			failureErrs = append(failureErrs, entryErr)
		}
	}

	if len(failureErrs) > 0 {
		return errUtils.Build(errUtils.ErrScaffoldGeneration).
			WithCause(errors.Join(failureErrs...)).
			WithExplanation("Failed to render the old ref's template for the rendered update-strategy base").
			Err()
	}
	return nil
}
