package hooks

import (
	"github.com/cloudposse/atmos/pkg/docs"
	"github.com/cloudposse/atmos/pkg/hooks/events"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Docs generate operation identifiers shared by policy input, templates, and hook environment.
const (
	docsOperationType = "docs"

	// DocsTypeEnvVar, DocsOutputPathEnvVar, and DocsBaseDirEnvVar are exported to hooks that fire
	// around `atmos docs generate`.
	DocsTypeEnvVar       = "ATMOS_DOCS_TYPE"
	DocsOutputPathEnvVar = "ATMOS_DOCS_OUTPUT_PATH"
	DocsBaseDirEnvVar    = "ATMOS_DOCS_BASE_DIR"
)

//nolint:gochecknoinits // registers the docs hook runner; pkg/docs cannot import this package without an import cycle.
func init() {
	docs.RegisterHookRunnerFactory(newDocsHookRunner)
}

// docsHookRunner adapts Hooks to the docs.HookRunner interface.
type docsHookRunner struct {
	hooks *Hooks
	req   *docs.HookRunnerRequest
}

// newDocsHookRunner builds the hooks declared under docs.generate.<type>.hooks, together with
// any governing policy, for one generation.
func newDocsHookRunner(req *docs.HookRunnerRequest) (docs.HookRunner, error) {
	h, err := NewOperationHooks(req.AtmosConfig, req.Hooks, docsOperationContext(req))
	if err != nil {
		return nil, err
	}
	return &docsHookRunner{hooks: h, req: req}, nil
}

// Run fires event with the lifecycle result recorded, so hooks filter on `when`.
func (r *docsHookRunner) Run(event events.HookEvent, genErr error) error {
	defer perf.Track(r.req.AtmosConfig, "hooks.docsHookRunner.Run")()

	if !r.hooks.HasHooks() {
		return nil
	}
	outcome := Outcome{Status: RunSuccess}
	if genErr != nil {
		outcome = Outcome{Status: RunFailure, Err: genErr, ExitCode: 1}
	}
	r.hooks.SetOutcome(outcome)
	return r.hooks.RunOperation(event, r.req.Cmd)
}

// docsOperationContext exposes the document type, output path, and base directory to hooks as
// environment variables and as `.docs.*` template data.
func docsOperationContext(req *docs.HookRunnerRequest) OperationContext {
	return OperationContext{
		Type:       docsOperationType,
		Name:       req.DocType,
		ConfigPath: "docs.generate." + req.DocType + ".hooks",
		Env: map[string]string{
			DocsTypeEnvVar:       req.DocType,
			DocsOutputPathEnvVar: req.OutputPath,
			DocsBaseDirEnvVar:    req.BaseDir,
		},
		Data: map[string]any{
			"type":        req.DocType,
			"output_path": req.OutputPath,
			"base_dir":    req.BaseDir,
		},
	}
}
