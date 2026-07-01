package step

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/exec"
	"github.com/cloudposse/atmos/pkg/filesystem"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// RequireHandler asserts that required CLI tools are on PATH and that required
// files and directories exist. It is a read-only preconditions gate: it never
// mutates PATH or the environment, which keeps it safe to run alongside other
// steps that may write to the shared step environment.
type RequireHandler struct {
	BaseHandler
	// exec resolves executables on PATH; injectable for tests.
	exec exec.CommandExecutor
	// fs checks file and directory existence; injectable for tests.
	fs filesystem.FileSystem
}

func init() {
	Register(newRequireHandler())
}

// newRequireHandler builds a RequireHandler backed by the real OS executor and
// filesystem. Tests construct the handler directly with mocks instead.
func newRequireHandler() *RequireHandler {
	return &RequireHandler{
		BaseHandler: NewBaseHandler("require", CategoryCommand, false, "assert"),
		exec:        exec.Default(),
		fs:          filesystem.NewOSFileSystem(),
	}
}

// Validate checks that the step declares at least one requirement.
func (h *RequireHandler) Validate(step *schema.WorkflowStep) error {
	defer perf.Track(nil, "step.RequireHandler.Validate")()

	if len(step.Tools) == 0 && len(step.Files) == 0 && len(step.Dirs) == 0 {
		return errUtils.Build(errUtils.ErrRequireStepEmpty).
			WithContext("step", step.Name).
			WithContext("type", step.Type).
			WithHint("Add a `tools`, `files`, or `dirs` list to the require step").
			Err()
	}
	return nil
}

// Execute verifies every declared requirement and fails with an aggregated,
// hinted error listing everything that is missing.
func (h *RequireHandler) Execute(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (*StepResult, error) {
	defer perf.Track(nil, "step.RequireHandler.Execute")()

	tools, err := resolveRequireList(step, vars, "tools", step.Tools)
	if err != nil {
		return nil, err
	}
	files, err := resolveRequireList(step, vars, "files", step.Files)
	if err != nil {
		return nil, err
	}
	dirs, err := resolveRequireList(step, vars, "dirs", step.Dirs)
	if err != nil {
		return nil, err
	}

	// Snapshot the effective PATH once. The toolchain-augmented PATH is exposed
	// through the executor's environment (vars.Env), not the in-process PATH, so
	// read it from there and fall back to the process PATH. Reading once keeps
	// this step's exposure to the shared env map to a single access.
	pathValue := vars.Env["PATH"]
	if pathValue == "" {
		pathValue = os.Getenv("PATH") //nolint:forbidigo // Fallback when the executor did not seed PATH.
	}
	pathDirs := filepath.SplitList(pathValue)

	missingTools := h.findMissingTools(tools, pathDirs)
	missingFiles := h.findMissingFiles(files)
	missingDirs := h.findMissingDirs(dirs)

	if len(missingTools) == 0 && len(missingFiles) == 0 && len(missingDirs) == 0 {
		summary := fmt.Sprintf("all requirements satisfied (%d tool(s), %d file(s), %d dir(s))", len(tools), len(files), len(dirs))
		return NewStepResult(summary).
			WithMetadata("tools", len(tools)).
			WithMetadata("files", len(files)).
			WithMetadata("dirs", len(dirs)), nil
	}

	return nil, h.buildMissingError(step, vars, missingTools, missingFiles, missingDirs)
}

// findMissingTools returns the declared tools that are not executable on the
// supplied PATH directories.
func (h *RequireHandler) findMissingTools(tools, pathDirs []string) []string {
	var missing []string
	for _, tool := range tools {
		if tool == "" {
			continue
		}
		if !h.toolAvailable(tool, pathDirs) {
			missing = append(missing, tool)
		}
	}
	return missing
}

// toolAvailable reports whether tool resolves to an executable. A tool that
// includes a path component is checked directly; a bare name is searched across
// the provided PATH directories. Resolving a path with a separator through
// LookPath validates the executable bit (and Windows PATHEXT), so this stays
// cross-platform.
func (h *RequireHandler) toolAvailable(tool string, pathDirs []string) bool {
	if filepath.Base(tool) != tool || filepath.IsAbs(tool) {
		_, err := h.exec.LookPath(tool)
		return err == nil
	}
	for _, dir := range pathDirs {
		if dir == "" {
			continue
		}
		if _, err := h.exec.LookPath(filepath.Join(dir, tool)); err == nil {
			return true
		}
	}
	return false
}

// findMissingFiles returns the declared paths that do not exist.
func (h *RequireHandler) findMissingFiles(files []string) []string {
	var missing []string
	for _, file := range files {
		if file == "" {
			continue
		}
		if _, err := h.fs.Stat(file); err != nil {
			missing = append(missing, file)
		}
	}
	return missing
}

// findMissingDirs returns the declared directories that do not exist or are not
// directories.
func (h *RequireHandler) findMissingDirs(dirs []string) []string {
	var missing []string
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		info, err := h.fs.Stat(dir)
		if err != nil || !info.IsDir() {
			missing = append(missing, dir)
		}
	}
	return missing
}

// buildMissingError assembles a single error listing every missing requirement,
// with one self-contained, lightbulb-rendered hint per requirement kind plus the
// user's optional custom hint.
func (h *RequireHandler) buildMissingError(step *schema.WorkflowStep, vars *Variables, missingTools, missingFiles, missingDirs []string) error {
	builder := errUtils.Build(errUtils.ErrRequirementsNotMet).
		WithContext("step", step.Name)

	total := len(missingTools) + len(missingFiles) + len(missingDirs)
	builder = builder.WithExplanationf("Step '%s' is missing %d required tool(s)/path(s)", step.Name, total)

	if len(missingTools) > 0 {
		builder = builder.
			WithContext("missing_tools", strings.Join(missingTools, ", ")).
			WithHintf("Install missing tool(s) [%s] with `atmos toolchain install <tool>`, or add them to `dependencies.tools`", strings.Join(missingTools, ", "))
	}
	if len(missingFiles) > 0 {
		builder = builder.
			WithContext("missing_files", strings.Join(missingFiles, ", ")).
			WithHintf("Create or fix the missing file(s): %s", strings.Join(missingFiles, ", "))
	}
	if len(missingDirs) > 0 {
		builder = builder.
			WithContext("missing_dirs", strings.Join(missingDirs, ", ")).
			WithHintf("Create the missing director(ies): %s", strings.Join(missingDirs, ", "))
	}

	// Append the user's custom remediation note as its own self-contained hint.
	if step.Hint != "" {
		if resolved, err := vars.Resolve(step.Hint); err == nil {
			builder = builder.WithHint(resolved)
		} else {
			builder = builder.WithHint(step.Hint)
		}
	}

	return builder.WithExitCode(1).Err()
}

// resolveRequireList expands Go templates in each entry of a require list.
func resolveRequireList(step *schema.WorkflowStep, vars *Variables, field string, items []string) ([]string, error) {
	if len(items) == 0 {
		return nil, nil
	}
	resolved := make([]string, 0, len(items))
	for _, item := range items {
		value, err := vars.Resolve(item)
		if err != nil {
			return nil, errUtils.Build(errUtils.ErrTemplateEvaluation).
				WithCause(err).
				WithContext("step", step.Name).
				WithContext("field", field).
				Err()
		}
		resolved = append(resolved, value)
	}
	return resolved, nil
}
