package sandbox

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"golang.org/x/sync/errgroup"
)

func splitPaths(data string) []string {
	paths := strings.FieldsFunc(data, func(r rune) bool { return r == 0 })
	if paths == nil {
		return []string{}
	}
	return paths
}

func selectedFiles(ctx context.Context, root, scope, base string) ([]string, error) {
	args := []string{gitDiff, "--cached", gitNameOnly, gitNull, "--diff-filter=ACMR"}
	if scope == "all" {
		args = []string{"ls-files", gitNull}
	}
	if scope == "pr" {
		if _, err := sourceGitOutput(ctx, root, gitRevParse, "--verify", base+"^{commit}"); err != nil {
			return nil, fmt.Errorf("%w: base ref %q is unavailable; fetch it or pass --base-ref: %w", ErrMissingBase, base, err)
		}
		args = []string{gitDiff, gitNameOnly, gitNull, "--diff-filter=ACMR", base + "...HEAD"}
	}
	out, err := sourceGitOutput(ctx, root, args...)
	return splitPaths(out), err
}

func addedFiles(ctx context.Context, root, scope, base string) ([]string, error) {
	args := []string{gitDiff, "--cached", gitNameOnly, gitNull, "--diff-filter=A"}
	if scope == "pr" {
		args = []string{gitDiff, gitNameOnly, gitNull, "--diff-filter=A", base + "...HEAD"}
	}
	result, err := sourceGitOutput(ctx, root, args...)
	return splitPaths(result), err
}

// snapshot creates private Git metadata and checks out the caller's index. Object
// storage is read-only through alternates; refs and index writes stay private.
func snapshot(ctx context.Context, root, dest, scope, baseRef string) error {
	reportProgress(ctx, "Preparing private Git metadata", 0, 0)
	if err := initializeRepository(ctx, root, dest, baseRef); err != nil {
		return err
	}
	entries, err := copyIndex(ctx, root, dest)
	if err != nil {
		return err
	}
	if scope == "staged" {
		reportProgress(ctx, "Copying staged files", 0, len(entries))
		if _, err := gitOutput(ctx, dest, "checkout-index", "--all", "--force"); err != nil {
			return err
		}
	}
	// PR/all need working-tree contents only. Avoid checking out index blobs just
	// to replace every one of them with the working copy immediately afterward.
	return overlayFiles(ctx, root, dest, scope, entries)
}

func initializeRepository(ctx context.Context, root, dest, baseRef string) error {
	if _, err := gitOutput(ctx, dest, "init", "--quiet"); err != nil {
		return err
	}
	objects, err := sourceGitOutput(ctx, root, gitRevParse, "--path-format=absolute", "--git-path", "objects")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dest, ".git", "objects", "info", "alternates"), []byte(strings.TrimSuffix(objects, "\n")+"\n"), privateFileMode); err != nil {
		return err
	}
	if _, err := gitOutput(ctx, dest, "config", "core.hooksPath", filepath.Join(dest, ".git", "disabled-hooks")); err != nil {
		return err
	}
	return copyRefs(ctx, root, dest, baseRef)
}

// copyRefs gives the private repository the refs that checks compare against: HEAD,
// a pending merge, and the caller's comparison base under its own full ref name.
// A base given as a commit needs no ref; its objects are reachable through alternates.
func copyRefs(ctx context.Context, root, dest, baseRef string) error {
	for _, ref := range refsToCopy(ctx, root, baseRef) {
		value, err := sourceGitOutput(quietContext(ctx), root, gitRevParse, "--verify", ref)
		if err != nil {
			continue
		}
		if err := copyRef(ctx, dest, ref, strings.TrimSpace(value)); err != nil {
			return err
		}
	}
	return ensureHead(ctx, dest)
}

// refsToCopy lists HEAD, a possibly pending merge, and the base's full ref name.
func refsToCopy(ctx context.Context, root, baseRef string) []string {
	refs := []string{"HEAD", mergeHeadRef}
	if strings.HasPrefix(baseRef, "-") { // Never let a base ref act as a Git option.
		return refs
	}
	name, err := sourceGitOutput(quietContext(ctx), root, gitRevParse, "--symbolic-full-name", baseRef)
	if err != nil || strings.TrimSpace(name) == "" {
		return refs
	}
	return append(refs, strings.TrimSpace(name))
}

// copyRef records a ref in the private repository. Checks of a merge commit must see
// its second parent as Git does; without it, everything merged in counts as branch work.
func copyRef(ctx context.Context, dest, ref, value string) error {
	if ref == mergeHeadRef {
		return os.WriteFile(filepath.Join(dest, ".git", mergeHeadRef), []byte(value+"\n"), privateFileMode)
	}
	_, err := gitOutput(ctx, dest, "update-ref", ref, value)
	return err
}

// ensureHead gives a repository without commits an empty private parent.
func ensureHead(ctx context.Context, dest string) error {
	if _, err := gitOutput(quietContext(ctx), dest, gitRevParse, "--verify", "HEAD"); err == nil {
		return nil
	}
	// Give initial-commit checks an empty private parent without touching source refs.
	tree, err := gitOutput(ctx, dest, "mktree")
	if err != nil {
		return err
	}
	commit, err := gitOutput(ctx, dest, "-c", "user.name=Atmos checks", "-c", "user.email=checks@localhost", "commit-tree", strings.TrimSpace(tree), "-m", "Empty check baseline")
	if err != nil {
		return err
	}
	_, err = gitOutput(ctx, dest, "update-ref", "HEAD", strings.TrimSpace(commit))
	return err
}

func copyIndex(ctx context.Context, root, dest string) ([]string, error) {
	entries, err := sourceGitOutput(ctx, root, "ls-files", "--stage", gitNull)
	if err != nil {
		return nil, err
	}
	paths := splitPaths(entries)
	for _, entry := range paths {
		meta, _, _ := strings.Cut(entry, "\t")
		fields := strings.Fields(meta)
		if len(fields) != 3 || fields[2] != "0" {
			return nil, ErrUnmergedIndex
		}
	}
	if out, err := gitInput(ctx, dest, strings.NewReader(entries), "update-index", gitNull, "--index-info"); err != nil {
		return nil, fmt.Errorf("copy index: %w: %s", err, out)
	}
	return paths, nil
}

// PR/all inspect current tracked contents. Preserve timestamps for prebuilt-tool checks.
func overlayFiles(ctx context.Context, root, dest, scope string, entries []string) error {
	// Bound filesystem contention and the number of file buffers held at once.
	const maxCopyWorkers = 4
	copier := checkoutCopier{root: root, dest: dest, scope: scope}
	return copier.copyFiles(ctx, entries, min(runtime.GOMAXPROCS(0), maxCopyWorkers))
}

type checkoutCopier struct {
	root, dest, scope string
}

// Workers only touch independent checkout paths. Progress callbacks are serialized,
// and all workers finish before returning, including on failure or cancellation.
func (c *checkoutCopier) copyFiles(ctx context.Context, entries []string, workers int) error {
	const progressInterval = 100
	phase := "Copying working files"
	if c.scope == "staged" {
		phase = "Preserving file timestamps"
	}
	reportProgress(ctx, phase, 0, len(entries))
	group, copyCtx := errgroup.WithContext(ctx)
	var progressMu sync.Mutex
	completed := 0
	workers = min(workers, len(entries))
	for worker := range workers {
		group.Go(func() error {
			for i := worker; i < len(entries); i += workers {
				if err := c.copyEntry(copyCtx, entries[i]); err != nil {
					return err
				}
				progressMu.Lock()
				completed++
				if completed%progressInterval == 0 {
					reportProgress(ctx, phase, completed, len(entries))
				}
				progressMu.Unlock()
			}
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	reportProgress(ctx, phase, len(entries), len(entries))
	return nil
}

func (c *checkoutCopier) copyEntry(ctx context.Context, entry string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	meta, name, _ := strings.Cut(entry, "\t")
	if strings.HasPrefix(meta, "160000 ") {
		return nil
	}
	if err := overlayFile(c.root, c.dest, c.scope, name); err != nil {
		return fmt.Errorf("copy %q: %w", name, err)
	}
	return nil
}

// overlayFile copies working contents when requested and preserves regular-file timestamps.
func overlayFile(root, dest, scope, name string) error {
	src, err := safePath(root, name)
	if err != nil {
		return err
	}
	dst, err := safePath(dest, name)
	if err != nil {
		return err
	}
	if scope != "staged" {
		state, err := readState(src)
		if err != nil {
			return err
		}
		if err := writeCheckoutState(dst, state); err != nil {
			return err
		}
	}
	return preserveCheckoutTimestamps(src, dst)
}

// preserveCheckoutTimestamps copies timestamps without following a destination symlink.
func preserveCheckoutTimestamps(src, dst string) error {
	info, err := os.Lstat(src)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return nil
	}
	// The index can contain a symlink even when the working file is regular.
	// Chtimes follows symlinks, so only preserve timestamps on regular destinations.
	dstInfo, err := os.Lstat(dst)
	if err != nil {
		return err
	}
	if !dstInfo.Mode().IsRegular() {
		return nil
	}
	return os.Chtimes(dst, info.ModTime(), info.ModTime())
}

// writeCheckoutState prepares disposable files without syncing each file to disk.
// No check sees this checkout until preparation succeeds; live fix reconciliation
// still uses writeState's atomic writer. Per-file fsync makes large checkouts slow.
func writeCheckoutState(path string, state fileState) error {
	if !state.Exists || !state.Mode.IsRegular() {
		return writeState(path, state)
	}
	if err := os.MkdirAll(filepath.Dir(path), checkoutDirMode); err != nil {
		return err
	}
	if err := os.WriteFile(path, state.Data, state.Mode.Perm()); err != nil {
		return err
	}
	return os.Chmod(path, state.Mode.Perm())
}

// captureStates records both sides before checks modify the temporary checkout.
func captureStates(ctx context.Context, root, tmp string, paths []string) (map[string]fileState, map[string]fileState, error) {
	original, err := readStates(ctx, root, paths)
	if err != nil {
		return nil, nil, err
	}
	before, err := readStates(ctx, tmp, paths)
	return original, before, err
}

func readStates(ctx context.Context, root string, paths []string) (map[string]fileState, error) {
	states := make(map[string]fileState, len(paths))
	for _, name := range paths {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		path, err := safePath(root, name)
		if err != nil {
			return nil, err
		}
		if info, err := os.Lstat(path); err == nil && info.IsDir() {
			continue
		}
		state, err := readState(path)
		if err != nil {
			return nil, err
		}
		states[name] = state
	}
	return states, nil
}
