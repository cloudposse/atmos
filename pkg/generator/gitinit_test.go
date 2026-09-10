package generator

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/generator/storage"
)

func TestInitGitRepository_CreatesInitialCommit(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("# demo\n"), 0o600))

	skipped, headSHA, err := InitGitRepository(InitGitOptions{
		TargetPath:      dir,
		TemplateName:    "basic",
		TemplateVersion: "1.0.0",
	})
	require.NoError(t, err)
	assert.False(t, skipped)
	require.NotEmpty(t, headSHA)

	repo, err := git.PlainOpen(dir)
	require.NoError(t, err)
	head, err := repo.Head()
	require.NoError(t, err)
	assert.Equal(t, head.Hash().String(), headSHA, "returned headSHA must match the actual commit created")
	commit, err := repo.CommitObject(head.Hash())
	require.NoError(t, err)
	assert.Equal(t, "Initial commit from atmos init (basic@1.0.0)", commit.Message)

	wt, err := repo.Worktree()
	require.NoError(t, err)
	status, err := wt.Status()
	require.NoError(t, err)
	assert.True(t, status.IsClean(), "generated repository should be clean after initial commit")
}

func TestInitGitRepository_SkipsInsideExistingRepo(t *testing.T) {
	root := t.TempDir()
	_, err := git.PlainInit(root, false)
	require.NoError(t, err)
	child := filepath.Join(root, "generated")
	require.NoError(t, os.MkdirAll(child, 0o755))

	skipped, headSHA, err := InitGitRepository(InitGitOptions{TargetPath: child, TemplateName: "basic"})
	require.NoError(t, err)
	assert.True(t, skipped)
	assert.Empty(t, headSHA, "no commit was created, so there is nothing to pin")
	_, statErr := os.Stat(filepath.Join(child, ".git"))
	assert.True(t, os.IsNotExist(statErr), "nested target should not get its own .git")
}

// TestInitGitRepository_EmptyTargetPathReturnsError reproduces the
// InitGitOptions.TargetPath validation guard: InitGitRepository must reject
// an empty target path up front, before ever touching git.PlainInit, rather
// than letting go-git fail with an opaque error about the current directory.
func TestInitGitRepository_EmptyTargetPathReturnsError(t *testing.T) {
	skipped, headSHA, err := InitGitRepository(InitGitOptions{TargetPath: ""})

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrGitTargetPathInvalid)
	assert.False(t, skipped)
	assert.Empty(t, headSHA)
}

// TestInitGitRepository_PlainInitErrorWraps reproduces a corrupt/blocked
// .git path (e.g. a leftover regular file named ".git" instead of a
// directory -- something a prior interrupted `git init` or manual copy can
// leave behind): git.PlainInit fails, and InitGitRepository must wrap that
// failure as ErrGitWorkdirNotInitialized instead of silently swallowing it
// or panicking on the nil *Repository.
func TestInitGitRepository_PlainInitErrorWraps(t *testing.T) {
	dir := t.TempDir()
	// A regular file (not a directory) at .git blocks git.PlainInit from
	// creating the real git directory structure underneath it, while still
	// letting isInsideGitRepository correctly report false (it's not a valid
	// git dir, just a name collision), so InitGitRepository proceeds to
	// PlainInit and observes the failure.
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".git"), []byte("blocker"), 0o600))

	skipped, headSHA, err := InitGitRepository(InitGitOptions{TargetPath: dir, TemplateName: "basic"})

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrGitWorkdirNotInitialized)
	assert.False(t, skipped)
	assert.Empty(t, headSHA)
}

// TestInitGitRepository_CommitErrorWraps reproduces generating into a
// directory with zero files: wt.AddGlob(".") still succeeds (it always
// matches the root "." itself, even when empty), but wt.Commit then fails
// with "nothing to commit" against a clean working tree. InitGitRepository
// must surface that as ErrGitArtifactWrite rather than returning a bogus
// empty-but-successful headSHA that PinInitialBaseRef would go on to persist.
func TestInitGitRepository_CommitErrorWraps(t *testing.T) {
	dir := t.TempDir() // Intentionally empty: no files for the commit to include.

	skipped, headSHA, err := InitGitRepository(InitGitOptions{TargetPath: dir, TemplateName: "basic"})

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrGitArtifactWrite)
	assert.False(t, skipped)
	assert.Empty(t, headSHA)
}

func TestInitialCommitMessage_NoVersion(t *testing.T) {
	assert.Equal(t, "Initial commit from atmos init (basic)", initialCommitMessage("basic", ""))
}

func TestPinInitialBaseRef_WritesMetadata(t *testing.T) {
	dir := t.TempDir()

	require.NoError(t, PinInitialBaseRef(
		dir, "abc123",
		WithTemplateName("basic"),
		WithTemplateVersion("1.0.0"),
		WithSource("embedded"),
	))

	metadata, err := storage.NewMetadataStorage(storage.ScaffoldMetadataPath(dir)).Load()
	require.NoError(t, err)
	require.NotNil(t, metadata)
	assert.Equal(t, "abc123", metadata.BaseRef)
	assert.Equal(t, "basic", metadata.Template.Name)
	assert.Equal(t, "1.0.0", metadata.Template.Version)
	assert.Equal(t, "embedded", metadata.Template.Source)
}

// TestPinInitialBaseRef_PropagatesSaveError reproduces a metadata write
// failure (here, a regular file blocking the .atmos/scaffold directory
// storage.MetadataStorage.Save needs to create) surfacing through
// PinInitialBaseRef as ErrMetadataSave instead of being silently swallowed
// -- a swallowed failure here would leave --update permanently unable to
// find a pin, silently falling back to the live-HEAD bug PinInitialBaseRef
// exists to fix.
func TestPinInitialBaseRef_PropagatesSaveError(t *testing.T) {
	dir := t.TempDir()
	// A regular file at .atmos blocks os.MkdirAll from creating
	// .atmos/scaffold underneath it.
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".atmos"), []byte("blocker"), 0o600))

	err := PinInitialBaseRef(dir, "abc123", WithTemplateName("basic"))

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrMetadataSave)
	assert.NoFileExists(t, storage.ScaffoldMetadataPath(dir), "the blocked write must not silently succeed")
}

func TestPinInitialBaseRef_NoopWhenSkipped(t *testing.T) {
	dir := t.TempDir()

	require.NoError(t, PinInitialBaseRef(
		dir, "",
		WithTemplateName("basic"),
		WithTemplateVersion("1.0.0"),
		WithSource("embedded"),
	))

	assert.NoFileExists(t, storage.ScaffoldMetadataPath(dir))
}

// TestPinInitialBaseRef_NoOptionsStillWritesBaseRef verifies PinInitialBaseRef
// works correctly with no options at all (all pinOptions fields left at their
// zero value) -- confirming the functional options are genuinely optional,
// not silently required for the pin itself to succeed.
func TestPinInitialBaseRef_NoOptionsStillWritesBaseRef(t *testing.T) {
	dir := t.TempDir()

	require.NoError(t, PinInitialBaseRef(dir, "def456"))

	metadata, err := storage.NewMetadataStorage(storage.ScaffoldMetadataPath(dir)).Load()
	require.NoError(t, err)
	require.NotNil(t, metadata)
	assert.Equal(t, "def456", metadata.BaseRef)
	assert.Empty(t, metadata.Template.Name)
}

// TestPinInitialBaseRefForInit_WritesInitMetadata verifies `atmos init`'s pin
// writes to .atmos/init/metadata.yaml (storage.InitMetadataPath) rather than
// the scaffold command's .atmos/scaffold/metadata.yaml -- the two commands
// must not clobber each other's pinned base ref.
func TestPinInitialBaseRefForInit_WritesInitMetadata(t *testing.T) {
	dir := t.TempDir()

	require.NoError(t, PinInitialBaseRefForInit(
		dir, "abc123",
		WithTemplateName("simple"),
		WithTemplateVersion("2.0.0"),
		WithSource("embedded"),
	))

	metadata, err := storage.NewMetadataStorage(storage.InitMetadataPath(dir)).Load()
	require.NoError(t, err)
	require.NotNil(t, metadata)
	assert.Equal(t, "abc123", metadata.BaseRef)
	assert.Equal(t, "simple", metadata.Template.Name)
	assert.Equal(t, "2.0.0", metadata.Template.Version)
	assert.Equal(t, "embedded", metadata.Template.Source)

	// Must not also write (or be confused with) the scaffold command's
	// separate metadata file at the same target directory.
	assert.NoFileExists(t, storage.ScaffoldMetadataPath(dir))
}

// TestPinInitialBaseRefForInit_NoopWhenSkipped mirrors
// TestPinInitialBaseRef_NoopWhenSkipped for the init variant: no commit means
// nothing to pin.
func TestPinInitialBaseRefForInit_NoopWhenSkipped(t *testing.T) {
	dir := t.TempDir()

	require.NoError(t, PinInitialBaseRefForInit(dir, "", WithTemplateName("simple")))

	assert.NoFileExists(t, storage.InitMetadataPath(dir))
}

// TestResolveDefaultBaseRef_ExplicitAlwaysWins verifies an explicit --base-ref
// short-circuits before ever consulting pinned metadata, regardless of what
// (if anything) is pinned at targetDir.
func TestResolveDefaultBaseRef_ExplicitAlwaysWins(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, PinInitialBaseRef(dir, "pinned-ref", WithTemplateName("basic")))

	resolved, err := ResolveDefaultBaseRef("v1.2.3", dir, storage.ScaffoldMetadataPath(dir))

	require.NoError(t, err)
	assert.Equal(t, "v1.2.3", resolved)
}

// TestResolveDefaultBaseRef_FallsBackToHEADWithNoPin reproduces the original
// bug fix: with no --base-ref and no pinned metadata (a pre-fix target, or one
// that was never git-initialized), --update must still get a usable base ref
// instead of silently setting up no git storage at all.
func TestResolveDefaultBaseRef_FallsBackToHEADWithNoPin(t *testing.T) {
	dir := t.TempDir()

	resolved, err := ResolveDefaultBaseRef("", dir, storage.ScaffoldMetadataPath(dir))

	require.NoError(t, err)
	assert.Equal(t, "HEAD", resolved)
}

// TestResolveDefaultBaseRef_PrefersPinnedMetadataOverHEAD verifies the actual
// fix: once a pin exists, it wins over live HEAD so a customization committed
// after generation doesn't silently become indistinguishable from the
// unmodified base.
func TestResolveDefaultBaseRef_PrefersPinnedMetadataOverHEAD(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, PinInitialBaseRef(dir, "pinned-sha", WithTemplateName("basic")))

	resolved, err := ResolveDefaultBaseRef("", dir, storage.ScaffoldMetadataPath(dir))

	require.NoError(t, err)
	assert.Equal(t, "pinned-sha", resolved)
}

// TestResolveDefaultBaseRef_PropagatesLoadError verifies a genuinely
// unreadable metadata file (corrupt YAML here) surfaces as an error instead
// of silently falling back to "HEAD" -- swallowing it would quietly
// reintroduce the silent-overwrite bug the first time the pin file itself is
// damaged.
func TestResolveDefaultBaseRef_PropagatesLoadError(t *testing.T) {
	dir := t.TempDir()
	metadataPath := storage.ScaffoldMetadataPath(dir)
	require.NoError(t, os.MkdirAll(filepath.Dir(metadataPath), 0o755))
	require.NoError(t, os.WriteFile(metadataPath, []byte("not: valid: yaml: ["), 0o600))

	resolved, err := ResolveDefaultBaseRef("", dir, metadataPath)

	require.Error(t, err)
	assert.Empty(t, resolved)
	assert.NotEqual(t, "HEAD", resolved)
}
