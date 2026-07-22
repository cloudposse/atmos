package hooks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/component"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
)

// On-failure modes for the command engine.
const (
	OnFailureWarn   = "warn"
	OnFailureFail   = "fail"
	OnFailureIgnore = "ignore"
)

// Format constants understood by the engine for inline rendering.
const (
	FormatMarkdown = "markdown"
)

// Artifact metadata keys. Defined as constants so the same names are used
// here and (eventually) on the Pro upload DTO without drifting.
const (
	metadataKeyKind      = "kind"
	metadataKeyStack     = "stack"
	metadataKeyComponent = "component"
)

const nonBlockingSecuritySeverity = "0.0"

// executableBits is the standard "any execute" Unix permission mask.
// A binary on PATH must have at least one of owner/group/other execute
// bits set; we use this to skip non-runnable files (data files, READMEs,
// etc.) that may share a directory with a tool's actual binary.
const executableBits os.FileMode = 0o111

// logKeyKind is the structured-log key for the hook kind name. Used in
// every log line so log search by `kind=trivy` etc. works consistently.
const logKeyKind = "kind"

// shouldPropagateHookLogGroupSentinel is a test seam for subprocess environment
// construction.
//
//nolint:gochecknoglobals // test seam for CI log grouping.
var shouldPropagateHookLogGroupSentinel = ci.ShouldPropagateLogGroupSentinel

// init registers the generic `command` kind so users can invoke arbitrary
// toolchain-resolved binaries without writing Go.
func init() {
	if err := RegisterKind(&Kind{
		Name:      "command",
		Engine:    &CommandEngine{},
		OnFailure: OnFailureWarn,
	}); err != nil {
		panic("failed to register built-in command kind: " + err.Error())
	}
}

// CommandEngine runs a binary (resolved via PATH — set up by the toolchain
// or any other install mechanism) against the component being orchestrated.
// It exposes standard ATMOS_* env vars to the subprocess and captures
// structured output via ATMOS_OUTPUT_FILE.
//
// Tool stdout/stderr stream straight through to the user's terminal so
// progress and warnings appear in real time (same UX as Terraform output).
// Structured output goes through the side-channel file, read by the kind's
// ResultHandler (if any) and packaged as an Artifact.
type CommandEngine struct{}

// Run satisfies Engine. It's an orchestrator — the actual work lives in
// named helpers (validateCtx / prepareSubprocess / runSubprocess /
// captureOutput / renderTerminal) so each step has a single
// responsibility and the orchestrator stays a flat linear pipeline.
func (e *CommandEngine) Run(ctx *ExecContext) (*Output, error) {
	defer perf.Track(nil, "hooks.CommandEngine.Run")()

	if err := validateCtx(ctx); err != nil {
		return nil, err
	}

	tmpDir, outputFile, err := makeOutputDir()
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	prep, err := prepareSubprocess(ctx, tmpDir, outputFile)
	if err != nil {
		return nil, err
	}

	endLogGroup := startHookLogGroup(ctx)
	defer endLogGroup()

	runErr := runSubprocess(prep)
	updateExecCtx(ctx, outputFile, tmpDir, runErr)

	out := captureOutput(ctx, outputFile)
	renderTerminal(ctx, out)
	renderCISummary(ctx, out)
	emitCIAnnotations(ctx, out)
	publishCIResults(ctx, out)

	if runErr != nil {
		return out, applyOnFailure(ctx, runErr)
	}
	return out, nil
}

// validateCtx checks the engine's preconditions on the ExecContext.
// Returns nil on success; a typed error otherwise.
func validateCtx(ctx *ExecContext) error {
	if ctx == nil || ctx.Hook == nil || ctx.Kind == nil {
		return errUtils.ErrNilParam
	}
	if ctx.Hook.Command == "" {
		return errUtils.Build(errUtils.ErrInvalidConfig).
			WithExplanation("hook kind has no command configured").
			WithContext("hook_kind", ctx.Hook.Kind).
			Err()
	}
	return nil
}

// makeOutputDir creates the per-invocation temp directory plus the
// path to the structured-output file inside it. Caller is responsible
// for `os.RemoveAll(tmpDir)`.
func makeOutputDir() (tmpDir, outputFile string, err error) {
	tmpDir, err = os.MkdirTemp("", "atmos-hook-*")
	if err != nil {
		return "", "", fmt.Errorf("%w: hook output directory: %w", errUtils.ErrCreateDirectory, err)
	}
	return tmpDir, filepath.Join(tmpDir, "output"), nil
}

// subprocessPrep is the fully-prepared invocation: rendered args, env,
// resolved binary path. Built once by prepareSubprocess, consumed by
// runSubprocess.
type subprocessPrep struct {
	binary string
	args   []string
	env    []string
	// captureStdoutPath, when non-empty, is the file the subprocess's stdout
	// is redirected into (instead of the terminal). Set for kinds with
	// CaptureStdout — tools that emit structured output to stdout and have no
	// file-output flag (tflint). Points at the same file ATMOS_OUTPUT_FILE does.
	captureStdoutPath string
}

// prepareSubprocess renders args / env (with $ATMOS_* expansion) and
// resolves the binary on the toolchain-augmented PATH. Returns the
// prep struct ready for runSubprocess to exec.
func prepareSubprocess(ctx *ExecContext, tmpDir, outputFile string) (*subprocessPrep, error) {
	envVars := buildAtmosEnv(ctx, outputFile, tmpDir)

	hookEnv := make(map[string]string, len(ctx.Hook.Env))
	for k, v := range ctx.Hook.Env {
		hookEnv[k] = expandEnvVars(v, envVars)
	}

	args := make([]string, 0, len(ctx.Hook.Args))
	for _, a := range ctx.Hook.Args {
		args = append(args, expandEnvVars(a, envVars))
	}

	log.Debug(
		"Running hook command",
		logKeyKind, ctx.Hook.Kind,
		"command", ctx.Hook.Command,
		"args", args,
	)

	// exec.Command resolves the binary via the PROCESS PATH (os.Environ()),
	// not via cmd.Env. So if we want the toolchain-installed pinned version
	// to win, we have to do the lookup ourselves against the augmented PATH
	// and pass the absolute path.
	resolved, err := resolveBinaryOnPath(ctx.Hook.Command, ctx.ToolchainPATH)
	if err != nil {
		// resolveBinaryOnPath's own error already wraps ErrCommandNotFound (needed so its
		// direct callers/tests can errors.Is() against it standalone). Build the user-facing
		// message from scratch here rather than via WithCause(err): that would double the
		// sentinel's own text into the final message ("command not found: command not found:
		// <name>"), since err already wraps the same sentinel this Build() call starts from,
		// and the explanation/hint below already say everything err's text would add.
		return nil, errUtils.Build(errUtils.ErrCommandNotFound).
			WithExplanationf("Hook command %q is not on PATH", ctx.Hook.Command).
			WithHintf("Declare it in dependencies.tools (e.g. `%s: \"<version>\"`) to auto-install before the hook fires", ctx.Hook.Command).
			WithContext("hook_kind", ctx.Hook.Kind).
			WithContext("command", ctx.Hook.Command).
			Err()
	}

	captureStdoutPath := ""
	if ctx.Kind != nil && ctx.Kind.CaptureStdout {
		// Redirect stdout into the same file ATMOS_OUTPUT_FILE points at, so
		// the kind's ResultHandler reads it via sarif.DefaultOutputFile — no
		// difference from a tool that writes the file itself (trivy/checkov).
		captureStdoutPath = outputFile
	}

	env := mergeEnv(prependToolchainPATH(os.Environ(), ctx.ToolchainPATH), envVars, hookEnv)
	if shouldPropagateHookLogGroupSentinel(ctx.AtmosConfig, ci.DimensionPhase) {
		env = append(env, ci.LogGroupSentinelEnv())
	}

	return &subprocessPrep{
		binary:            resolved,
		args:              args,
		env:               env,
		captureStdoutPath: captureStdoutPath,
	}, nil
}

// runSubprocess executes the prepared command, wiring stdin/stdout/stderr
// to the host process so the user sees tool output in real time. When the kind
// opts into stdout capture (p.captureStdoutPath set), stdout is redirected into
// that file instead of the terminal — for tools that emit structured output
// (SARIF) to stdout with no file-output flag. Stderr still streams so the
// tool's diagnostics/errors remain visible.
func runSubprocess(p *subprocessPrep) error {
	cmd := exec.Command(p.binary, p.args...) // #nosec G204 -- intentional: this is the whole point of a hook
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr
	cmd.Env = p.env

	if p.captureStdoutPath != "" {
		f, err := os.Create(p.captureStdoutPath) // #nosec G304 -- engine-controlled temp path (makeOutputDir)
		if err != nil {
			return fmt.Errorf("%w: hook stdout capture file: %w", errUtils.ErrCreateFile, err)
		}
		defer f.Close()
		cmd.Stdout = f
	} else {
		cmd.Stdout = os.Stdout
	}

	return cmd.Run()
}

// updateExecCtx writes subprocess result state back into the
// ExecContext so the kind's ResultHandler (called later) has access to
// the structured-output file path, exit code, and error.
func updateExecCtx(ctx *ExecContext, outputFile, tmpDir string, runErr error) {
	ctx.OutputFile = outputFile
	ctx.OutputDir = tmpDir
	ctx.ExitCode = exitCodeFromErr(runErr)
	ctx.CommandError = runErr
}

// captureOutput reads the structured-output file (if any) into an
// Artifact and runs the kind's ResultHandler (if any) to build a
// Summary. Both halves are optional; nil for either is fine.
func captureOutput(ctx *ExecContext, outputFile string) *Output {
	out := &Output{}

	if data, readErr := os.ReadFile(outputFile); readErr == nil && len(data) > 0 {
		metadata := map[string]string{
			metadataKeyKind: ctx.Hook.Kind,
		}
		if ctx.Info != nil {
			metadata[metadataKeyStack] = ctx.Info.Stack
			metadata[metadataKeyComponent] = ctx.Info.ComponentFromArg
		}
		out.Artifact = &Artifact{
			Name:     filepath.Base(outputFile),
			Body:     data,
			Format:   ctx.Hook.Format,
			Metadata: metadata,
		}
	}

	if handler := resolveResultHandler(ctx); handler != nil {
		summary, hErr := handler(ctx)
		if hErr != nil {
			log.Warn(
				"Hook ResultHandler failed",
				logKeyKind, ctx.Hook.Kind,
				"error", hErr,
			)
		}
		out.Summary = summary
	}
	return out
}

// renderTerminal emits the hook's user-facing output: a styled
// markdown block via ui.MarkdownMessage when there's a summary body or
// a markdown-formatted artifact. The leading blank line visually
// separates the rendered block from preceding output (terraform plan,
// the hook log line, the tool's own stdout). MarkdownMessage's renderer
// (glamour) trims leading whitespace, so we emit the blank line as a
// separate UI write rather than relying on a `\n` prefix in the body.
func renderTerminal(ctx *ExecContext, out *Output) {
	if out == nil {
		return
	}
	if out.Summary != nil && out.Summary.Body != "" {
		ui.Writeln("")
		ui.MarkdownMessage(out.Summary.Body)
		return
	}
	if out.Artifact != nil && ctx.Hook.Format == FormatMarkdown {
		ui.Writeln("")
		ui.MarkdownMessage(string(out.Artifact.Body))
	}
}

func startHookLogGroup(ctx *ExecContext) func() {
	if !ciEnabled(ctx) {
		return func() {}
	}
	return ci.StartLogGroup(hookLogGroupTitle(ctx))
}

func hookLogGroupTitle(ctx *ExecContext) string {
	if ctx == nil {
		return "hook"
	}

	label := strings.TrimSpace(ctx.HookName)
	kind := ""
	command := ""
	if ctx.Hook != nil {
		kind = strings.TrimSpace(ctx.Hook.Kind)
		command = strings.TrimSpace(ctx.Hook.Command)
	}
	if label == "" {
		label = kind
	}
	if label == "" {
		label = command
	}
	if label == "" {
		label = "hook"
	}

	if kind != "" && kind != label {
		label = fmt.Sprintf("%s (%s)", label, kind)
	}
	if ctx.Event != "" {
		return fmt.Sprintf("hook %s - %s", label, ctx.Event)
	}
	return fmt.Sprintf("hook %s", label)
}

// BuildAtmosEnv exposes the ATMOS_* env-var map builder to engines outside the
// CommandEngine (e.g. the step bridge in step_engine.go) so they expose the
// same standard variables (ATMOS_STACK, ATMOS_COMPONENT, ATMOS_COMPONENT_PATH,
// …) to the work they run.
func BuildAtmosEnv(ctx *ExecContext, outputFile, outputDir string) map[string]string {
	defer perf.Track(nil, "hooks.BuildAtmosEnv")()

	return buildAtmosEnv(ctx, outputFile, outputDir)
}

// renderCISummary appends the hook's markdown summary to the active CI
// provider's job step summary (e.g. GitHub Actions' $GITHUB_STEP_SUMMARY) so
// scanner findings are visible in the pipeline run, not just the log stream.
// It is a no-op outside CI. Writing to the step summary is best-effort: a
// failure here must never fail the hook (the findings already rendered to the
// terminal), so the error is logged at debug and swallowed.
func renderCISummary(ctx *ExecContext, out *Output) {
	if out == nil || out.Summary == nil || out.Summary.Body == "" {
		return
	}
	if !ciSummaryEnabled(ctx) {
		return
	}
	// The step summary is append-only and shared: a prior step or hook may
	// have left content without a trailing blank line. Each summary body
	// starts with a `## <tool>` heading, and GitHub-flavored Markdown only
	// renders an ATX heading as a heading when a blank line precedes it. So
	// prefix a newline to guarantee separation — every appended summary lands
	// as its own cleanly-delimited chapter. Mirrors renderTerminal's leading
	// ui.Writeln("").
	if err := ci.WriteStepSummary("\n" + out.Summary.Body); err != nil {
		log.Debug("Failed to write hook summary to CI step summary", "error", err)
	}
}

// emitCIAnnotations renders the hook's findings as inline CI annotations
// (GitHub: `::error`/`::warning` on the PR diff) when `ci.annotations` is
// enabled. Best-effort: the findings already rendered to the terminal/summary,
// so a failure here logs at debug and never fails the hook. The active provider
// decides whether/how to render — outside CI it no-ops.
func emitCIAnnotations(ctx *ExecContext, out *Output) {
	if out == nil || out.Summary == nil || len(out.Summary.Findings) == 0 {
		return
	}
	if !ciAnnotationsEnabled(ctx) {
		return
	}
	annotations := make([]ci.Annotation, 0, len(out.Summary.Findings))
	for _, f := range out.Summary.Findings {
		annotations = append(annotations, ci.Annotation{
			Path:      f.Path,
			StartLine: f.Line,
			Level:     annotationLevelForHook(ctx, f.Severity),
			Title:     f.RuleID,
			Message:   f.Message,
		})
	}
	if err := ci.Annotate(annotations); err != nil {
		log.Debug("Failed to emit CI annotations", logKeyKind, ctx.Hook.Kind, "error", err)
	}
}

// publishCIResults uploads the hook's raw SARIF to the CI provider's
// security-findings store (GitHub Code Scanning) when `ci.results` is enabled.
// The analysis category is auto-derived from the scan target. Best-effort:
// a failure logs at debug and never fails the hook; outside CI it no-ops.
func publishCIResults(ctx *ExecContext, out *Output) {
	if out == nil || out.Summary == nil || len(out.Summary.SARIF) == 0 {
		return
	}
	if !ciResultsEnabled(ctx) {
		return
	}
	body := out.Summary.SARIF
	if reportsAsWarning(ctx) {
		body = normalizeSARIFLevels(body, "warning")
	}
	report := ci.SARIFReport{Body: body, Category: deriveSARIFCategory(ctx, body)}
	if err := ci.ReportSARIF(context.Background(), report); err != nil {
		log.Debug("Failed to publish SARIF results to CI provider", logKeyKind, ctx.Hook.Kind, "error", err)
	}
}

func annotationLevelForHook(ctx *ExecContext, severity string) ci.AnnotationLevel {
	if reportsAsWarning(ctx) {
		return ci.AnnotationWarning
	}
	return annotationLevelForSeverity(severity)
}

// annotationLevelForSeverity maps a normalized scanner severity to the CI
// annotation level. Critical/High surface as errors; everything else as
// warnings (matching the scanner default of not blocking the plan).
func annotationLevelForSeverity(severity string) ci.AnnotationLevel {
	switch severity {
	case "critical", "high":
		return ci.AnnotationError
	default:
		return ci.AnnotationWarning
	}
}

func reportsAsWarning(ctx *ExecContext) bool {
	return ctx != nil && ctx.Hook != nil && ctx.Hook.OnFailure != OnFailureFail
}

func normalizeSARIFLevels(sarif []byte, level string) []byte {
	if len(sarif) == 0 || level == "" {
		return sarif
	}
	var doc map[string]any
	if err := json.Unmarshal(sarif, &doc); err != nil {
		return sarif
	}
	runs, ok := doc["runs"].([]any)
	if !ok {
		return sarif
	}
	for _, rawRun := range runs {
		run, ok := rawRun.(map[string]any)
		if !ok {
			continue
		}
		normalizeRunResultLevels(run, level)
		normalizeRunRuleLevels(run, level)
	}
	out, err := json.Marshal(doc)
	if err != nil {
		return sarif
	}
	return out
}

func normalizeRunResultLevels(run map[string]any, level string) {
	results, ok := run["results"].([]any)
	if !ok {
		return
	}
	for _, rawResult := range results {
		result, ok := rawResult.(map[string]any)
		if ok {
			result["level"] = level
			normalizeSecuritySeverity(result)
		}
	}
}

func normalizeRunRuleLevels(run map[string]any, level string) {
	tool, ok := run["tool"].(map[string]any)
	if !ok {
		return
	}
	driver, ok := tool["driver"].(map[string]any)
	if !ok {
		return
	}
	rules, ok := driver["rules"].([]any)
	if !ok {
		return
	}
	for _, rawRule := range rules {
		rule, ok := rawRule.(map[string]any)
		if !ok {
			continue
		}
		defaultConfig, ok := rule["defaultConfiguration"].(map[string]any)
		if !ok {
			defaultConfig = map[string]any{}
			rule["defaultConfiguration"] = defaultConfig
		}
		defaultConfig["level"] = level
		normalizeSecuritySeverity(rule)
	}
}

func normalizeSecuritySeverity(item map[string]any) {
	properties, ok := item["properties"].(map[string]any)
	if !ok {
		return
	}
	if _, ok := properties["security-severity"]; ok {
		properties["security-severity"] = nonBlockingSecuritySeverity
	}
}

// deriveSARIFCategory builds the Code Scanning analysis category from the
// scanner tool identity. The same component source can appear in many stacks,
// so stack/component are intentionally not part of the category.
func deriveSARIFCategory(ctx *ExecContext, sarif []byte) string {
	if toolName := firstSARIFToolName(sarif); toolName != "" {
		return toolName
	}
	if ctx == nil || ctx.Hook == nil {
		return ""
	}
	if ctx.Hook.Kind == "command" && ctx.Hook.Command != "" {
		return ctx.Hook.Command
	}
	if ctx.Hook.Kind != "" {
		return ctx.Hook.Kind
	}
	return ctx.Hook.Command
}

func firstSARIFToolName(sarif []byte) string {
	if len(sarif) == 0 {
		return ""
	}
	var doc map[string]any
	if err := json.Unmarshal(sarif, &doc); err != nil {
		return ""
	}
	runs, ok := doc["runs"].([]any)
	if !ok {
		return ""
	}
	for _, rawRun := range runs {
		run, ok := rawRun.(map[string]any)
		if !ok {
			continue
		}
		tool, ok := run["tool"].(map[string]any)
		if !ok {
			continue
		}
		driver, ok := tool["driver"].(map[string]any)
		if !ok {
			continue
		}
		name, ok := driver["name"].(string)
		if ok && strings.TrimSpace(name) != "" {
			return strings.TrimSpace(name)
		}
	}
	return ""
}

// ciEnabled reports whether CI integration is enabled in config — the master
// switch all CI reporting outputs (summary/annotations/results) require.
func ciEnabled(ctx *ExecContext) bool {
	return ctx != nil && ci.Enabled(ctx.AtmosConfig)
}

// ciSummaryEnabled reports whether the job step summary should be written.
// Defaults to true (nil) when ci.enabled, matching ci.summary's default.
func ciSummaryEnabled(ctx *ExecContext) bool {
	if !ciEnabled(ctx) {
		return false
	}
	e := ctx.AtmosConfig.CI.Summary.Enabled
	return e == nil || *e
}

// ciAnnotationsEnabled reports whether inline annotations should be emitted.
// Defaults to true (nil) when ci.enabled.
func ciAnnotationsEnabled(ctx *ExecContext) bool {
	return ctx != nil && ci.AnnotationsEnabled(ctx.AtmosConfig)
}

// ciResultsEnabled reports whether SARIF should be uploaded to the provider's
// findings store. Defaults to false (nil) — opt-in, since it has side effects
// and extra requirements (GitHub Advanced Security, security-events: write).
func ciResultsEnabled(ctx *ExecContext) bool {
	return ctx != nil && ci.ResultsEnabled(ctx.AtmosConfig)
}

// buildAtmosEnv builds the ATMOS_* env-var map for the subprocess.
func buildAtmosEnv(ctx *ExecContext, outputFile, outputDir string) map[string]string {
	componentPath := componentPathFor(ctx)
	env := map[string]string{
		"ATMOS_COMPONENT_PATH": componentPath,
		"ATMOS_OUTPUT_FILE":    outputFile,
		"ATMOS_OUTPUT_DIR":     outputDir,
	}
	if ctx.Info != nil {
		env["ATMOS_STACK"] = ctx.Info.Stack
		env["ATMOS_COMPONENT"] = ctx.Info.ComponentFromArg
	}
	if planfile := planfileFor(ctx); planfile != "" {
		env["ATMOS_PLANFILE"] = planfile
	}
	// Lifecycle outcome so hooks can report what happened (e.g. a `say` or
	// `http` step on `when: failure`). Status mirrors the `{{ .status }}`
	// template key; component/stack are already exported above.
	if ctx.Outcome.Status != "" {
		env["ATMOS_HOOK_STATUS"] = string(ctx.Outcome.Status)
		env["ATMOS_HOOK_EXIT_CODE"] = strconv.Itoa(ctx.Outcome.ExitCode)
	}
	if ctx.Outcome.Err != nil {
		env["ATMOS_HOOK_ERROR"] = ctx.Outcome.Err.Error()
	}
	return env
}

// componentPathFor resolves the on-disk path the tool should scan. It is
// the SAME directory Terraform/OpenTofu runs in — this is what the user
// expects when they configure a hook against a component, and it keeps
// scanners aligned with the actual workdir (which may be a provisioned
// copy, not the in-repo component path) when the workdir feature is
// enabled.
//
// Resolution order:
//
//  1. If the workdir feature resolves an existing directory for this
//     component, use that.
//  2. Otherwise fall back to TerraformDirAbsolutePath /
//     ComponentFolderPrefix / FinalComponent (the legacy in-repo path).
//  3. As a last resort (mostly tests), the process working directory.
//
// Errors from the workdir resolver are non-fatal: hooks should still run
// even if workdir resolution fails for a reason unrelated to the hook.
func componentPathFor(ctx *ExecContext) string {
	if ctx == nil || ctx.AtmosConfig == nil || ctx.Info == nil {
		wd, _ := os.Getwd()
		return wd
	}

	// Prefer the provisioned workdir if one resolves and exists on disk.
	if path, exists, err := component.BuildAndResolveWorkdirPath(ctx.AtmosConfig, ctx.Info, cfg.TerraformComponentType); err == nil && exists && path != "" {
		return path
	}

	base := ctx.AtmosConfig.TerraformDirAbsolutePath
	if base == "" {
		wd, _ := os.Getwd()
		return wd
	}
	finalComponent := ctx.Info.FinalComponent
	if finalComponent == "" {
		finalComponent = ctx.Info.ComponentFromArg
	}
	return filepath.Join(base, ctx.Info.ComponentFolderPrefix, finalComponent)
}

// planfileFor returns the planfile path for after-plan events when known.
// Wired more thoroughly in a later commit once the terraform engine threads
// planfile metadata into hook execution.
func planfileFor(_ *ExecContext) string {
	return ""
}

// resolveBinaryOnPath finds the absolute path of `name` looking first at
// the supplied toolchainPATH (so pinned versions win) and then falling
// back to the process PATH. Mirrors exec.LookPath but uses our augmented
// PATH instead of the process environment.
//
// Returns the resolved path on success. Absolute or relative paths
// containing a separator are accepted only when they point at an
// executable file.
func resolveBinaryOnPath(name, toolchainPATH string) (string, error) {
	return resolveBinaryOnPathWithEnv(
		name,
		toolchainPATH,
		os.Getenv("PATH"),    //nolint:forbidigo // PATH is a shell-managed env var, not viper-bound config
		os.Getenv("PATHEXT"), //nolint:forbidigo // PATHEXT is Windows shell-managed command lookup config
		runtime.GOOS,
	)
}

func resolveBinaryOnPathWithEnv(name, toolchainPATH, processPATH, pathExt, goos string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("%w: empty command name", errUtils.ErrCommandNotFound)
	}
	if filepath.IsAbs(name) || strings.ContainsAny(name, "/\\") {
		if isExecutableFile(name, goos) {
			return name, nil
		}
		return "", fmt.Errorf("%w: %s", errUtils.ErrCommandNotFound, name)
	}

	searchPath := combineSearchPath(toolchainPATH, processPATH)
	if path, ok := searchPathForBinary(name, searchPath, pathExt, goos); ok {
		return path, nil
	}

	return "", fmt.Errorf("%w: %s", errUtils.ErrCommandNotFound, name)
}

func combineSearchPath(toolchainPATH, processPATH string) string {
	if toolchainPATH != "" {
		if processPATH == "" {
			return toolchainPATH
		}
		return toolchainPATH + string(os.PathListSeparator) + processPATH
	}
	return processPATH
}

// searchPathForBinary walks the PATH-style string and returns the first
// directory entry that contains an executable file matching name.
func searchPathForBinary(name, searchPath, pathExt, goos string) (string, bool) {
	for _, dir := range filepath.SplitList(searchPath) {
		if dir == "" {
			continue
		}
		for _, candidateName := range candidateBinaryNames(name, pathExt, goos) {
			candidate := filepath.Join(dir, candidateName)
			if isExecutableFile(candidate, goos) {
				return candidate, true
			}
		}
	}
	return "", false
}

func candidateBinaryNames(name, pathExt, goos string) []string {
	if goos != "windows" || filepath.Ext(name) != "" {
		return []string{name}
	}

	exts := strings.Split(pathExt, ";")
	if pathExt == "" {
		exts = []string{".com", ".exe", ".bat", ".cmd"}
	}

	names := []string{name}
	seen := map[string]struct{}{strings.ToLower(name): {}}
	for _, ext := range exts {
		ext = strings.TrimSpace(ext)
		if ext == "" {
			continue
		}
		if !strings.HasPrefix(ext, ".") {
			ext = "." + ext
		}
		candidate := name + ext
		key := strings.ToLower(candidate)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		names = append(names, candidate)
	}
	return names
}

func isExecutableFile(path, goos string) bool {
	info, err := os.Stat(path) //nolint:gosec // path built from PATH entries + binary name — both operator-controlled
	if err != nil || info.IsDir() {
		return false
	}
	if goos == "windows" {
		return true
	}
	return info.Mode()&executableBits != 0
}

// prependToolchainPATH puts the toolchain bin dirs at the front of PATH in
// a copy of the given environment, so installed pinned versions resolve
// before anything else on the operator's PATH. If toolchainPATH is empty
// (no dependencies declared), env is returned unchanged.
func prependToolchainPATH(env []string, toolchainPATH string) []string {
	if toolchainPATH == "" {
		return env
	}
	out := make([]string, 0, len(env))
	patched := false
	const prefix = "PATH="
	for _, e := range env {
		if !patched && len(e) > len(prefix) && e[:len(prefix)] == prefix {
			out = append(out, prefix+toolchainPATH+string(os.PathListSeparator)+e[len(prefix):])
			patched = true
			continue
		}
		out = append(out, e)
	}
	if !patched {
		out = append(out, prefix+toolchainPATH)
	}
	return out
}

// expandEnvVars expands $VAR / ${VAR} tokens using vars first, then OS env.
func expandEnvVars(s string, vars map[string]string) string {
	return os.Expand(s, func(key string) string {
		if v, ok := vars[key]; ok {
			return v
		}
		// Reading the host environment for token expansion is the whole
		// point of this helper — there's no viper-managed surface that
		// could substitute here. Localized nolint matches pkg/dependencies'
		// convention.
		return os.Getenv(key) //nolint:forbidigo // intentional process-env passthrough for token expansion
	})
}

// mergeEnv combines the base environment with hook-supplied env additions.
// Later maps override earlier ones.
func mergeEnv(base []string, layers ...map[string]string) []string {
	out := append([]string(nil), base...)
	for _, layer := range layers {
		for k, v := range layer {
			out = append(out, k+"="+v)
		}
	}
	return out
}

// exitCodeFromErr extracts the exit code from a subprocess error.
// Returns 0 if err is nil, the actual exit code on *exec.ExitError, or 1
// for any other error.
func exitCodeFromErr(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return 1
}

// ApplyOnFailure applies the hook's effective on_failure mode to runErr:
// "warn" logs and swallows, "ignore" swallows silently, "fail" propagates.
// Exported so engines living outside this package (pkg/hooks/kinds/*) enforce
// the same on_failure semantics as the built-in command engine.
func ApplyOnFailure(ctx *ExecContext, runErr error) error {
	return applyOnFailure(ctx, runErr)
}

// applyOnFailure returns nil or an error depending on the configured failure
// mode. "warn" (default) logs and returns nil; "fail" propagates the error;
// "ignore" drops it entirely.
func applyOnFailure(ctx *ExecContext, runErr error) error {
	mode := ctx.Hook.OnFailure
	if mode == "" && ctx.Kind != nil {
		mode = ctx.Kind.OnFailure
	}
	if mode == "" {
		mode = OnFailureWarn
	}
	switch mode {
	case OnFailureFail:
		return fmt.Errorf("hook (kind %s) failed: %w", ctx.Hook.Kind, runErr)
	case OnFailureIgnore:
		log.Debug(
			"Hook command failed (ignored per on_failure)",
			logKeyKind, ctx.Hook.Kind,
			"error", runErr,
		)
		return nil
	case OnFailureWarn:
		fallthrough
	default:
		log.Warn(
			"Hook command failed (warning per on_failure)",
			logKeyKind, ctx.Hook.Kind,
			"error", runErr,
		)
		return nil
	}
}
