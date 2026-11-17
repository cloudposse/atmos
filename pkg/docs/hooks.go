package docs

import (
	"errors"
	"path/filepath"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/hooks/events"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// HookRunnerRequest describes the docs generation a HookRunner serves.
type HookRunnerRequest struct {
	// AtmosConfig is the active configuration.
	AtmosConfig *schema.AtmosConfiguration
	// DocType is the docs.generate.<doc-type> key.
	DocType string
	// BaseDir is the absolute docs.generate.<doc-type>.base-dir.
	BaseDir string
	// OutputPath is the absolute path of the document being generated.
	OutputPath string
	// Hooks is the untyped docs.generate.<doc-type>.hooks map.
	Hooks map[string]any
	// Cmd is the running command, used for --skip-hooks.
	Cmd *cobra.Command
}

// HookRunner fires lifecycle hooks around document generation.
type HookRunner interface {
	// Run fires event. genErr is the generation result for after events and nil for before
	// events. The returned error already reflects each hook's on_failure handling and any
	// policy denial.
	Run(event events.HookEvent, genErr error) error
}

// HookRunnerFactory builds a HookRunner for one generation.
type HookRunnerFactory func(req *HookRunnerRequest) (HookRunner, error)

// hookRunnerFactory is registered by pkg/hooks. The hook engine depends on the exec pipeline,
// which depends on this package, so this package cannot import it directly.
//
//nolint:gochecknoglobals // registry seam that breaks the pkg/hooks -> internal/exec -> pkg/docs import cycle.
var hookRunnerFactory HookRunnerFactory

// RegisterHookRunnerFactory installs the factory that supplies lifecycle hooks to Generate
// callers. The pkg/hooks package registers it at init; a nil factory disables hooks.
func RegisterHookRunnerFactory(factory HookRunnerFactory) {
	defer perf.Track(nil, "docs.RegisterHookRunnerFactory")()

	hookRunnerFactory = factory
}

// GenerateWithHooks generates the document configured under docs.generate.<docType> and runs the
// before.docs.generate and after.docs.generate lifecycle hooks around it. A before hook failure
// (or a policy denial) aborts before any file is written. After hooks run on success and on
// failure, filtered by each hook's `when`.
func GenerateWithHooks(atmosConfig *schema.AtmosConfiguration, cmd *cobra.Command, docType string, docsGenerate *schema.DocsGenerate) error {
	defer perf.Track(atmosConfig, "docs.GenerateWithHooks")()

	baseDir, err := filepath.Abs(docsGenerate.BaseDir)
	if err != nil {
		return errors.Join(errUtils.ErrPathResolution, err)
	}
	outputPath, err := resolvePath(docsGenerate.Output, baseDir, defaultReadmeOutput)
	if err != nil {
		return errors.Join(errUtils.ErrResolveOutputPath, err)
	}

	var runner HookRunner
	if hookRunnerFactory == nil && len(docsGenerate.Hooks) > 0 {
		// Never drop configured hooks silently: they may gate or publish the document.
		return errUtils.Build(errUtils.ErrDocsHooksUnavailable).
			WithHint("Link the hook engine into the binary by importing github.com/cloudposse/atmos/pkg/hooks, or remove docs.generate.<doc-type>.hooks.").
			Err()
	}
	if hookRunnerFactory != nil {
		runner, err = hookRunnerFactory(&HookRunnerRequest{
			AtmosConfig: atmosConfig, DocType: docType, BaseDir: baseDir,
			OutputPath: outputPath, Hooks: docsGenerate.Hooks, Cmd: cmd,
		})
		if err != nil {
			return err
		}
	}
	return runLifecycle(atmosConfig, baseDir, docsGenerate, defaultTemplateRenderer{}, runner)
}

// runLifecycle runs before hooks, generates the document, then runs after hooks. A nil runner
// generates without hooks.
func runLifecycle(atmosConfig *schema.AtmosConfiguration, baseDir string, docsGenerate *schema.DocsGenerate, renderer templateRenderer, runner HookRunner) error {
	if runner == nil {
		_, err := generateDocument(atmosConfig, baseDir, docsGenerate, renderer)
		return err
	}
	if err := runner.Run(events.BeforeDocsGenerate, nil); err != nil {
		return err
	}
	outputPath, genErr := generateDocument(atmosConfig, baseDir, docsGenerate, renderer)
	if genErr != nil {
		log.Debug("Docs generation failed, running after hooks for failure", "error", genErr)
	} else {
		log.Debug("Docs generated, running after hooks", "output", outputPath)
	}
	return errors.Join(genErr, runner.Run(events.AfterDocsGenerate, genErr))
}
