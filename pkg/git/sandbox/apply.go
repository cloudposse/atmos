package sandbox

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type fixStates struct{ original, before, after fileState }

func (s *Sandbox) finish(mode string) (bool, error) {
	ctx, root, tmp, paths, original, before := s.ctx, s.Root, s.Path, s.paths, s.original, s.before
	changed := false
	var failures []error
	for _, name := range paths {
		old, ok := before[name]
		if !ok {
			continue
		}
		path, err := safePath(tmp, name)
		if err != nil {
			return changed, err
		}
		after, err := readState(path)
		if err != nil {
			return changed, err
		}
		if equalState(old, after) {
			continue
		}
		changed = true
		fmt.Fprintf(services(ctx).stderr, "Modified by checks: %s\n", name)
		if mode == "check" {
			continue
		}
		if err := applyFix(ctx, root, name, &fixStates{original[name], old, after}); err != nil {
			failures = append(failures, err)
		}
	}
	return changed, errors.Join(failures...)
}

func applyFix(ctx context.Context, root, name string, states *fixStates) error {
	original, before, after := states.original, states.before, states.after
	path, err := safePath(root, name)
	if err != nil {
		return err
	}
	current, err := readState(path)
	if err != nil {
		return err
	}
	if !equalState(current, original) {
		return saveConflict(ctx, name, before, after, "file changed while checks ran")
	}
	result := after
	if !equalState(original, before) {
		result, err = mergeFix(ctx, original, before, after)
		if err != nil {
			return saveConflict(ctx, name, before, after, "fix overlaps unstaged changes")
		}
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	// Check again after merging, immediately before replacing the file.
	latest, err := readState(path)
	if err != nil {
		return err
	}
	if !equalState(latest, original) {
		return saveConflict(ctx, name, before, after, "concurrent edit")
	}
	return writeState(path, result)
}

func mergeFix(ctx context.Context, ours, base, theirs fileState) (fileState, error) {
	for _, state := range []fileState{ours, base, theirs} {
		if err := mergeable(state); err != nil {
			return fileState{}, err
		}
	}

	dir, err := os.MkdirTemp("", "atmos-merge-fix-*")
	if err != nil {
		return fileState{}, err
	}
	defer os.RemoveAll(dir)
	for name, state := range map[string]fileState{"ours": ours, "base": base, "theirs": theirs} {
		if err := os.WriteFile(filepath.Join(dir, name), state.Data, privateFileMode); err != nil {
			return fileState{}, err
		}
	}
	data, err := gitOutput(ctx, dir, "merge-file", "-p", "ours", "base", "theirs")
	if err != nil {
		return fileState{}, err
	}
	ours.Data = []byte(data) // Preserve the user's working-tree mode.
	return ours, nil
}

func mergeable(state fileState) error {
	if !state.Exists || !state.Mode.IsRegular() {
		return fmt.Errorf("%w: non-regular or removed file", ErrFixConflict)
	}
	if bytes.Contains(state.Data, []byte{0}) {
		return fmt.Errorf("%w: binary file", ErrFixConflict)
	}
	return nil
}

func saveConflict(ctx context.Context, name string, before, after fileState, reason string) error {
	// Keep recoverable patches outside the working tree and never auto-apply them.
	dir, err := os.MkdirTemp("", "atmos-check-recovery-*")
	if err != nil {
		return err
	}
	for side, state := range map[string]fileState{"before": before, "after": after} {
		base := filepath.Join(dir, side)
		if err := os.MkdirAll(base, privateDirMode); err != nil {
			return err
		}
		path, err := safePath(base, name)
		if err != nil {
			return err
		}
		if err := writeState(path, state); err != nil {
			return err
		}
	}
	patch, diffErr := gitOutput(ctx, dir, "diff", "--no-index", "--binary", "--src-prefix=", "--dst-prefix=", "--", "before", "after")
	if len(patch) == 0 {
		return fmt.Errorf("%w: %s: %s; recovery files saved at %s: %w", ErrFixConflict, name, reason, dir, diffErr)
	}
	patchPath := filepath.Join(dir, "fix.patch")
	if err := os.WriteFile(patchPath, []byte(patch), privateFileMode); err != nil {
		return err
	}
	return fmt.Errorf("%w: %s: %s; original untouched. Review %s (before/after copies alongside it)", ErrFixConflict, name, reason, patchPath)
}
