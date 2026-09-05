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

	return pinBaseRef(storage.ScaffoldMetadataPath(targetPath), storage.NewScaffoldMetadata, headSHA, opts...)
}

// PinInitialBaseRefForInit is PinInitialBaseRef's counterpart for `atmos
// init`, which records its own generation metadata separately at
// .atmos/init/metadata.yaml (see storage.InitMetadataPath) rather than
// .atmos/scaffold/metadata.yaml -- the two commands track unrelated
// generation history, but the pin mechanism itself is identical, so it's
// shared here rather than reimplemented a second time. It was reimplemented
// once before: `atmos init --update` shipped with the same silent-overwrite
// bug PinInitialBaseRef/ResolveDefaultBaseRef guard against, because the
// fix for `atmos scaffold generate --update` lived only in cmd/scaffold and
// was never ported to cmd/init.
func PinInitialBaseRefForInit(targetPath, headSHA string, opts ...PinOption) error {
	defer perf.Track(nil, "generator.PinInitialBaseRefForInit")()

	return pinBaseRef(storage.InitMetadataPath(targetPath), storage.NewInitMetadata, headSHA, opts...)
}

// pinBaseRef is the shared implementation behind PinInitialBaseRef and
// PinInitialBaseRefForInit; only the metadata path and constructor differ
// between the two commands.
func pinBaseRef(
	metadataPath string,
	newMetadata func(templateName, templateVersion, templateSource, baseRef string, variables map[string]string) *storage.GenerationMetadata,
	headSHA string,
	opts ...PinOption,
) error {
	if headSHA == "" {
		return nil
	}

	var pinOpts pinOptions
	for _, opt := range opts {
		opt(&pinOpts)
	}

	metadata := newMetadata(pinOpts.templateName, pinOpts.templateVersion, pinOpts.source, headSHA, nil)
	if err := storage.NewMetadataStorage(metadataPath).Save(metadata); err != nil {
		return fmt.Errorf("%w: pin initial base ref: %w", errUtils.ErrMetadataSave, err)
	}
	return nil
}

// ResolveDefaultBaseRef fills in the 3-way-merge base ref used by `--update`
// when the caller didn't supply --base-ref explicitly (which always wins
// when set). It prefers the ref pinned at targetDir by PinInitialBaseRef/
// PinInitialBaseRefForInit -- the commit that actually contains this
// project's pristine generated content -- over live HEAD. Without a pin,
// --update always diffs against whatever HEAD happens to be by the time it
// runs; once a customization is committed, that makes it indistinguishable
// from the unmodified base, and the merge silently lets the freshly
// rendered template overwrite it. Falling back to plain "HEAD" (pre-fix
// targets with no pin, or a non-git target) still fixes the original bug
// this guards against: with no baseRef at all, --update silently sets up no
// git storage, and every file fails with an opaque "three-way merge
// failed".
//
// A genuinely unreadable metadata file (corrupt YAML, permission denied --
// anything other than the file simply not existing yet) is surfaced as an
// error instead of silently falling back to "HEAD": swallowing it would
// defeat the whole point of the pin, quietly reintroducing the
// silent-overwrite bug the first time the pin file itself is damaged. The
// storage.MetadataStorage.Load method returns (nil, nil) specifically when
// the file is absent, so that case alone still falls through to the
// HEAD/pin logic below.
//
// The metadataPath parameter is the caller's own pinned-metadata location
// (see storage.ScaffoldMetadataPath/storage.InitMetadataPath) -- shared here
// so `atmos scaffold generate --update` and `atmos init --update`, which pin
// and resolve base refs identically but keep separate metadata, can't drift
// apart the way they did before.
func ResolveDefaultBaseRef(baseRef, targetDir, metadataPath string) (string, error) {
	defer perf.Track(nil, "generator.ResolveDefaultBaseRef")()

	if baseRef != "" {
		return baseRef, nil
	}
	metadata, err := storage.NewMetadataStorage(metadataPath).Load()
	if err != nil {
		return "", fmt.Errorf("resolve default --base-ref from %s: %w", targetDir, err)
	}
	if metadata != nil && metadata.BaseRef != "" {
		return metadata.BaseRef, nil
	}
	return "HEAD", nil
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
