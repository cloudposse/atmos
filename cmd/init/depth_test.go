package initcmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/generator/source"
	"github.com/cloudposse/atmos/pkg/generator/templates"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestPrepareInitDirectoryGitDepth(t *testing.T) {
	requireGitBinaryForInitRenderedE2E(t)
	src := t.TempDir()
	repo, err := git.PlainInit(src, false)
	require.NoError(t, err)
	wt, err := repo.Worktree()
	require.NoError(t, err)
	for i := range 3 {
		require.NoError(t, os.WriteFile(filepath.Join(src, "file"), []byte(strconv.Itoa(i)), 0o600))
		require.NoError(t, wt.AddGlob("."))
		_, err = wt.Commit("change", &git.CommitOptions{Author: &object.Signature{Name: "Test", Email: "test@example.com"}})
		require.NoError(t, err)
	}
	for _, tc := range []struct {
		name, query string
		depth, want int
	}{
		{"shallow", "", 1, 1},
		{"full", "", 0, 3},
		{"source-override", "?depth=2", 1, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opts := &initOptions{atmosConfig: &schema.AtmosConfiguration{Init: schema.InitConfig{Depth: tc.depth}}}
			prepared, err := prepareInitDirectory(opts, &templates.Configuration{}, source.InitSource{
				Source: "git::" + initRenderedE2EFileURI(src) + tc.query, Name: "example",
			})
			require.NoError(t, err)
			defer prepared.cleanup()
			cloned, err := git.PlainOpen(prepared.directory.Path)
			require.NoError(t, err)
			shallow, err := cloned.Storer.Shallow()
			require.NoError(t, err)
			if tc.depth == 0 {
				assert.Empty(t, shallow)
			} else {
				assert.NotEmpty(t, shallow)
			}
			// Native Git understands the shallow boundary when counting commits.
			assertGitCommitCount(t, prepared.directory.Path, tc.want)
		})
	}
}

func TestInitRejectsNegativeDepth(t *testing.T) {
	t.Cleanup(viper.Reset)
	t.Chdir(t.TempDir())
	cmd := &cobra.Command{}
	initParser.RegisterFlags(cmd)
	require.NoError(t, cmd.Flags().Set("depth", "-1"))
	dest := filepath.Join(t.TempDir(), "target")
	err := initCmd.RunE(cmd, []string{t.TempDir(), dest})
	require.ErrorIs(t, err, errUtils.ErrInvalidFlagValue)
	assert.NoDirExists(t, dest)
}

func TestNamedTemplateDepth(t *testing.T) {
	opts := &initOptions{atmosConfig: &schema.AtmosConfiguration{Init: schema.InitConfig{Depth: 2}}}
	selected := &templates.Configuration{Source: "github.com/acme/templates//starter?ref=main"}
	require.NoError(t, applyTemplateDepth(opts, selected))
	assert.Contains(t, selected.Source, "depth=2")
	selected.Source = "github.com/acme/templates//starter?depth=0&ref=main"
	require.NoError(t, applyTemplateDepth(opts, selected))
	assert.Contains(t, selected.Source, "depth=0")
}

func assertGitCommitCount(t *testing.T, dir string, want int) {
	t.Helper()
	cmd := exec.Command("git", "-C", dir, "rev-list", "--count", "HEAD")
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", output)
	assert.Equal(t, strconv.Itoa(want), strings.TrimSpace(string(output)))
}
