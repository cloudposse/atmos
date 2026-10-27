package sandbox

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	atmosgit "github.com/cloudposse/atmos/pkg/git"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Options controls selection and the checkout contents. Staged uses the caller's
// index; pr and all overlay tracked working-tree contents. No refs are fetched.
type Options struct {
	Directory  string
	Scope      string
	BaseRef    string
	Env        []string
	Runner     atmosgit.Runner
	Stderr     io.Writer
	OnProgress func(Progress)
}

// Sandbox owns a temporary repository. Call Close even if checks or reconciliation
// fail. Files and Added contain repo-relative paths; Base is a resolved commit.
type Sandbox struct {
	mu       sync.Mutex
	Root     string
	Path     string
	Scope    string
	Base     string
	Files    []string
	Added    []string
	ctx      context.Context
	paths    []string
	original map[string]fileState
	before   map[string]fileState
	closed   bool
	finished bool
}

// Open captures the index and selected files without modifying the source repository.
//
//nolint:gocritic // Copy options so applying defaults does not mutate caller configuration.
func Open(ctx context.Context, opts Options) (_ *Sandbox, err error) {
	defer perf.Track(nil, "sandbox.Open")()

	if err := normalizeOptions(&opts); err != nil {
		return nil, err
	}
	ctx = context.WithValue(ctx, servicesKey{}, gitServices{runner: opts.Runner, env: opts.Env, stderr: opts.Stderr, progress: opts.OnProgress})
	reportProgress(ctx, "Selecting files for isolated checks", 0, 0)
	root, err := sourceGitOutput(ctx, opts.Directory, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, err
	}
	s := &Sandbox{ctx: ctx, Root: strings.TrimSuffix(root, "\n"), Scope: opts.Scope, Base: "HEAD", original: map[string]fileState{}, before: map[string]fileState{}}
	if err := s.selectFiles(opts.BaseRef); err != nil {
		return nil, err
	}
	s.Path, err = os.MkdirTemp("", "atmos-git-sandbox-*")
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			_ = s.Close()
		}
	}()
	canonicalPath, err := filepath.EvalSymlinks(s.Path)
	if err != nil {
		return nil, err
	}
	s.Path = canonicalPath
	// Capture source states before materializing the checkout so concurrent edits
	// during setup cannot silently become the expected working-tree state.
	reportProgress(ctx, "Capturing original file contents", 0, len(s.Files))
	s.original, err = readStates(ctx, s.Root, s.Files)
	if err != nil {
		return nil, err
	}
	if err = snapshot(ctx, s.Root, s.Path, opts.Scope); err != nil {
		return nil, err
	}
	reportProgress(ctx, "Preparing check baselines", 0, len(s.Files))
	s.before, err = readStates(ctx, s.Path, s.Files)
	if err != nil {
		return nil, err
	}
	s.paths = slices.Clone(s.Files)
	return s, nil
}

func (s *Sandbox) selectFiles(baseRef string) error {
	var err error
	s.Files, err = selectedFiles(s.ctx, s.Root, s.Scope, baseRef)
	if err != nil {
		return err
	}
	s.Added, err = addedFiles(s.ctx, s.Root, s.Scope, baseRef)
	if err != nil {
		return err
	}
	if s.Scope == "pr" {
		base, err := sourceGitOutput(s.ctx, s.Root, "rev-parse", "--verify", baseRef+"^{commit}")
		if err != nil {
			return err
		}
		s.Base = strings.TrimSpace(base)
	}
	return nil
}

func normalizeOptions(opts *Options) error {
	if opts.Scope == "" {
		opts.Scope = "staged"
	}
	if opts.Scope != "staged" && opts.Scope != "pr" && opts.Scope != "all" {
		return fmt.Errorf("%w: scope %q; expected staged, pr, or all", ErrInvalidOptions, opts.Scope)
	}
	if opts.BaseRef == "" {
		opts.BaseRef = "origin/main"
	}
	if opts.Runner == nil {
		opts.Runner = atmosgit.NewExecRunner()
	}
	if opts.Env == nil {
		opts.Env = os.Environ()
	}
	if opts.Stderr == nil {
		opts.Stderr = io.Discard
	}

	return nil
}

// Track adds formatter side effects (e.g. go.sum) to reconciliation. Call before
// running modifying checks. Paths must be repository-relative, without symlink parents.
func (s *Sandbox) Track(paths []string) error {
	defer perf.Track(nil, "sandbox.Track")()

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.finished {
		return ErrClosed
	}
	for _, name := range paths {
		if !filepath.IsLocal(name) {
			return fmt.Errorf("%w: %q", ErrUnsafePath, name)
		}
		name = filepath.ToSlash(filepath.Clean(name))
		if _, exists := s.before[name]; exists {
			continue
		}
		original, before, err := captureStates(s.ctx, s.Root, s.Path, []string{name})
		if err != nil {
			return err
		}
		for p, state := range original {
			s.original[p] = state
			s.before[p] = before[p]
			s.paths = append(s.paths, p)
		}
	}
	return nil
}

// Copy adds an existing, untracked support file such as a prebuilt linter. Its
// timestamp and permissions are preserved. It is never reconciled into the source.
func (s *Sandbox) Copy(paths []string) error {
	defer perf.Track(nil, "sandbox.Copy")()

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.finished {
		return ErrClosed
	}
	for _, name := range paths {
		if err := s.copyFile(name); err != nil {
			return err
		}
	}
	return nil
}

func (s *Sandbox) copyFile(name string) error {
	if !filepath.IsLocal(name) {
		return fmt.Errorf("%w: %q", ErrUnsafePath, name)
	}
	name = filepath.ToSlash(filepath.Clean(name))
	if _, tracked := s.before[name]; tracked {
		return fmt.Errorf("%w: cannot replace tracked sandbox file %q", ErrSupportFile, name)
	}
	src, err := safePath(s.Root, name)
	if err != nil {
		return err
	}
	dst, err := safePath(s.Path, name)
	if err != nil {
		return err
	}
	if _, err := os.Lstat(dst); !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%w: destination already exists: %s", ErrSupportFile, name)
	}
	return copySupport(src, dst)
}

func copySupport(src, dst string) error {
	state, err := readState(src)
	if err != nil {
		return err
	}
	if !state.Exists || !state.Mode.IsRegular() {
		return fmt.Errorf("%w: %q must be an existing regular file", ErrSupportFile, src)
	}
	if err := writeState(dst, state); err != nil {
		return err
	}
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if err := os.Chtimes(dst, info.ModTime(), info.ModTime()); err != nil {
		return err
	}
	return nil
}

// Finish reports differences, optionally applying them as unstaged edits. It never
// writes the live index. Cancellation prevents reconciliation; conflicts preserve
// the original file and produce a recoverable patch. It may run only once.
func (s *Sandbox) Finish(apply bool) (bool, error) {
	defer perf.Track(nil, "sandbox.Finish")()

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.finished {
		return false, ErrClosed
	}
	s.finished = true
	if err := s.ctx.Err(); err != nil {
		return false, err
	}
	mode := "check"
	if apply {
		mode = "format"
	}
	return s.finish(mode)
}

// Close deletes only this sandbox's private checkout and Git metadata.
func (s *Sandbox) Close() error {
	defer perf.Track(nil, "sandbox.Close")()

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	return os.RemoveAll(filepath.Clean(s.Path))
}
