package generator

import (
	"fmt"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/generator/storage"
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
// untouched and skipped=true is returned. HeadSHA is the hash of the initial
// commit (empty when skipped), so callers can pin it as the project's frozen
// scaffold base ref -- see PinInitialBaseRef.
func InitGitRepository(opts InitGitOptions) (skipped bool, headSHA string, err error) {
	defer perf.Track(nil, "generator.InitGitRepository")()

	if opts.TargetPath == "" {
		return false, "", fmt.Errorf("%w: git init target path is empty", errUtils.ErrGitTargetPathInvalid)
	}
	if isInsideGitRepository(opts.TargetPath) {
		return true, "", nil
	}

	repo, err := git.PlainInit(opts.TargetPath, false)
	if err != nil {
		return false, "", fmt.Errorf("%w: initialize generated project git repository: %w", errUtils.ErrGitWorkdirNotInitialized, err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		return false, "", fmt.Errorf("%w: open generated project worktree: %w", errUtils.ErrGitWorkdirNotInitialized, err)
	}
	if err := wt.AddGlob("."); err != nil {
		return false, "", fmt.Errorf("%w: stage generated project files: %w", errUtils.ErrGitArtifactWrite, err)
	}
	commitHash, err := wt.Commit(initialCommitMessage(opts.TemplateName, opts.TemplateVersion), &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Atmos",
			Email: gitInitAuthorEmail,
			When:  time.Now(),
		},
	})
	if err != nil {
		return false, "", fmt.Errorf("%w: commit generated project files: %w", errUtils.ErrGitArtifactWrite, err)
	}
	return false, commitHash.String(), nil
}

// pinOptions holds PinInitialBaseRef's optional metadata fields. Grouping
// TemplateName/TemplateVersion/Source behind functional options (rather than
// three same-typed positional string parameters) rules out accidentally
// swapping them at a call site, matching this package's existing options
// pattern (see options.go's Option/WriterOption).
type pinOptions struct {
	templateName    string
	templateVersion string
	source          string
}

// PinOption is a functional option for PinInitialBaseRef.
type PinOption func(*pinOptions)

// WithTemplateName sets the template name recorded in the pinned metadata.
func WithTemplateName(name string) PinOption {
	defer perf.Track(nil, "generator.WithTemplateName")()

	return func(o *pinOptions) {
		o.templateName = name
	}
}

// WithTemplateVersion sets the template version recorded in the pinned metadata.
func WithTemplateVersion(version string) PinOption {
	defer perf.Track(nil, "generator.WithTemplateVersion")()

	return func(o *pinOptions) {
		o.templateVersion = version
	}
}

// WithSource sets the template source recorded in the pinned metadata.
func WithSource(source string) PinOption {
	defer perf.Track(nil, "generator.WithSource")()

	return func(o *pinOptions) {
		o.source = source
	}
}

// PinInitialBaseRef persists headSHA (the initial commit created by
// InitGitRepository) as targetPath's frozen scaffold base ref, in
// .atmos/scaffold/metadata.yaml. A later `--update` with no explicit
// --base-ref reads this pin instead of defaulting to live HEAD, so it always
// 3-way-merges against the commit that actually contains the pristine
// generated content -- regardless of what the user has committed since.
// Without this, a customization the user commits after generation becomes
// indistinguishable from the unmodified base by the time --update runs, and
// the merge silently lets the freshly rendered template overwrite it.
//
// TargetPath and headSHA are the operation's actual subject (what to pin,
// and where) and stay positional; the template's own descriptive metadata
// (name/version/source) is optional configuration, passed via
// WithTemplateName/WithTemplateVersion/WithSource.
//
// No-op when headSHA is empty (InitGitRepository returned skipped=true,
// meaning targetPath was already inside a git repository and no commit --
// containing verified pristine content -- was created for atmos to pin).
func PinInitialBaseRef(targetPath, headSHA string, opts ...PinOption) error {
	defer perf.Track(nil, "generator.PinInitialBaseRef")()

	if headSHA == "" {
		return nil
	}

	var pinOpts pinOptions
	for _, opt := range opts {
		opt(&pinOpts)
	}

	metadata := storage.NewScaffoldMetadata(pinOpts.templateName, pinOpts.templateVersion, pinOpts.source, headSHA, nil)
	if err := storage.NewMetadataStorage(storage.ScaffoldMetadataPath(targetPath)).Save(metadata); err != nil {
		return fmt.Errorf("%w: pin initial scaffold base ref: %w", errUtils.ErrMetadataSave, err)
	}
	return nil
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
