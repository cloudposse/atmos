package generator

import (
	"fmt"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

const gitInitAuthorEmail = "atmos@localhost"

// InitGitOptions controls repository initialization after project generation.
type InitGitOptions struct {
	TargetPath      string
	TemplateName    string
	TemplateVersion string
}

// InitGitRepository initializes targetPath as a git repository and creates an
// initial commit. If targetPath is already inside a git repository, it is left
// untouched and skipped=true is returned.
func InitGitRepository(opts InitGitOptions) (skipped bool, err error) {
	defer perf.Track(nil, "generator.InitGitRepository")()

	if opts.TargetPath == "" {
		return false, fmt.Errorf("%w: git init target path is empty", errUtils.ErrGitTargetPathInvalid)
	}
	if isInsideGitRepository(opts.TargetPath) {
		return true, nil
	}

	repo, err := git.PlainInit(opts.TargetPath, false)
	if err != nil {
		return false, fmt.Errorf("%w: initialize generated project git repository: %w", errUtils.ErrGitWorkdirNotInitialized, err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		return false, fmt.Errorf("%w: open generated project worktree: %w", errUtils.ErrGitWorkdirNotInitialized, err)
	}
	if err := wt.AddGlob("."); err != nil {
		return false, fmt.Errorf("%w: stage generated project files: %w", errUtils.ErrGitArtifactWrite, err)
	}
	if _, err := wt.Commit(initialCommitMessage(opts.TemplateName, opts.TemplateVersion), &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Atmos",
			Email: gitInitAuthorEmail,
			When:  time.Now(),
		},
	}); err != nil {
		return false, fmt.Errorf("%w: commit generated project files: %w", errUtils.ErrGitArtifactWrite, err)
	}
	return false, nil
}

func isInsideGitRepository(path string) bool {
	_, err := git.PlainOpenWithOptions(path, &git.PlainOpenOptions{DetectDotGit: true})
	return err == nil
}

func initialCommitMessage(templateName, templateVersion string) string {
	if templateVersion == "" {
		return fmt.Sprintf("Initial commit from atmos init (%s)", templateName)
	}
	return fmt.Sprintf("Initial commit from atmos init (%s@%s)", templateName, templateVersion)
}
