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
