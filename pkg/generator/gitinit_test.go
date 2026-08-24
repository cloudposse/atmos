package generator

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
