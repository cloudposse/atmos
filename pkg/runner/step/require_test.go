package step

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	cockroachErrors "github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	execpkg "github.com/cloudposse/atmos/pkg/exec"
	"github.com/cloudposse/atmos/pkg/filesystem"
	"github.com/cloudposse/atmos/pkg/schema"
)

// writeTempFile creates an empty file at path, failing the test on error.
func writeTempFile(path string) error {
	return os.WriteFile(path, []byte("x"), 0o600)
}

// newFakePathExecutor returns a CommandExecutor whose LookPath succeeds only for
// candidates whose base name is in installed. This simulates a PATH where only
// the named tools are present, independent of which directory is searched.
func newFakePathExecutor(t *testing.T, installed ...string) execpkg.CommandExecutor {
	t.Helper()
	ctrl := gomock.NewController(t)
	mockExec := execpkg.NewMockCommandExecutor(ctrl)
	present := make(map[string]bool, len(installed))
	for _, name := range installed {
		present[name] = true
	}
	mockExec.EXPECT().LookPath(gomock.Any()).DoAndReturn(func(file string) (string, error) {
		if present[filepath.Base(file)] {
			return file, nil
		}
		return "", exec.ErrNotFound
	}).AnyTimes()
	return mockExec
}

// requireHandlerWith builds a RequireHandler with the given executor and the
// real OS filesystem, bypassing the singleton registry.
func requireHandlerWith(execer execpkg.CommandExecutor) *RequireHandler {
	return &RequireHandler{
		BaseHandler: NewBaseHandler("require", CategoryCommand, false, "assert"),
		exec:        execer,
		fs:          filesystem.NewOSFileSystem(),
	}
}

// varsWithPath returns Variables seeded with a deterministic PATH so tool lookups
// target known directories rather than the host environment.
func varsWithPath(dirs ...string) *Variables {
	vars := NewVariables()
	vars.Env["PATH"] = strings.Join(dirs, string(filepath.ListSeparator))
	return vars
}

// allHints flattens the user-facing hints attached to a built error.
func allHints(err error) string {
	return strings.Join(cockroachErrors.GetAllHints(err), "\n")
}

func TestRequireHandler_Registered(t *testing.T) {
	for _, name := range []string{"require", "assert"} {
		h, ok := Get(name)
		require.True(t, ok, "handler %q should be registered", name)
		assert.Equal(t, "require", h.GetName())
	}
	assert.True(t, IsExtendedStepType("require"))
	assert.True(t, IsExtendedStepType("assert"))
}

func TestRequireHandler_Validate(t *testing.T) {
	h := requireHandlerWith(newFakePathExecutor(t))

	t.Run("empty step is rejected", func(t *testing.T) {
		err := h.Validate(&schema.WorkflowStep{Name: "gate", Type: "require"})
		require.Error(t, err)
		require.ErrorIs(t, err, errUtils.ErrRequireStepEmpty)
	})

	t.Run("tools only is valid", func(t *testing.T) {
		require.NoError(t, h.Validate(&schema.WorkflowStep{Tools: []string{"vhs"}}))
	})
	t.Run("files only is valid", func(t *testing.T) {
		require.NoError(t, h.Validate(&schema.WorkflowStep{Files: []string{"a.txt"}}))
	})
	t.Run("dirs only is valid", func(t *testing.T) {
		require.NoError(t, h.Validate(&schema.WorkflowStep{Dirs: []string{"d"}}))
	})
}

func TestRequireHandler_Execute_AllPresent(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "present.txt")
	require.NoError(t, writeTempFile(file))

	h := requireHandlerWith(newFakePathExecutor(t, "vhs", "ffmpeg", "ttyd"))
	step := &schema.WorkflowStep{
		Name:  "gate",
		Type:  "require",
		Tools: []string{"vhs", "ffmpeg", "ttyd"},
		Files: []string{file},
		Dirs:  []string{dir},
	}

	result, err := h.Execute(context.Background(), step, varsWithPath("/fake/bin"))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 3, result.Metadata["tools"])
	assert.Equal(t, 1, result.Metadata["files"])
	assert.Equal(t, 1, result.Metadata["dirs"])
}

func TestRequireHandler_Execute_MissingTool(t *testing.T) {
	// vhs present, ttyd missing.
	h := requireHandlerWith(newFakePathExecutor(t, "vhs"))
	step := &schema.WorkflowStep{
		Name:  "gate",
		Type:  "require",
		Tools: []string{"vhs", "ttyd"},
		Hint:  "on macOS run: brew install ttyd",
	}

	_, err := h.Execute(context.Background(), step, varsWithPath("/fake/bin"))
	require.Error(t, err)
	require.ErrorIs(t, err, errUtils.ErrRequirementsNotMet)
	hints := allHints(err)
	assert.Contains(t, hints, "ttyd")
	assert.NotContains(t, hints, "vhs") // vhs is present, should not be listed as missing.
	assert.Contains(t, hints, "brew install ttyd")
}

func TestRequireHandler_Execute_MissingFileAndDir(t *testing.T) {
	dir := t.TempDir()
	missingFile := filepath.Join(dir, "nope.txt")
	missingDir := filepath.Join(dir, "no-such-dir")

	// A plain file masquerading as a dir must be reported as a missing dir.
	notADir := filepath.Join(dir, "file-not-dir")
	require.NoError(t, writeTempFile(notADir))

	h := requireHandlerWith(newFakePathExecutor(t))
	step := &schema.WorkflowStep{
		Name:  "gate",
		Type:  "require",
		Files: []string{missingFile},
		Dirs:  []string{missingDir, notADir},
	}

	_, err := h.Execute(context.Background(), step, varsWithPath("/fake/bin"))
	require.Error(t, err)
	hints := allHints(err)
	assert.Contains(t, hints, missingFile)
	assert.Contains(t, hints, missingDir)
	assert.Contains(t, hints, notADir)
}

func TestRequireHandler_Execute_MixedMissing_ListsFirstAndLast(t *testing.T) {
	h := requireHandlerWith(newFakePathExecutor(t)) // nothing installed.
	step := &schema.WorkflowStep{
		Name:  "gate",
		Type:  "require",
		Tools: []string{"alpha", "beta", "omega"},
	}

	_, err := h.Execute(context.Background(), step, varsWithPath("/fake/bin"))
	require.Error(t, err)
	hints := allHints(err)
	// Assert both the first and last missing items appear (not just a count).
	assert.Contains(t, hints, "alpha")
	assert.Contains(t, hints, "omega")
}

func TestRequireHandler_Execute_PathFromVarsEnv(t *testing.T) {
	// The handler must search vars.Env["PATH"], not the host PATH. Use a fake
	// executor that records the directories it was asked about.
	ctrl := gomock.NewController(t)
	mockExec := execpkg.NewMockCommandExecutor(ctrl)
	var seenDirs []string
	mockExec.EXPECT().LookPath(gomock.Any()).DoAndReturn(func(file string) (string, error) {
		seenDirs = append(seenDirs, filepath.Dir(file))
		return "", exec.ErrNotFound
	}).AnyTimes()

	h := requireHandlerWith(mockExec)
	step := &schema.WorkflowStep{Name: "gate", Type: "require", Tools: []string{"vhs"}}

	fakeDir := filepath.Join(string(filepath.Separator)+"fake", "toolchain", "bin")
	_, err := h.Execute(context.Background(), step, varsWithPath(fakeDir))
	require.Error(t, err)
	assert.Contains(t, seenDirs, fakeDir, "tool lookup should target the PATH from vars.Env")
}

func TestRequireHandler_Execute_TemplateResolution(t *testing.T) {
	h := requireHandlerWith(newFakePathExecutor(t, "vhs"))
	vars := varsWithPath("/fake/bin")
	vars.Flags["tool"] = "vhs"

	step := &schema.WorkflowStep{
		Name:  "gate",
		Type:  "require",
		Tools: []string{"{{ .flags.tool }}"},
	}

	result, err := h.Execute(context.Background(), step, vars)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Metadata["tools"])
}

func TestRequireHandler_Execute_AbsoluteToolPath(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "mytool")
	require.NoError(t, writeTempFile(bin))

	// Executor confirms the absolute path directly (no PATH search).
	h := requireHandlerWith(newFakePathExecutor(t, "mytool"))
	step := &schema.WorkflowStep{Name: "gate", Type: "require", Tools: []string{bin}}

	result, err := h.Execute(context.Background(), step, varsWithPath())
	require.NoError(t, err)
	assert.Equal(t, 1, result.Metadata["tools"])
}
