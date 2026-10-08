package initcmd

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/generator/templates"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestRepositoryDirectorySource(t *testing.T) {
	for _, tc := range []struct {
		repository, directory, ref, want string
	}{
		{"", "examples/demo", "", "github.com/cloudposse/atmos//examples/demo?ref=main"},
		{schema.DefaultInitRepository, "examples/demo", "v1", "github.com/cloudposse/atmos//examples/demo?ref=v1"},
		{"github.com/acme/starters", "examples/demo", "", "github.com/acme/starters//examples/demo"},
		{"github.com/acme/starters?ref=stable", "examples/demo", "ignored", "github.com/acme/starters//examples/demo?ref=stable"},
		{"github.com/acme/starters?ref=stable", "examples/demo?ref=explicit", "ignored", "github.com/acme/starters//examples/demo?ref=explicit"},
		{"git::ssh://git@example.com/org/repo.git", "templates/demo", "feature/demo", "git::ssh://git@example.com/org/repo.git//templates/demo?ref=feature%2Fdemo"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			got, err := repositoryDirectorySource(tc.repository, tc.directory, tc.ref)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
	for _, invalid := range []string{"./local", "github.com/acme/starters//nested"} {
		_, err := repositoryDirectorySource(invalid, "examples/demo", "")
		assert.Error(t, err)
	}
}

func TestRepositoryDirectorySourceRejectsMalformedQuery(t *testing.T) {
	for _, tc := range []struct{ repository, directory string }{
		{"github.com/acme/starters?ref=%zz", "examples/demo"},
		{"github.com/acme/starters", "examples/demo?ref=%zz"},
	} {
		result, err := repositoryDirectorySource(tc.repository, tc.directory, "")
		require.Error(t, err)
		assert.Empty(t, result, "malformed revisions must not fall back to the default branch")
	}
}

func TestInitDefaultsReuseResolvedConfiguration(t *testing.T) {
	t.Chdir(t.TempDir())
	require.NoError(t, os.WriteFile("atmos.yaml", []byte("invalid: ["), 0o600))
	resolved := &schema.AtmosConfiguration{Init: schema.InitConfig{Repository: "github.com/acme/examples", Ref: "selected-profile", Depth: 4}}
	SetAtmosConfig(resolved)
	t.Cleanup(func() { SetAtmosConfig(nil) })
	v := viper.New()
	got, err := applyInitDefaults(v)
	require.NoError(t, err, "defaults must reuse the resolved config, without reading atmos.yaml again")
	assert.Equal(t, resolved.Init, got.Init)
	assert.Equal(t, "selected-profile", v.GetString("ref"))
	assert.Equal(t, 4, v.GetInt("depth"))
}

func TestInitPropagatesConfigurationErrorsBeforeFetch(t *testing.T) {
	t.Chdir(t.TempDir())
	require.NoError(t, os.WriteFile("atmos.yaml", []byte("invalid: ["), 0o600))
	SetAtmosConfig(nil)
	t.Cleanup(func() { SetAtmosConfig(nil) })
	config, err := applyInitDefaults(viper.New())
	require.Error(t, err)
	assert.Nil(t, config)
	opts := &initOptions{templateName: "examples/demo", copy: true}
	require.Error(t, expandInitRepository(opts))
	selected := &templates.Configuration{Source: "github.com/acme/starters"}
	prepared, err := prepareInitSource(opts, selected, nil)
	require.Error(t, err)
	assert.Nil(t, prepared)
}

func TestNormalizeInitArgumentConfiguredRepository(t *testing.T) {
	t.Chdir(t.TempDir())
	require.NoError(t, os.WriteFile("atmos.yaml", []byte("init:\n  repository: github.com/acme/starters?ref=stable\n"), 0o600))
	configs := map[string]templates.Configuration{"aws/app": {Name: "aws/app"}}
	for _, tc := range []struct {
		input, want string
	}{
		{"examples/demo", "git::https://github.com/acme/starters.git//examples/demo?ref=stable"},
		{"templates/demo", "git::https://github.com/acme/starters.git//templates/demo?ref=stable"},
		{"github.com/cloudposse/atmos//examples/demo", "git::https://github.com/cloudposse/atmos.git//examples/demo"},
		{"./examples/demo", "./examples/demo"},
		{"aws/app", "aws/app"},
	} {
		opts := &initOptions{templateName: tc.input}
		require.NoError(t, normalizeInitArgument(opts, configs))
		assert.Equal(t, tc.want, opts.templateName)
	}
	require.NoError(t, os.MkdirAll(filepath.Join("examples", "demo"), 0o755))
	opts := &initOptions{templateName: "examples/demo"}
	require.NoError(t, normalizeInitArgument(opts, configs))
	assert.Contains(t, opts.templateName, "github.com/acme/starters", "examples shorthand must not be shadowed by a local examples directory")
}

func TestInitDefaultsPrecedence(t *testing.T) {
	cmd := &cobra.Command{}
	initParser.RegisterFlags(cmd)
	v := viper.New()
	require.NoError(t, initParser.BindFlagsToViper(cmd, v))
	disabled := false
	bindInitDefaults(v, schema.InitConfig{Ref: "configured", Depth: 3, Git: &disabled})
	assert.Equal(t, "configured", v.GetString("ref"))
	assert.False(t, v.GetBool("git"))
	assert.Equal(t, 3, v.GetInt("depth"))
	t.Setenv("ATMOS_INIT_DEPTH", "2")
	assert.Equal(t, 2, v.GetInt("depth"))
	require.NoError(t, cmd.Flags().Set("depth", "0"))
	assert.Zero(t, v.GetInt("depth"), "explicit zero requests full history")
	t.Setenv("ATMOS_INIT_REF", "environment")
	t.Setenv("ATMOS_INIT_GIT", "true")
	assert.Equal(t, "environment", v.GetString("ref"))
	assert.True(t, v.GetBool("git"))
	require.NoError(t, cmd.Flags().Set("ref", "flag"))
	require.NoError(t, cmd.Flags().Set("git", "false"))
	assert.Equal(t, "flag", v.GetString("ref"))
	assert.False(t, v.GetBool("git"))
}

func TestInitRunEConfiguredDefaults(t *testing.T) {
	t.Cleanup(viper.Reset)
	t.Chdir(t.TempDir())
	require.NoError(t, os.WriteFile("atmos.yaml", []byte("init:\n  repository: github.com/acme/starters\n  ref: stable\n  git: false\n"), 0o600))
	src := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(src, "file"), []byte("raw"), 0o600))
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	initParser.RegisterFlags(cmd)
	target := filepath.Join(t.TempDir(), "result")
	require.NoError(t, initCmd.RunE(cmd, []string{src, target}))
	assert.Equal(t, "stable", viper.GetString("ref"))
	assert.False(t, viper.GetBool("git"))
	assert.FileExists(t, filepath.Join(target, "file"))
	assert.NoDirExists(t, filepath.Join(target, ".git"))
}

func TestInitRunEWithoutConfiguration(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	workDir := t.TempDir()
	t.Chdir(workDir)
	for _, key := range []string{"HOME", "USERPROFILE", "APPDATA", "ATMOS_CLI_CONFIG_PATH"} {
		t.Setenv(key, workDir)
	}
	SetAtmosConfig(nil)
	t.Cleanup(func() { SetAtmosConfig(nil) })

	// Missing atmos.yaml must use the built-in defaults without an injected config.
	defaults, err := applyInitDefaults(viper.New())
	require.NoError(t, err)
	require.Empty(t, cfg.LoadedConfigFiles(), "this test must not load a physical configuration file")
	assert.Equal(t, schema.DefaultInitRepository, defaults.Init.Repository)
	assert.Empty(t, defaults.Init.Ref)
	assert.Equal(t, 1, defaults.Init.Depth)
	require.NotNil(t, defaults.Init.Git)
	assert.True(t, *defaults.Init.Git)

	src := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(src, "file"), []byte("raw"), 0o600))
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	initParser.RegisterFlags(cmd)
	target := filepath.Join(workDir, "result")
	require.NoError(t, initCmd.RunE(cmd, []string{src, target}))
	require.Empty(t, cfg.LoadedConfigFiles())
	content, err := os.ReadFile(filepath.Join(target, "file"))
	require.NoError(t, err)
	assert.Equal(t, "raw", string(content))
	repo, err := git.PlainOpen(target)
	require.NoError(t, err)
	_, err = repo.Head()
	require.NoError(t, err, "default initialization must create the initial Git commit")
}
