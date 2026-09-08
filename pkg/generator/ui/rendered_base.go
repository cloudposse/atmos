package ui

import (
	"errors"
	"fmt"
	"os"
	"strings"

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
// than the current run's.
func loadOldScaffoldConfig(oldConfig *tmpl.Configuration) (*config.ScaffoldConfig, error) {
	var oldScaffoldConfigFile *tmpl.File
	for i := range oldConfig.Files {
		if oldConfig.Files[i].Path == config.ScaffoldConfigFileName {
			oldScaffoldConfigFile = &oldConfig.Files[i]
			break
		}
	}
	if oldScaffoldConfigFile == nil {
		return nil, errUtils.Build(errUtils.ErrScaffoldConfigMissing).
			WithExplanationf("%s not found in the old ref's rendered configuration", config.ScaffoldConfigFileName).
			WithHint("--update-strategy=rendered requires the template to carry a scaffold.yaml at every ref it's updated across").
			Err()
	}

	oldScaffoldConfig, err := config.LoadScaffoldConfigFromContent(oldScaffoldConfigFile.Content)
	if err != nil {
		return nil, fmt.Errorf("failed to load the old ref's scaffold configuration: %w", err)
	}
	return oldScaffoldConfig, nil
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
func (ui *InitUI) renderPristineBase(oldConfig *tmpl.Configuration, oldValues map[string]interface{}) (tempDir string, cleanup func(), err error) {
	oldScaffoldConfig, err := loadOldScaffoldConfig(oldConfig)
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
	// interleave with the real run's own output buffer.
	savedOutput := ui.output
	ui.output = strings.Builder{}
	defer func() { ui.output = savedOutput }()

	if err := ui.renderPristineBaseFiles(oldConfig, oldScaffoldConfig, mergedOldValues, tempDir); err != nil {
		cleanup()
		return "", nil, err
	}

	return tempDir, cleanup, nil
}

// renderPristineBaseFiles loops oldConfig's files (skipping scaffold.yaml
// and directory entries) and renders each into tempDir via
// ui.processFileEntry, joining any per-file failures into a single error.
func (ui *InitUI) renderPristineBaseFiles(oldConfig *tmpl.Configuration, oldScaffoldConfig *config.ScaffoldConfig, mergedOldValues map[string]interface{}, tempDir string) error {
	activeDelimiters := ResolveDelimiters(nil, oldScaffoldConfig)
	fileSpecs := FileSpecByPath(oldScaffoldConfig)
	seenRenderedPaths := make(map[string]string)

	var failureErrs []error
	for _, file := range oldConfig.Files {
		if file.Path == config.ScaffoldConfigFileName || file.IsDirectory {
			continue
		}

		spec := fileSpecs[file.Path]
		_, _, _, entryErr := ui.processFileEntry(file, spec, tempDir, true, false, oldScaffoldConfig, mergedOldValues, activeDelimiters, seenRenderedPaths)
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
