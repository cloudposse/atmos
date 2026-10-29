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

type plumbingRunner func(context.Context, string, []string, atmosgit.RunOptions) (atmosgit.RunResult, error)

//nolint:gocritic // Match the Runner interface's value parameter.
func (f plumbingRunner) Run(ctx context.Context, command string, args []string, opts atmosgit.RunOptions) (atmosgit.RunResult, error) {
	return f(ctx, command, args, opts)
}

func TestOpenPropagatesGitFailuresAndRemovesCheckout(t *testing.T) {
	for _, command := range []string{
		"rev-parse --show-toplevel", "diff --cached --name-only -z --diff-filter=ACMR",
		"diff --cached --name-only -z --diff-filter=A", "init --quiet",
		"rev-parse --path-format=absolute --git-path objects", "config core.hooksPath",
		"rev-parse --path-format=absolute --git-path MERGE_HEAD",
		"update-ref --no-deref HEAD", "ls-files --stage", "update-index", "checkout-index",
		"mktree", "-c user.name=Atmos checks",
	} {
		t.Run(command, func(t *testing.T) {
			root := newRepo(t)
			mustWrite(t, filepath.Join(root, "file"), "staged\n")
			if command != "mktree" && command != "-c user.name=Atmos checks" {
				commit(t, root)
				mustWrite(t, filepath.Join(root, "file"), "changed\n")
			}
			git(t, root, "add", ".")
			index := git(t, root, "ls-files", "--stage")
			failure := errors.New("git plumbing failed")
			var checkout string
			runner := plumbingRunner(func(ctx context.Context, executable string, args []string, opts atmosgit.RunOptions) (atmosgit.RunResult, error) {
				if args[0] == "init" {
					checkout = opts.Dir
				}
				if strings.HasPrefix(strings.Join(args, " "), command) {
					return atmosgit.RunResult{}, failure
				}
				return atmosgit.NewExecRunner().Run(ctx, executable, args, opts)
			})
			box, err := Open(t.Context(), Options{Directory: root, Runner: runner})
			require.ErrorIs(t, err, failure)
			require.Nil(t, box)
			if checkout != "" {
				require.NoDirExists(t, checkout)
			}
			require.Equal(t, index, git(t, root, "ls-files", "--stage"))
		})
	}
}

func TestOpenPRSelectionAndResolvedBase(t *testing.T) {
	root := newRepo(t)
	mustWrite(t, filepath.Join(root, "existing"), "base\n")
	commit(t, root)
	base := strings.TrimSpace(git(t, root, "rev-parse", "HEAD"))
	mustWrite(t, filepath.Join(root, "added"), "committed\n")
	commit(t, root)
	mustWrite(t, filepath.Join(root, "added"), "working\n")
	box, err := Open(t.Context(), Options{Directory: root, Scope: "pr", BaseRef: base})
	require.NoError(t, err)
	defer box.Close()
	require.Equal(t, base, box.Base)
	require.Equal(t, []string{"added"}, box.Files)
	require.Equal(t, []string{"added"}, box.Added)
	data, err := os.ReadFile(filepath.Join(box.Path, "added"))
	require.NoError(t, err)
	require.Equal(t, "working\n", string(data))

	failure := errors.New("base disappeared")
	reads := 0
	runner := plumbingRunner(func(ctx context.Context, executable string, args []string, opts atmosgit.RunOptions) (atmosgit.RunResult, error) {
		if strings.Join(args, " ") == "rev-parse --verify "+base+"^{commit}" {
			reads++
			if reads == 2 {
				return atmosgit.RunResult{}, failure
			}
		}
		return atmosgit.NewExecRunner().Run(ctx, executable, args, opts)
	})
	_, err = Open(t.Context(), Options{Directory: root, Scope: "pr", BaseRef: base, Runner: runner})
	require.ErrorIs(t, err, failure)
}

func TestOpenEmptyIndex(t *testing.T) {
	box, err := Open(t.Context(), Options{Directory: newRepo(t)})
	require.NoError(t, err)
	defer box.Close()
	require.Equal(t, []string{}, box.Files)
	require.Equal(t, []string{}, box.Added)
	changed, err := box.Finish(true)
	require.NoError(t, err)
	require.False(t, changed)
}

func TestOpenRejectsUnsafeSelectionsAndBaselines(t *testing.T) {
	for _, phase := range []string{"Capturing original file contents", "Preparing check baselines"} {
		t.Run(phase, func(t *testing.T) {
			root := newRepo(t)
			mustWrite(t, filepath.Join(root, "parent", "file"), "contents\n")
			git(t, root, "add", ".")
			var checkout string
			runner := plumbingRunner(func(ctx context.Context, executable string, args []string, opts atmosgit.RunOptions) (atmosgit.RunResult, error) {
				if args[0] == "init" {
					checkout = opts.Dir
				}
				return atmosgit.NewExecRunner().Run(ctx, executable, args, opts)
			})
			box, err := Open(t.Context(), Options{Directory: root, Runner: runner, OnProgress: func(p Progress) {
				if p.Phase == phase {
					dir := root
					if checkout != "" {
						dir = checkout
					}
					require.NoError(t, os.RemoveAll(filepath.Join(dir, "parent")))
					mustWrite(t, filepath.Join(dir, "parent"), "not a directory")
				}
			}})
			require.ErrorIs(t, err, ErrUnsafePath)
			require.Nil(t, box)
			if checkout != "" {
				require.NoDirExists(t, checkout)
			}
		})
	}
}

func TestTemporaryStorageFailures(t *testing.T) {
	root := newRepo(t)
	missing := filepath.Join(t.TempDir(), "missing")
	for _, name := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(name, missing)
	}
	box, err := Open(t.Context(), Options{Directory: root})
	require.Error(t, err)
	require.Nil(t, box)
	state := fileState{Exists: true, Mode: 0o644, Data: []byte("contents")}
	_, err = mergeFix(t.Context(), state, state, state)
	require.Error(t, err)
	require.Error(t, saveConflict(t.Context(), "file", state, state, "conflict"))
}

func TestCheckoutRejectsInvalidFilesystemStates(t *testing.T) {
	root, dest := t.TempDir(), t.TempDir()
	mustWrite(t, filepath.Join(root, "file"), "contents")
	mustWrite(t, filepath.Join(dest, "parent"), "not a directory")
	state := fileState{Exists: true, Mode: 0o644, Data: []byte("contents")}
	require.Error(t, writeCheckoutState(filepath.Join(dest, "parent", "file"), state))
	require.Error(t, writeCheckoutState(dest, state))
	require.Error(t, overlayFile(root, dest, "all", "parent/file"))
	require.Error(t, overlayFile(root, dest, "all", "."))
	require.Error(t, overlayFile(root, dest, "all", "file/child"))
	require.Error(t, preserveCheckoutTimestamps(filepath.Join(root, "file"), filepath.Join(dest, "absent")))
	require.Error(t, preserveCheckoutTimestamps("\x00", dest))
	require.Error(t, copySupport(root, filepath.Join(dest, "copy")))
	require.Error(t, copySupport(filepath.Join(root, "file"), dest))
	require.Error(t, writeState(filepath.Join(dest, "parent", "file"), state))
	require.Error(t, writeState(dest, fileState{Exists: true, Mode: os.ModeSymlink, Data: []byte("target")}))
	_, err := readState("\x00")
	require.Error(t, err)
	_, err = readStates(t.Context(), root, []string{"file", "."})
	require.ErrorIs(t, err, ErrUnsafePath)
	require.NoError(t, os.Mkdir(filepath.Join(root, "directory"), 0o755))
	states, err := readStates(t.Context(), root, []string{"directory", "file"})
	require.NoError(t, err)
	require.Len(t, states, 1)
	require.Equal(t, "contents", string(states["file"].Data))

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	copier := checkoutCopier{root: root, dest: dest, scope: "all"}
	require.ErrorIs(t, copier.copyFiles(ctx, nil, 1), context.Canceled)
}
