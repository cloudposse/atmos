package sandbox

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	atmosgit "github.com/cloudposse/atmos/pkg/git"
)

func TestFinishRejectsUnsafeAndNonFileOutputs(t *testing.T) {
	for _, name := range []string{"../escape", "directory"} {
		t.Run(name, func(t *testing.T) {
			root, dest := t.TempDir(), t.TempDir()
			require.NoError(t, os.Mkdir(filepath.Join(dest, "directory"), 0o755))
			box := &Sandbox{ctx: t.Context(), Root: root, Path: dest, paths: []string{"untracked", name}, before: map[string]fileState{name: {}}}
			changed, err := box.Finish(true)
			require.Error(t, err)
			require.False(t, changed)
		})
	}
}

func TestFinishPreservesConflictingFilesAndAppliesIndependentFixes(t *testing.T) {
	root := newRepo(t)
	for _, name := range []string{"conflict", "independent"} {
		mustWrite(t, filepath.Join(root, name), "staged\n")
	}
	git(t, root, "add", ".")
	index := git(t, root, "ls-files", "--stage")
	box, err := Open(t.Context(), Options{Directory: root})
	require.NoError(t, err)
	defer box.Close()
	for _, name := range []string{"conflict", "independent"} {
		mustWrite(t, filepath.Join(box.Path, name), "formatted\n")
	}
	mustWrite(t, filepath.Join(root, "conflict"), "concurrent edit\n")
	recovery := t.TempDir()
	t.Setenv("TMPDIR", recovery)
	t.Setenv("TMP", recovery)
	t.Setenv("TEMP", recovery)
	changed, err := box.Finish(true)
	require.True(t, changed)
	require.ErrorIs(t, err, ErrFixConflict)
	for name, want := range map[string]string{"conflict": "concurrent edit\n", "independent": "formatted\n"} {
		data, err := os.ReadFile(filepath.Join(root, name))
		require.NoError(t, err)
		require.Equal(t, want, string(data))
	}
	require.Equal(t, index, git(t, root, "ls-files", "--stage"))
	patches, err := filepath.Glob(filepath.Join(recovery, "atmos-check-recovery-*", "fix.patch"))
	require.NoError(t, err)
	require.Len(t, patches, 1)
	patch, err := os.ReadFile(patches[0])
	require.NoError(t, err)
	require.Contains(t, string(patch), "+formatted")
}

func TestApplyFixDetectsChangesDuringMerge(t *testing.T) {
	for _, change := range []string{"edit", "directory", "cancel"} {
		t.Run(change, func(t *testing.T) {
			root := newRepo(t)
			path := filepath.Join(root, "file")
			before := fileState{Exists: true, Mode: 0o644, Data: []byte("first\n2\n3\n4\n5\n6\nlast\n")}
			original, after := before, before
			original.Data = []byte(strings.ReplaceAll(string(before.Data), "last", "unstaged"))
			after.Data = []byte(strings.ReplaceAll(string(before.Data), "first", "formatted"))
			require.NoError(t, writeState(path, original))
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			recovery := t.TempDir()
			t.Setenv("TMPDIR", recovery)
			t.Setenv("TMP", recovery)
			t.Setenv("TEMP", recovery)
			runner := plumbingRunner(func(ctx context.Context, command string, args []string, opts atmosgit.RunOptions) (atmosgit.RunResult, error) {
				result, err := atmosgit.NewExecRunner().Run(ctx, command, args, opts)
				if args[0] == "merge-file" {
					require.NoError(t, err)
					switch change {
					case "edit":
						mustWrite(t, path, "new concurrent edit\n")
					case "directory":
						require.NoError(t, os.Remove(path))
						require.NoError(t, os.Mkdir(path, 0o755))
					case "cancel":
						cancel()
					}
				}
				return result, err
			})
			s := services(ctx)
			s.runner = runner
			ctx = context.WithValue(ctx, servicesKey{}, s)
			err := applyFix(ctx, root, "file", &fixStates{original, before, after})
			switch change {
			case "edit":
				require.ErrorIs(t, err, ErrFixConflict)
				require.ErrorContains(t, err, "concurrent edit")
				data, readErr := os.ReadFile(path)
				require.NoError(t, readErr)
				require.Equal(t, "new concurrent edit\n", string(data))
			case "directory":
				require.ErrorIs(t, err, ErrFileType)
				require.DirExists(t, path)
			case "cancel":
				require.ErrorIs(t, err, context.Canceled)
				data, readErr := os.ReadFile(path)
				require.NoError(t, readErr)
				require.Equal(t, original.Data, data)
			}
		})
	}
}

func TestReconciliationRejectsUnsupportedStates(t *testing.T) {
	regular := fileState{Exists: true, Mode: 0o644, Data: []byte("text")}
	for _, invalid := range []fileState{{}, {Exists: true, Mode: os.ModeSymlink}, {Exists: true, Mode: 0o644, Data: []byte{0}}} {
		_, err := mergeFix(t.Context(), regular, invalid, regular)
		require.ErrorIs(t, err, ErrFixConflict)
	}
	root := t.TempDir()
	require.ErrorIs(t, applyFix(t.Context(), root, "../outside", &fixStates{}), ErrUnsafePath)
	require.NoError(t, os.Mkdir(filepath.Join(root, "directory"), 0o755))
	require.ErrorIs(t, applyFix(t.Context(), root, "directory", &fixStates{}), ErrFileType)
}

func TestRecoveryRetainsCopiesWhenDiffFails(t *testing.T) {
	recovery := t.TempDir()
	t.Setenv("TMPDIR", recovery)
	t.Setenv("TMP", recovery)
	t.Setenv("TEMP", recovery)
	failure := errors.New("diff unavailable")
	s := services(t.Context())
	s.runner = plumbingRunner(func(_ context.Context, _ string, args []string, opts atmosgit.RunOptions) (atmosgit.RunResult, error) {
		require.Equal(t, "diff", args[0])
		return atmosgit.RunResult{}, failure
	})
	ctx := context.WithValue(t.Context(), servicesKey{}, s)
	before := fileState{Exists: true, Mode: 0o644, Data: []byte("before")}
	after := before
	after.Data = []byte("after")
	err := saveConflict(ctx, "file", before, after, "overlap")
	require.ErrorIs(t, err, ErrFixConflict)
	require.ErrorIs(t, err, failure)
	dirs, err := filepath.Glob(filepath.Join(recovery, "atmos-check-recovery-*"))
	require.NoError(t, err)
	require.Len(t, dirs, 1)
	for side, want := range map[string]string{"before": "before", "after": "after"} {
		data, err := os.ReadFile(filepath.Join(dirs[0], side, "file"))
		require.NoError(t, err)
		require.Equal(t, want, string(data))
	}
	require.ErrorIs(t, saveConflict(ctx, "../escape", before, after, "unsafe"), ErrUnsafePath)
}
