// Package sandbox runs repository checks against an isolated Git index and checkout.
// It never writes the source index, creates branches, registers worktrees, or fetches refs.
package sandbox

import (
	"context"
	"io"
	"os"
	"strings"

	atmosgit "github.com/cloudposse/atmos/pkg/git"
)

type (
	servicesKey struct{}
	gitServices struct {
		runner   atmosgit.Runner
		env      []string
		stderr   io.Writer
		progress func(Progress)
	}
)

func services(ctx context.Context) gitServices {
	if s, ok := ctx.Value(servicesKey{}).(gitServices); ok {
		return s
	}
	return gitServices{runner: atmosgit.NewExecRunner(), env: os.Environ(), stderr: io.Discard}
}

// CleanEnvironment removes the caller's repository/index overrides before invoking
// Git in a different checkout. Source reads must retain these overrides, especially
// GIT_INDEX_FILE supplied by git commit --only.
func CleanEnvironment(env []string) []string {
	result := make([]string, 0, len(env))
	for _, entry := range env {
		key, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(key, "GIT_") && key != "GIT_CONFIG_NOSYSTEM" && key != "GIT_CONFIG_GLOBAL" {
			continue
		}
		result = append(result, entry)
	}
	return result
}

func sourceGitOutput(ctx context.Context, dir string, args ...string) (string, error) {
	s := services(ctx)
	result, err := s.runner.Run(ctx, "git", args, atmosgit.RunOptions{Dir: dir, Env: s.env, Stderr: s.stderr, DiscoverRepository: len(args) == 2 && args[0] == "rev-parse" && args[1] == "--show-toplevel"})
	return result.Stdout, err
}

func gitOutput(ctx context.Context, dir string, args ...string) (string, error) {
	return gitInput(ctx, dir, nil, args...)
}

func gitInput(ctx context.Context, dir string, input io.Reader, args ...string) (string, error) {
	s := services(ctx)
	result, err := s.runner.Run(ctx, "git", args, atmosgit.RunOptions{Dir: dir, Env: CleanEnvironment(s.env), Stdin: input, Stderr: s.stderr})
	return result.Stdout, err
}

func quietContext(ctx context.Context) context.Context {
	s := services(ctx)
	s.stderr = io.Discard
	return context.WithValue(ctx, servicesKey{}, s)
}
