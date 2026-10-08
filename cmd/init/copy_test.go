package initcmd

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/generator/templates"
)

func TestExecuteInitCopiesDirectory(t *testing.T) {
	for _, gitEnabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "no-git", true: "git"}[gitEnabled], func(t *testing.T) {
			src := filepath.Join(t.TempDir(), "example")
			require.NoError(t, os.Mkdir(src, 0o755))
			readme := "# Example\n\nLiteral {{ .NotATemplate }} and {{ invalid syntax }}\n"
			require.NoError(t, os.WriteFile(filepath.Join(src, "README.md"), []byte(readme), 0o600))
			t.Chdir(t.TempDir())
			err := executeInit(context.Background(), &initOptions{templateName: src, git: gitEnabled})
			require.NoError(t, err)
			got, err := os.ReadFile(filepath.Join("example", "README.md"))
			require.NoError(t, err)
			assert.Equal(t, readme, string(got))
			assert.NoFileExists(t, filepath.Join("example", ".atmos", "scaffold.yaml"))
			assert.NoFileExists(t, filepath.Join("example", ".atmos", "init", "metadata.yaml"))
			if gitEnabled {
				repo, err := git.PlainOpen("example")
				require.NoError(t, err)
				_, err = repo.Head()
				require.NoError(t, err, "initial commit must exist")
			} else {
				assert.NoDirExists(t, filepath.Join("example", ".git"))
			}
		})
	}
}

func TestExecuteInitCopyManifestAndMissingREADME(t *testing.T) {
	src := t.TempDir()
	manifest := "invalid: ["
	require.NoError(t, os.WriteFile(filepath.Join(src, "scaffold.yaml"), []byte(manifest), 0o600))
	dst := filepath.Join(t.TempDir(), "target")
	opts := &initOptions{templateName: src, targetDir: dst}
	require.Error(t, executeInit(context.Background(), opts), "invalid scaffold must not silently become a copy")
	assert.NoDirExists(t, dst)
	opts.copy = true
	require.NoError(t, executeInit(context.Background(), opts))
	got, err := os.ReadFile(filepath.Join(dst, "scaffold.yaml"))
	require.NoError(t, err)
	assert.Equal(t, manifest, string(got))
}

func TestExecuteInitCopyEmptyDirectoryWithGit(t *testing.T) {
	src := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(src, "empty"), 0o755))
	dst := filepath.Join(t.TempDir(), "target")
	require.NoError(t, executeInit(context.Background(), &initOptions{templateName: src, targetDir: dst, git: true}))
	assert.DirExists(t, filepath.Join(dst, "empty"))
	repo, err := git.PlainOpen(dst)
	require.NoError(t, err)
	_, err = repo.Head()
	require.NoError(t, err)
}

func TestExecuteInitCopyRejectsScaffoldOptions(t *testing.T) {
	for _, tc := range []struct {
		name string
		set  func(*initOptions)
	}{
		{"update", func(o *initOptions) { o.update = true }},
		{"set", func(o *initOptions) { o.templateVars = map[string]interface{}{"name": "foo"} }},
		{"rendered", func(o *initOptions) { o.updateStrategy = "rendered" }},
		{"explicit-default", func(o *initOptions) { o.copyUnsupported = []string{"--merge-driver"} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dst := filepath.Join(t.TempDir(), "target")
			opts := &initOptions{templateName: t.TempDir(), targetDir: dst}
			tc.set(opts)
			err := executeInit(context.Background(), opts)
			require.ErrorIs(t, err, errUtils.ErrInvalidFlagValue)
			assert.NoDirExists(t, dst)
		})
	}
}

func TestNormalizeInitArgument(t *testing.T) {
	configs := map[string]templates.Configuration{"basic": {Name: "basic"}}
	for _, value := range []string{"examples/scaffolding", "github.com/cloudposse/atmos//examples/scaffolding"} {
		opts := &initOptions{templateName: value}
		require.NoError(t, normalizeInitArgument(opts, configs))
		assert.True(t, opts.copy, "official scaffold examples must be copied too")
	}
	opts := &initOptions{templateName: "basic"}
	require.NoError(t, normalizeInitArgument(opts, configs))
	assert.Equal(t, "basic", opts.templateName)
	assert.False(t, opts.copy)
	t.Chdir(t.TempDir())
	require.NoError(t, os.Mkdir("local", 0o755))
	opts.templateName = "local"
	require.NoError(t, normalizeInitArgument(opts, configs))
	assert.True(t, filepath.IsAbs(opts.templateName))
}

func TestExplicitScaffoldFlags(t *testing.T) {
	cmd := &cobra.Command{}
	initParser.RegisterFlags(cmd)
	v := viper.New()
	assert.Empty(t, explicitScaffoldFlags(cmd, v))
	require.NoError(t, cmd.Flags().Set("merge-driver", "auto"))
	assert.Contains(t, explicitScaffoldFlags(cmd, v), "--merge-driver")
	t.Setenv("ATMOS_INIT_SET", "name=demo")
	assert.Contains(t, explicitScaffoldFlags(cmd, v), "--set")
}

func TestCopyEnvironmentBinding(t *testing.T) {
	cmd := &cobra.Command{}
	initParser.RegisterFlags(cmd)
	v := viper.New()
	require.NoError(t, initParser.BindFlagsToViper(cmd, v))
	t.Setenv("ATMOS_INIT_COPY", "true")
	assert.True(t, v.GetBool("copy"))
	require.NoError(t, cmd.Flags().Set("copy", "false"))
	assert.False(t, v.GetBool("copy"), "explicit flag wins over environment")
}

func TestExecuteInitCopyGitSubdirectoryAtPinnedRevision(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("Git source transport requires git")
	}
	src := t.TempDir()
	repo, err := git.PlainInit(src, false)
	require.NoError(t, err)
	wt, err := repo.Worktree()
	require.NoError(t, err)
	require.NoError(t, os.Mkdir(filepath.Join(src, "example"), 0o755))
	file := filepath.Join(src, "example", "file.tmpl")
	require.NoError(t, os.WriteFile(file, []byte("{{ original }}"), 0o755))
	require.NoError(t, wt.AddGlob("."))
	first, err := wt.Commit("first", &git.CommitOptions{Author: &object.Signature{Name: "Test", Email: "test@example.com"}})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(file, []byte("changed"), 0o755))
	require.NoError(t, wt.AddGlob("."))
	_, err = wt.Commit("second", &git.CommitOptions{Author: &object.Signature{Name: "Test", Email: "test@example.com"}})
	require.NoError(t, err)
	t.Chdir(t.TempDir())
	require.NoError(t, executeInit(context.Background(), &initOptions{
		templateName: "git::" + initRenderedE2EFileURI(src) + "//example?ref=" + first.String(),
		ref:          "ignored-because-source-is-pinned",
	}))
	content, err := os.ReadFile(filepath.Join("example", "file.tmpl"))
	require.NoError(t, err)
	assert.Equal(t, "{{ original }}", string(content))
	assert.NoDirExists(t, filepath.Join("example", ".git"))
}
