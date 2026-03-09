package terraform

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/ci/internal/plugin"
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
	"github.com/cloudposse/atmos/pkg/ci/plugins/terraform/planfile"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	tfoutput "github.com/cloudposse/atmos/pkg/terraform/output"
	"github.com/cloudposse/atmos/pkg/version"
)

// onBeforePlan handles the before.terraform.plan event.
// Creates a check run with in_progress status.
func (p *Plugin) onBeforePlan(ctx *plugin.HookContext) error {
	defer perf.Track(ctx.Config, "terraform.Plugin.onBeforePlan")()

	if isCheckEnabled(ctx.Config) {
		if err := p.createCheckRun(ctx); err != nil {
			log.Warn("CI check run creation failed", "error", err)
		}
	}
	return nil
}

// onAfterPlan handles the after.terraform.plan event.
// Writes summary, outputs, uploads planfile, and updates check run.
func (p *Plugin) onAfterPlan(ctx *plugin.HookContext) error {
	defer perf.Track(ctx.Config, "terraform.Plugin.onAfterPlan")()

	result := p.parseOutputWithError(ctx)

	// Summary -- warn-only.
	var renderedSummary string
	if isSummaryEnabled(ctx.Config) {
		var err error
		renderedSummary, err = p.writeSummary(ctx, result)
		if err != nil {
			log.Warn("CI summary failed", "error", err)
		}
	}

	// Output -- warn-only.
	if isOutputEnabled(ctx.Config) {
		if err := p.writeOutputs(ctx, result, renderedSummary); err != nil {
			log.Warn("CI output failed", "error", err)
		}
	}

	// Upload -- FATAL (downstream apply depends on it).
	if err := p.uploadPlanfile(ctx); err != nil {
		return err
	}

	// Check -- warn-only.
	if isCheckEnabled(ctx.Config) {
		if err := p.updateCheckRun(ctx, result); err != nil {
			log.Warn("CI check run update failed", "error", err)
		}
	}

	return nil
}

// onBeforeApply handles the before.terraform.apply event.
// Creates a check run with in_progress status.
func (p *Plugin) onBeforeApply(ctx *plugin.HookContext) error {
	defer perf.Track(ctx.Config, "terraform.Plugin.onBeforeApply")()

	if isCheckEnabled(ctx.Config) {
		if err := p.createCheckRun(ctx); err != nil {
			log.Warn("CI check run creation failed", "error", err)
		}
	}
	return nil
}

// onAfterApply handles the after.terraform.apply event.
// Writes summary, outputs, and updates check run.
func (p *Plugin) onAfterApply(ctx *plugin.HookContext) error {
	defer perf.Track(ctx.Config, "terraform.Plugin.onAfterApply")()

	result := p.parseOutputWithError(ctx)

	// Summary -- warn-only.
	var renderedSummary string
	if isSummaryEnabled(ctx.Config) {
		var err error
		renderedSummary, err = p.writeSummary(ctx, result)
		if err != nil {
			log.Warn("CI summary failed", "error", err)
		}
	}

	// Output -- warn-only.
	if isOutputEnabled(ctx.Config) {
		if err := p.writeOutputs(ctx, result, renderedSummary); err != nil {
			log.Warn("CI output failed", "error", err)
		}
	}

	// Check -- warn-only.
	if isCheckEnabled(ctx.Config) {
		if err := p.updateCheckRun(ctx, result); err != nil {
			log.Warn("CI check run update failed", "error", err)
		}
	}

	return nil
}

// onBeforeDeploy handles the before.terraform.deploy event.
// Downloads planfile from storage with stored prefix for verification.
// Download is warn-only: deploy can proceed without a stored planfile.
// The --verify-plan gate in RunE checks if the stored file exists on disk.
func (p *Plugin) onBeforeDeploy(ctx *plugin.HookContext) error {
	defer perf.Track(ctx.Config, "terraform.Plugin.onBeforeDeploy")()

	// Create check run if enabled.
	if isCheckEnabled(ctx.Config) {
		if err := p.createCheckRun(ctx); err != nil {
			log.Warn("CI check run creation failed", "error", err)
		}
	}

	// Download -- warn-only (deploy works without a stored planfile).
	if err := p.downloadPlanfileForVerification(ctx); err != nil {
		log.Warn("CI hook handler failed", "event", "before.terraform.deploy", "error", err)
	}
	return nil
}

// onAfterDeploy handles the after.terraform.deploy event.
// Deploy is semantically apply for CI purposes — delegates to onAfterApply
// so that apply.md template is used and terraform outputs are fetched.
func (p *Plugin) onAfterDeploy(ctx *plugin.HookContext) error {
	defer perf.Track(ctx.Config, "terraform.Plugin.onAfterDeploy")()

	ctx.Command = "apply"
	return p.onAfterApply(ctx)
}

// downloadPlanfileForVerification downloads a planfile from storage with a stored prefix
// so terraform can generate a fresh plan at the canonical path for comparison.
func (p *Plugin) downloadPlanfileForVerification(ctx *plugin.HookContext) error {
	defer perf.Track(ctx.Config, "terraform.Plugin.downloadPlanfileForVerification")()

	planfilePath := ctx.Info.PlanFile
	if planfilePath == "" {
		planfilePath = p.resolveArtifactPath(ctx)
	}
	if planfilePath == "" {
		log.Debug("No planfile path specified, skipping download")
		return nil
	}

	key, err := p.getArtifactKey(ctx.Info, ctx.CICtx)
	if err != nil {
		return errUtils.Build(errUtils.ErrPlanfileDownloadFailed).WithCause(err).
			WithExplanation("Failed to generate artifact key for planfile download").Err()
	}

	storeAny, err := ctx.CreatePlanfileStore()
	if err != nil {
		return errUtils.Build(errUtils.ErrPlanfileDownloadFailed).WithCause(err).
			WithExplanation("Failed to create planfile store").Err()
	}

	store, ok := storeAny.(planfile.Store)
	if !ok {
		return errUtils.Build(errUtils.ErrPlanfileDownloadFailed).
			WithExplanation("Invalid planfile store type").Err()
	}

	results, _, err := store.Download(context.Background(), key)
	if err != nil {
		return errUtils.Build(errUtils.ErrPlanfileDownloadFailed).WithCause(err).
			WithExplanation("Failed to download planfile from store").
			WithContext("key", key).WithContext("store", store.Name()).Err()
	}
	defer func() {
		for _, r := range results {
			r.Data.Close()
		}
	}()

	// Always write with stored prefix so terraform can generate a fresh plan
	// at the canonical path for comparison during verification.
	dir := filepath.Dir(planfilePath)
	storedPlanPath := filepath.Join(dir, planfile.StoredPlanPrefix+planfile.PlanFilename)

	if err := planfile.WritePlanfileResultsForVerification(results, storedPlanPath, planfilePath); err != nil {
		return err
	}

	ctx.Info.StoredPlanFile = storedPlanPath
	logArtifactOperation("Downloaded (stored for verification)", key, store.Name(), storedPlanPath, ctx.Info)

	return nil
}

// parseOutputWithError parses command output and enriches with command error info.
func (p *Plugin) parseOutputWithError(ctx *plugin.HookContext) *plugin.OutputResult {
	result := ParseOutput(ctx.Output, ctx.Command)

	// If the command had an error, ensure the result reflects that.
	if ctx.CommandError != nil {
		result.HasErrors = true
		if result.ExitCode == 0 {
			result.ExitCode = 1
		}
		if len(result.Errors) == 0 {
			result.Errors = []string{ctx.CommandError.Error()}
		}
	}

	return result
}

// writeSummary renders and writes the CI job summary.
func (p *Plugin) writeSummary(ctx *plugin.HookContext, _ *plugin.OutputResult) (string, error) {
	defer perf.Track(ctx.Config, "terraform.Plugin.writeSummary")()

	// Get template name - prefer config override, fall back to command name.
	templateName := ctx.Command
	if ctx.Config != nil && ctx.Config.CI.Summary.Template != "" {
		templateName = ctx.Config.CI.Summary.Template
	}

	if templateName == "" {
		return "", nil
	}

	writer := ctx.Provider.OutputWriter()
	if writer == nil {
		log.Debug("CI platform does not support summaries")
		return "", nil
	}

	// Build template context.
	tmplCtx, err := p.buildTemplateContext(ctx.Info, ctx.CICtx, ctx.Output, ctx.Command)
	if err != nil {
		return "", errUtils.Build(errUtils.ErrTemplateEvaluation).
			WithCause(err).
			WithExplanation("Failed to build template context").
			Err()
	}

	// Load and render template.
	rendered, err := ctx.TemplateLoader.LoadAndRender(
		"terraform",
		templateName,
		defaultTemplates,
		tmplCtx,
	)
	if err != nil {
		return "", errUtils.Build(errUtils.ErrTemplateEvaluation).
			WithCause(err).
			WithExplanation("Failed to render template").
			WithContext("template", templateName).
			Err()
	}

	// Write to job summary.
	if err := writer.WriteSummary(rendered); err != nil {
		return "", errUtils.Build(errUtils.ErrCIOutputWriteFailed).
			WithCause(err).
			WithExplanation("Failed to write CI summary").
			Err()
	}

	log.Debug("Wrote CI summary",
		"stack", ctx.Info.Stack,
		"component", ctx.Info.ComponentFromArg,
		"template", templateName,
	)
	return rendered, nil
}

// getComponentOutputsFunc is the function used to fetch terraform outputs.
// It defaults to tfoutput.GetComponentOutputs and can be overridden in tests.
var getComponentOutputsFunc = tfoutput.GetComponentOutputs

// writeOutputs writes CI output variables.
func (p *Plugin) writeOutputs(ctx *plugin.HookContext, result *plugin.OutputResult, renderedSummary string) error {
	defer perf.Track(ctx.Config, "terraform.Plugin.writeOutputs")()

	writer := ctx.Provider.OutputWriter()
	if writer == nil {
		log.Debug("CI platform does not support outputs")
		return nil
	}

	// Get output variables from parsed result.
	vars := p.getOutputVariables(result, ctx.Command)

	// Add common variables.
	vars["stack"] = ctx.Info.Stack
	vars["component"] = ctx.Info.ComponentFromArg
	vars["command"] = ctx.Command

	// Add rendered summary if available.
	if renderedSummary != "" {
		vars["summary"] = renderedSummary
	}

	// Filter native CI variables by configured whitelist if specified.
	if ctx.Config != nil && len(ctx.Config.CI.Output.Variables) > 0 {
		vars = filterVariables(vars, ctx.Config.CI.Output.Variables)
	}

	// Export terraform outputs after successful apply.
	// Terraform outputs bypass the whitelist — they are always included.
	if ctx.Command == "apply" && ctx.CommandError == nil {
		if tfOutputs := p.getTerraformOutputs(ctx); len(tfOutputs) > 0 {
			flatOutputs := tfoutput.FlattenMap(tfOutputs, "output", "_")
			for key, value := range flatOutputs {
				vars[key] = fmt.Sprintf("%v", value)
			}
		}
	}

	// Write each variable.
	for key, value := range vars {
		if err := writer.WriteOutput(key, value); err != nil {
			log.Warn("Failed to write CI output", "key", key, "error", err)
		}
	}

	log.Debug("Wrote CI outputs", "count", len(vars))
	return nil
}

// getTerraformOutputs fetches terraform outputs after a successful apply.
// Returns nil if the command is not apply, if apply failed, or if output fetching fails.
func (p *Plugin) getTerraformOutputs(ctx *plugin.HookContext) map[string]any {
	if ctx.Command != "apply" || ctx.CommandError != nil {
		return nil
	}
	if ctx.Config == nil || ctx.Info == nil {
		return nil
	}

	outputs, err := getComponentOutputsFunc(
		ctx.Config,
		ctx.Info.ComponentFromArg,
		ctx.Info.Stack,
		true, // skipInit — already initialized from apply.
		nil,  // no authManager.
	)
	if err != nil {
		log.Warn("Failed to fetch terraform outputs for CI export", "error", err)
		return nil
	}
	return outputs
}

// uploadPlanfile uploads a planfile to the configured storage backend.
func (p *Plugin) uploadPlanfile(ctx *plugin.HookContext) error {
	defer perf.Track(ctx.Config, "terraform.Plugin.uploadPlanfile")()

	planfilePath := ctx.Info.PlanFile
	if planfilePath == "" {
		planfilePath = p.resolveArtifactPath(ctx)
	}
	if planfilePath == "" {
		log.Debug("No planfile path specified, skipping upload")
		return nil
	}
	if _, err := os.Stat(planfilePath); os.IsNotExist(err) {
		log.Debug("Planfile does not exist, skipping upload", "path", planfilePath)
		return nil
	}

	key, err := p.getArtifactKey(ctx.Info, ctx.CICtx)
	if err != nil {
		return errUtils.Build(errUtils.ErrPlanfileUploadFailed).WithCause(err).
			WithExplanation("Failed to generate artifact key for planfile upload").Err()
	}

	storeAny, err := ctx.CreatePlanfileStore()
	if err != nil {
		return errUtils.Build(errUtils.ErrPlanfileUploadFailed).WithCause(err).
			WithExplanation("Failed to create planfile store").Err()
	}

	store, ok := storeAny.(planfile.Store)
	if !ok {
		return errUtils.Build(errUtils.ErrPlanfileUploadFailed).
			WithExplanation("Invalid planfile store type").Err()
	}

	planFile, err := os.Open(planfilePath)
	if err != nil {
		return errUtils.Build(errUtils.ErrPlanfileUploadFailed).WithCause(err).
			WithExplanation("Failed to open planfile for upload").WithContext("path", planfilePath).Err()
	}
	defer planFile.Close()

	// Build file entries for upload.
	files := []planfile.FileEntry{
		{Name: planfile.PlanFilename, Data: planFile, Size: -1},
	}

	// Open lock file if it exists alongside the planfile.
	lockPath := filepath.Join(filepath.Dir(planfilePath), planfile.LockFilename)
	if lf, err := os.Open(lockPath); err == nil {
		defer lf.Close()
		files = append(files, planfile.FileEntry{Name: planfile.LockFilename, Data: lf, Size: -1})
	}

	metadata := p.buildPlanfileMetadata(ctx)
	if err := metadata.Validate(); err != nil {
		return errUtils.Build(errUtils.ErrPlanfileUploadFailed).WithCause(err).
			WithExplanation("Planfile metadata validation failed").Err()
	}

	if err := store.Upload(context.Background(), key, files, metadata); err != nil {
		return errUtils.Build(errUtils.ErrPlanfileUploadFailed).WithCause(err).
			WithExplanation("Failed to upload planfile to store").
			WithContext("key", key).WithContext("store", store.Name()).Err()
	}

	logArtifactOperation("Uploaded", key, store.Name(), "", ctx.Info)
	return nil
}

// downloadPlanfile downloads a planfile from the configured storage backend.
func (p *Plugin) downloadPlanfile(ctx *plugin.HookContext) error {
	defer perf.Track(ctx.Config, "terraform.Plugin.downloadPlanfile")()

	planfilePath := ctx.Info.PlanFile
	if planfilePath == "" {
		planfilePath = p.resolveArtifactPath(ctx)
	}
	if planfilePath == "" {
		log.Debug("No planfile path specified, skipping download")
		return nil
	}

	key, err := p.getArtifactKey(ctx.Info, ctx.CICtx)
	if err != nil {
		return errUtils.Build(errUtils.ErrPlanfileDownloadFailed).WithCause(err).
			WithExplanation("Failed to generate artifact key for planfile download").Err()
	}

	storeAny, err := ctx.CreatePlanfileStore()
	if err != nil {
		return errUtils.Build(errUtils.ErrPlanfileDownloadFailed).WithCause(err).
			WithExplanation("Failed to create planfile store").Err()
	}

	store, ok := storeAny.(planfile.Store)
	if !ok {
		return errUtils.Build(errUtils.ErrPlanfileDownloadFailed).
			WithExplanation("Invalid planfile store type").Err()
	}

	results, _, err := store.Download(context.Background(), key)
	if err != nil {
		return errUtils.Build(errUtils.ErrPlanfileDownloadFailed).WithCause(err).
			WithExplanation("Failed to download planfile from store").
			WithContext("key", key).WithContext("store", store.Name()).Err()
	}
	defer func() {
		for _, r := range results {
			r.Data.Close()
		}
	}()

	// Write downloaded files to disk using the shared helper.
	if err := planfile.WritePlanfileResults(results, planfilePath); err != nil {
		return err
	}
	logArtifactOperation("Downloaded", key, store.Name(), planfilePath, ctx.Info)

	return nil
}

// createCheckRun creates a new check run with in_progress status.
func (p *Plugin) createCheckRun(ctx *plugin.HookContext) error {
	defer perf.Track(ctx.Config, "terraform.Plugin.createCheckRun")()

	name := provider.FormatCheckRunName(ctx.Command, ctx.Info.Stack, ctx.Info.ComponentFromArg)

	opts := &provider.CreateCheckRunOptions{
		Name:   name,
		Status: provider.CheckRunStateInProgress,
		Title:  fmt.Sprintf("Running %s...", ctx.Command),
	}

	if ctx.CICtx != nil {
		opts.Owner = ctx.CICtx.RepoOwner
		opts.Repo = ctx.CICtx.RepoName
		opts.SHA = ctx.CICtx.SHA
	}

	checkRun, err := ctx.Provider.CreateCheckRun(context.Background(), opts)
	if err != nil {
		return errUtils.Build(errUtils.ErrCICheckRunCreateFailed).
			WithCause(err).
			WithContext("name", name).
			Err()
	}

	log.Debug("Created check run", "name", name, "id", checkRun.ID)
	return nil
}

// updateCheckRun updates an existing check run with the final result.
// The provider handles ID correlation internally (or falls back to creating
// a new completed check run if no prior CreateCheckRun was called).
func (p *Plugin) updateCheckRun(ctx *plugin.HookContext, result *plugin.OutputResult) error {
	defer perf.Track(ctx.Config, "terraform.Plugin.updateCheckRun")()

	name := provider.FormatCheckRunName(ctx.Command, ctx.Info.Stack, ctx.Info.ComponentFromArg)
	status, conclusion := resolveCheckResult(ctx)
	now := time.Now()

	opts := &provider.UpdateCheckRunOptions{
		Name:        name,
		Status:      status,
		Conclusion:  conclusion,
		Title:       buildCheckTitle(ctx.Command, result),
		Summary:     buildCheckSummary(result),
		CompletedAt: &now,
	}

	if ctx.CICtx != nil {
		opts.Owner = ctx.CICtx.RepoOwner
		opts.Repo = ctx.CICtx.RepoName
		opts.SHA = ctx.CICtx.SHA
	}

	_, err := ctx.Provider.UpdateCheckRun(context.Background(), opts)
	if err != nil {
		return errUtils.Build(errUtils.ErrCICheckRunUpdateFailed).
			WithCause(err).
			WithContext("name", name).
			Err()
	}

	log.Debug("Updated check run", "name", name, "status", status)
	return nil
}

// resolveArtifactPath derives the planfile path from component and stack information.
func (p *Plugin) resolveArtifactPath(ctx *plugin.HookContext) string {
	if ctx.Info.Stack == "" || ctx.Info.ComponentFromArg == "" {
		return ""
	}

	resolved, err := e.ProcessStacks(ctx.Config, *ctx.Info, true, false, false, nil, nil)
	if err != nil {
		log.Debug("Failed to resolve artifact path", "error", err)
		return ""
	}

	// Carry over resolved fields needed for metadata.
	ctx.Info.ContextPrefix = resolved.ContextPrefix
	ctx.Info.Component = resolved.Component
	ctx.Info.FinalComponent = resolved.FinalComponent
	ctx.Info.ComponentFolderPrefix = resolved.ComponentFolderPrefix
	ctx.Info.ComponentFolderPrefixReplaced = resolved.ComponentFolderPrefixReplaced
	ctx.Info.ComponentSection = resolved.ComponentSection

	path := e.ConstructTerraformComponentPlanfilePath(ctx.Config, &resolved)
	if path != "" {
		ctx.Info.PlanFile = path
	}
	return path
}

// buildPlanfileMetadata builds metadata for a planfile.
func (p *Plugin) buildPlanfileMetadata(ctx *plugin.HookContext) *planfile.Metadata {
	defer perf.Track(ctx.Config, "terraform.Plugin.buildPlanfileMetadata")()

	result := ParseOutput(ctx.Output, ctx.Command)

	metadata := &planfile.Metadata{}
	metadata.Stack = ctx.Info.Stack
	metadata.Component = ctx.Info.ComponentFromArg
	metadata.CreatedAt = time.Now()
	metadata.AtmosVersion = version.Version

	// Add CI context if available.
	if ctx.CICtx != nil {
		metadata.SHA = ctx.CICtx.SHA
		metadata.Branch = ctx.CICtx.Branch
		metadata.RunID = ctx.CICtx.RunID
		metadata.Repository = ctx.CICtx.Repository
		if ctx.CICtx.PullRequest != nil {
			metadata.PRNumber = ctx.CICtx.PullRequest.Number
		}
	}

	// Add plan result data if available.
	if result != nil {
		metadata.HasChanges = result.HasChanges
		if tfData, ok := result.Data.(*plugin.TerraformOutputData); ok {
			metadata.Additions = tfData.ResourceCounts.Create
			metadata.Changes = tfData.ResourceCounts.Change
			metadata.Destructions = tfData.ResourceCounts.Destroy
			metadata.PlanSummary = tfData.ChangedResult
		}
	}

	return metadata
}

// isActionEnabled helpers.

// isSummaryEnabled checks if summary action is enabled in config.
func isSummaryEnabled(cfg *schema.AtmosConfiguration) bool {
	if cfg == nil {
		return true
	}
	if cfg.CI.Summary.Enabled == nil {
		return true
	}
	return *cfg.CI.Summary.Enabled
}

// isOutputEnabled checks if output action is enabled in config.
func isOutputEnabled(cfg *schema.AtmosConfiguration) bool {
	if cfg == nil {
		return true
	}
	if cfg.CI.Output.Enabled == nil {
		return true
	}
	return *cfg.CI.Output.Enabled
}

// isCheckEnabled checks if check action is enabled in config.
func isCheckEnabled(cfg *schema.AtmosConfiguration) bool {
	if cfg == nil {
		return false
	}
	if cfg.CI.Checks.Enabled == nil {
		return false
	}
	return *cfg.CI.Checks.Enabled
}

// filterVariables filters a map of variables to only include those in the allowed list.
func filterVariables(vars map[string]string, allowed []string) map[string]string {
	if len(allowed) == 0 {
		return vars
	}
	allowedSet := make(map[string]bool)
	for _, v := range allowed {
		allowedSet[v] = true
	}
	filtered := make(map[string]string)
	for k, v := range vars {
		if allowedSet[k] {
			filtered[k] = v
		}
	}
	return filtered
}

// resolveCheckResult determines the check run status and conclusion from the hook context.
func resolveCheckResult(ctx *plugin.HookContext) (provider.CheckRunState, string) {
	if ctx.CommandError != nil {
		return provider.CheckRunStateFailure, "failure"
	}
	return provider.CheckRunStateSuccess, "success"
}

// buildCheckTitle creates a human-readable title for a completed check run.
func buildCheckTitle(command string, result *plugin.OutputResult) string {
	if result != nil {
		if tfData, ok := result.Data.(*plugin.TerraformOutputData); ok && tfData.ChangedResult != "" {
			return tfData.ChangedResult
		}

		if result.HasChanges {
			return fmt.Sprintf("%s: changes detected", command)
		}
	}

	return fmt.Sprintf("%s: no changes", command)
}

// buildCheckSummary creates a brief summary for a completed check run.
func buildCheckSummary(result *plugin.OutputResult) string {
	if result != nil {
		if tfData, ok := result.Data.(*plugin.TerraformOutputData); ok && tfData.ChangedResult != "" {
			return tfData.ChangedResult
		}
	}

	return ""
}

// logArtifactOperation logs details about a planfile upload/download operation.
func logArtifactOperation(op, key, storeName, path string, info *schema.ConfigAndStacksInfo) {
	args := []any{"key", key, "store", storeName}
	if path != "" {
		args = append(args, "path", path)
	}
	args = append(args, "stack", info.Stack, "component", info.ComponentFromArg)
	log.Debug(op+" planfile", args...)
}
