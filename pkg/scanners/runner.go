package scanners

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
)

const (
	executableBits os.FileMode = 0o111
	logKeyScanner              = "scanner"
)

func Run(ctx context.Context, scan *Context) (*Output, error) {
	defer perf.Track(nil, "scanners.Run")()

	if err := validate(scan); err != nil {
		return nil, err
	}

	tmpDir, outputFile, err := makeOutputDir()
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	prep, err := prepareSubprocess(scan, tmpDir, outputFile)
	if err != nil {
		return nil, err
	}

	runErr := runSubprocess(ctx, prep)
	updateContext(scan, outputFile, tmpDir, runErr)

	out := captureOutput(scan, outputFile)
	renderTerminal(scan, out)
	renderCISummary(scan, out)
	emitCIAnnotations(scan, out)
	publishCIResults(scan, out)

	if runErr != nil {
		return out, applyOnFailure(scan, runErr)
	}
	return out, nil
}

func validate(scan *Context) error {
	if scan == nil {
		return errUtils.ErrNilParam
	}
	if scan.Command == "" {
		return errUtils.Build(errUtils.ErrInvalidConfig).
			WithExplanation("scanner has no command configured").
			WithContext("scanner", scan.Name).
			Err()
	}
	return nil
}

func makeOutputDir() (tmpDir, outputFile string, err error) {
	tmpDir, err = os.MkdirTemp("", "atmos-scan-*")
	if err != nil {
		return "", "", fmt.Errorf("failed to create temp dir for scanner output: %w", err)
	}
	return tmpDir, filepath.Join(tmpDir, "output"), nil
}

type subprocessPrep struct {
	binary            string
	args              []string
	env               []string
	captureStdoutPath string
}

func prepareSubprocess(scan *Context, tmpDir, outputFile string) (*subprocessPrep, error) {
	envVars := BuildAtmosEnv(scan, outputFile, tmpDir)

	scannerEnv := make(map[string]string, len(scan.Env))
	for k, v := range scan.Env {
		scannerEnv[k] = expandEnvVars(v, envVars)
	}

	args := make([]string, 0, len(scan.Args))
	for _, a := range scan.Args {
		args = append(args, expandEnvVars(a, envVars))
	}

	log.Debug("Running scanner command", logKeyScanner, scan.Name, "command", scan.Command, "args", args)

	baseEnv := scan.BaseEnv
	if len(baseEnv) == 0 {
		baseEnv = os.Environ()
	}
	resolved, err := resolveBinaryOnPath(scan.Command, scan.ToolchainPATH, pathFromEnv(baseEnv), pathExtFromEnv(baseEnv), runtime.GOOS)
	if err != nil {
		return nil, errUtils.Build(errUtils.ErrCommandNotFound).
			WithCause(err).
			WithExplanationf("Scanner command %q is not on PATH", scan.Command).
			WithHintf("Declare it in dependencies.tools (e.g. `%s: \"<version>\"`) to auto-install before the scanner runs", scan.Command).
			WithContext("scanner", scan.Name).
			WithContext("command", scan.Command).
			Err()
	}

	captureStdoutPath := ""
	if scan.CaptureStdout {
		captureStdoutPath = outputFile
	}

	return &subprocessPrep{
		binary:            resolved,
		args:              args,
		env:               mergeEnv(prependToolchainPATH(baseEnv, scan.ToolchainPATH), envVars, scannerEnv),
		captureStdoutPath: captureStdoutPath,
	}, nil
}

func runSubprocess(ctx context.Context, p *subprocessPrep) error {
	cmd := exec.CommandContext(ctx, p.binary, p.args...) // #nosec G204 -- scanner command is configured by Atmos users.
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr
	cmd.Env = p.env

	if p.captureStdoutPath != "" {
		f, err := os.Create(p.captureStdoutPath) // #nosec G304 -- engine-controlled temp path.
		if err != nil {
			return fmt.Errorf("%w: scanner stdout capture file: %w", errUtils.ErrCreateFile, err)
		}
		defer f.Close()
		cmd.Stdout = f
	} else {
		cmd.Stdout = os.Stdout
	}

	return cmd.Run()
}

func updateContext(scan *Context, outputFile, tmpDir string, runErr error) {
	scan.OutputFile = outputFile
	scan.OutputDir = tmpDir
	scan.ExitCode = exitCodeFromErr(runErr)
	scan.CommandError = runErr
}

func captureOutput(scan *Context, outputFile string) *Output {
	out := &Output{}

	if data, readErr := os.ReadFile(outputFile); readErr == nil && len(data) > 0 {
		metadata := map[string]string{"kind": scan.Name}
		if scan.Info != nil {
			metadata["stack"] = scan.Info.Stack
			metadata["component"] = scan.Info.ComponentFromArg
		}
		out.Artifact = &Artifact{
			Name:     filepath.Base(outputFile),
			Body:     data,
			Format:   scan.Format,
			Metadata: metadata,
		}
	}

	if scan.ResultHandler != nil {
		summary, hErr := scan.ResultHandler(scan)
		if hErr != nil {
			log.Warn("Scanner ResultHandler failed", logKeyScanner, scan.Name, "error", hErr)
		}
		out.Summary = summary
	}
	return out
}

func renderTerminal(scan *Context, out *Output) {
	if out == nil {
		return
	}
	if out.Summary != nil && out.Summary.Body != "" {
		ui.Writeln("")
		ui.MarkdownMessage(out.Summary.Body)
		return
	}
	if out.Artifact != nil && scan.Format == FormatMarkdown {
		ui.Writeln("")
		ui.MarkdownMessage(string(out.Artifact.Body))
	}
}

func BuildAtmosEnv(scan *Context, outputFile, outputDir string) map[string]string {
	defer perf.Track(nil, "scanners.BuildAtmosEnv")()

	env := map[string]string{
		"ATMOS_COMPONENT_PATH": ComponentPath(scan),
		"ATMOS_OUTPUT_FILE":    outputFile,
		"ATMOS_OUTPUT_DIR":     outputDir,
	}
	if scan != nil && scan.Info != nil {
		env["ATMOS_STACK"] = scan.Info.Stack
		env["ATMOS_COMPONENT"] = scan.Info.ComponentFromArg
	}
	if planfile := planfileFor(scan); planfile != "" {
		env["ATMOS_PLANFILE"] = planfile
	}
	return env
}

func planfileFor(_ *Context) string {
	return ""
}

func expandEnvVars(s string, vars map[string]string) string {
	return os.Expand(s, func(key string) string {
		if v, ok := vars[key]; ok {
			return v
		}
		return os.Getenv(key) //nolint:forbidigo // intentional process-env passthrough for token expansion
	})
}

func mergeEnv(base []string, layers ...map[string]string) []string {
	out := append([]string(nil), base...)
	for _, layer := range layers {
		for k, v := range layer {
			out = append(out, k+"="+v)
		}
	}
	return out
}

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

func pathFromEnv(env []string) string {
	const prefix = "PATH="
	for _, e := range env {
		if strings.HasPrefix(e, prefix) {
			return e[len(prefix):]
		}
	}
	return os.Getenv("PATH") //nolint:forbidigo // fallback when caller does not provide PATH.
}

func pathExtFromEnv(env []string) string {
	const prefix = "PATHEXT="
	for _, e := range env {
		if strings.HasPrefix(e, prefix) {
			return e[len(prefix):]
		}
	}
	return ""
}

func resolveBinaryOnPath(name, toolchainPATH, processPATH, pathExt, goos string) (string, error) {
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
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	if goos == "windows" {
		return true
	}
	return info.Mode()&executableBits != 0
}

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

func applyOnFailure(scan *Context, runErr error) error {
	mode := scan.OnFailure
	if mode == "" {
		mode = OnFailureWarn
	}
	switch mode {
	case OnFailureFail:
		return fmt.Errorf("scanner %s failed: %w", scan.Name, runErr)
	case OnFailureIgnore:
		log.Debug("Scanner command failed (ignored per on_failure)", logKeyScanner, scan.Name, "error", runErr)
		return nil
	case OnFailureWarn:
		fallthrough
	default:
		log.Warn("Scanner command failed (warning per on_failure)", logKeyScanner, scan.Name, "error", runErr)
		return nil
	}
}
