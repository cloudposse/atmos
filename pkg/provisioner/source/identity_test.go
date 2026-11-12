package source

import (
	"context"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/downloader"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time sentinels for the schema fields the tests set.
var (
	_ = schema.VendorComponentSource{Identity: "x"}
	_ = schema.Vendor{Identity: "x"}
)

func TestExtractSource_Identity(t *testing.T) {
	tests := []struct {
		name   string
		source any
		want   string
	}{
		{name: "map form with identity", source: map[string]any{"uri": "github.com/acme/modules//vpc", "identity": "ops"}, want: "ops"},
		{name: "map form without identity", source: map[string]any{"uri": "github.com/acme/modules//vpc"}, want: ""},
		{name: "string form has no identity", source: "github.com/acme/modules//vpc", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec, err := ExtractSource(map[string]any{"source": tt.source})
			require.NoError(t, err)
			require.NotNil(t, spec)
			assert.Equal(t, tt.want, spec.Identity)
		})
	}
}

func identityTestRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not available")
	}
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
	}
	run("init", "--quiet")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.tf"), []byte("# identity repo\n"), 0o600))
	run("add", "main.tf")
	run("-c", "user.name=test", "-c", "user.email=test@example.com", "-c", "commit.gpgsign=false", "commit", "--quiet", "-m", "init")
	repoPath := filepath.ToSlash(dir)
	if !strings.HasPrefix(repoPath, "/") {
		repoPath = "/" + repoPath
	}
	return (&url.URL{Scheme: "file", Path: repoPath}).String()
}

func TestDownloadSource_IdentityEnvironmentReachesGit(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(t.TempDir(), "gitconfig"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_TERMINAL_PROMPT", "0")
	repo := identityTestRepo(t)
	const remote = "git::https://git.example.invalid/acme/modules.git"
	var resolved []string
	downloader.RegisterIdentityEnvironmentResolver(func(_ context.Context, _ *schema.AtmosConfiguration, identity string) ([]string, error) {
		resolved = append(resolved, identity)
		return []string{"GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=url." + repo + ".insteadOf", "GIT_CONFIG_VALUE_0=https://git.example.invalid/acme/modules.git"}, nil
	})
	t.Cleanup(func() { downloader.RegisterIdentityEnvironmentResolver(nil) })

	tests := []struct {
		name     string
		spec     *schema.VendorComponentSource
		section  string
		resolved []string
	}{
		{name: "source identity", spec: &schema.VendorComponentSource{Identity: "ops"}, section: "default", resolved: []string{"ops"}},
		{name: "vendor.identity default", spec: &schema.VendorComponentSource{}, section: "default", resolved: []string{"default"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolved = nil
			dest := filepath.Join(t.TempDir(), "dst")
			atmosConfig := &schema.AtmosConfiguration{Vendor: schema.Vendor{Identity: tt.section}}

			err := downloadSource(context.Background(), atmosConfig, tt.spec, remote, dest)

			require.NoError(t, err)
			assert.FileExists(t, filepath.Join(dest, "main.tf"))
			assert.Equal(t, tt.resolved, resolved)
		})
	}
}

func TestDownloadSource_IdentityNeedingLoginFails(t *testing.T) {
	downloader.RegisterIdentityEnvironmentResolver(func(context.Context, *schema.AtmosConfiguration, string) ([]string, error) {
		return nil, errUtils.ErrRemoteIdentityLoginRequired
	})
	t.Cleanup(func() { downloader.RegisterIdentityEnvironmentResolver(nil) })

	err := downloadSource(context.Background(), &schema.AtmosConfiguration{}, &schema.VendorComponentSource{Identity: "ops"}, "git::https://git.example.invalid/acme/modules.git", filepath.Join(t.TempDir(), "dst"))

	require.ErrorIs(t, err, errUtils.ErrRemoteIdentityLoginRequired)
}
