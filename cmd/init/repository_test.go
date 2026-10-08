package initcmd

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
