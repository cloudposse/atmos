package install

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/downloader"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time sentinels for the schema fields the tests set.
var (
	_ = schema.Vendor{Identity: "x"}
	_ = schema.AtmosVendorSource{Identity: "x"}
	_ = schema.VendorComponentSource{Identity: "x"}
)

const (
	// The fake remote cannot be resolved: a fetch of it only succeeds through the identity's
	// `insteadOf` rewrite, delivered in the per-fetch environment.
	fakeRemote = "git::https://git.example.invalid/acme/modules.git"
	markerFile = "owner.txt"
)

func localGitRepo(t *testing.T, owner string) string {
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
	require.NoError(t, os.WriteFile(filepath.Join(dir, markerFile), []byte(owner), 0o600))
	run("add", markerFile)
	run("-c", "user.name=test", "-c", "user.email=test@example.com", "-c", "commit.gpgsign=false", "commit", "--quiet", "-m", "init")
	repoPath := filepath.ToSlash(dir)
	if !strings.HasPrefix(repoPath, "/") {
		repoPath = "/" + repoPath
	}
	return (&url.URL{Scheme: "file", Path: repoPath}).String()
}

// registerIdentityRepos registers a resolver that gives each identity an environment rewriting the
// fake remote to that identity's own repository, and records which identities were resolved.
func registerIdentityRepos(t *testing.T, repos map[string]string) (resolved func() []string) {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(t.TempDir(), "gitconfig"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_TERMINAL_PROMPT", "0")

	var mu sync.Mutex
	var names []string
	downloader.RegisterIdentityEnvironmentResolver(func(_ context.Context, _ *schema.AtmosConfiguration, identity string) ([]string, error) {
		mu.Lock()
		names = append(names, identity)
		mu.Unlock()
		repo, ok := repos[identity]
		if !ok {
			return nil, fmt.Errorf("%w: %q", errUtils.ErrIdentityNotFound, identity)
		}
		return []string{
			"GIT_CONFIG_COUNT=1",
			"GIT_CONFIG_KEY_0=url." + repo + ".insteadOf",
			"GIT_CONFIG_VALUE_0=https://git.example.invalid/acme/modules.git",
		}, nil
	})
	t.Cleanup(func() { downloader.RegisterIdentityEnvironmentResolver(nil) })
	return func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), names...)
	}
}

func fetchOwner(ctx context.Context, t *testing.T, atmosConfig *schema.AtmosConfiguration, entryIdentity string) (string, error) {
	t.Helper()
	dir, _, err := fetchToTempDir(ctx, atmosConfig, fakeRemote, PkgTypeRemote, filepath.Join(t.TempDir(), "dst"), fetchOptions{
		ClientMode: downloader.ClientModeDir,
		Identity:   entryIdentity,
	})
	if err != nil {
		return "", err
	}
	got, err := os.ReadFile(filepath.Join(dir, markerFile))
	return string(got), err
}

func TestFetchToTempDir_SourceIdentityEnvironmentReachesGit(t *testing.T) {
	registerIdentityRepos(t, map[string]string{"ops": localGitRepo(t, "ops-repo")})

	got, err := fetchOwner(context.Background(), t, &schema.AtmosConfiguration{}, "ops")

	require.NoError(t, err)
	assert.Equal(t, "ops-repo", got)
}

func TestFetchToTempDir_SourceWithoutIdentityBehavesAsBefore(t *testing.T) {
	resolved := registerIdentityRepos(t, map[string]string{"ops": localGitRepo(t, "ops-repo")})
	direct := localGitRepo(t, "direct-repo")

	dir, _, err := fetchToTempDir(context.Background(), &schema.AtmosConfiguration{}, "git::"+direct, PkgTypeRemote, filepath.Join(t.TempDir(), "dst"), fetchOptions{ClientMode: downloader.ClientModeDir})

	require.NoError(t, err)
	got, err := os.ReadFile(filepath.Join(dir, markerFile))
	require.NoError(t, err)
	assert.Equal(t, "direct-repo", string(got))
	assert.Empty(t, resolved(), "no identity applies, so the resolver is never consulted")
}

func TestFetchToTempDir_IdentityPrecedence(t *testing.T) {
	repos := map[string]string{
		"flag":    localGitRepo(t, "flag-repo"),
		"entry":   localGitRepo(t, "entry-repo"),
		"default": localGitRepo(t, "default-repo"),
	}
	tests := []struct {
		name     string
		override string
		entry    string
		section  string
		want     string
	}{
		{name: "section default applies to a source with no identity", section: "default", want: "default-repo"},
		{name: "the source's own identity beats the section default", entry: "entry", section: "default", want: "entry-repo"},
		{name: "the --identity flag beats both", override: "flag", entry: "entry", section: "default", want: "flag-repo"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolved := registerIdentityRepos(t, repos)
			ctx := downloader.WithIdentityOverride(context.Background(), tt.override)
			atmosConfig := &schema.AtmosConfiguration{Vendor: schema.Vendor{Identity: tt.section}}

			dir, _, err := fetchToTempDir(ctx, atmosConfig, fakeRemote, PkgTypeRemote, filepath.Join(t.TempDir(), "dst"), fetchOptions{ClientMode: downloader.ClientModeDir, Identity: tt.entry})

			require.NoError(t, err)
			got, err := os.ReadFile(filepath.Join(dir, markerFile))
			require.NoError(t, err)
			assert.Equal(t, tt.want, string(got))
			assert.Len(t, resolved(), 1)
		})
	}
}

// Two sources of one `vendor pull` that name different identities each fetch with their own.
func TestFetchToTempDir_ConcurrentSourcesWithDifferentIdentitiesStayIsolated(t *testing.T) {
	registerIdentityRepos(t, map[string]string{
		"alpha": localGitRepo(t, "alpha-repo"),
		"bravo": localGitRepo(t, "bravo-repo"),
	})

	const perIdentity = 4
	type outcome struct {
		identity, got string
		err           error
	}
	outcomes := make(chan outcome, 2*perIdentity)
	var wg sync.WaitGroup
	for i := 0; i < perIdentity; i++ {
		for _, identity := range []string{"alpha", "bravo"} {
			wg.Add(1)
			go func() {
				defer wg.Done()
				dst := filepath.Join(t.TempDir(), "dst")
				dir, _, err := fetchToTempDir(context.Background(), &schema.AtmosConfiguration{}, fakeRemote, PkgTypeRemote, dst, fetchOptions{ClientMode: downloader.ClientModeDir, Identity: identity})
				if err != nil {
					outcomes <- outcome{identity: identity, err: err}
					return
				}
				got, err := os.ReadFile(filepath.Join(dir, markerFile))
				outcomes <- outcome{identity: identity, got: string(got), err: err}
			}()
		}
	}
	wg.Wait()
	close(outcomes)

	n := 0
	for o := range outcomes {
		require.NoError(t, o.err)
		assert.Equal(t, o.identity+"-repo", o.got)
		n++
	}
	assert.Equal(t, 2*perIdentity, n)
}

func TestFetchToTempDir_IdentityNeedingLoginFailsBeforeFetching(t *testing.T) {
	downloader.RegisterIdentityEnvironmentResolver(func(context.Context, *schema.AtmosConfiguration, string) ([]string, error) {
		return nil, errUtils.Build(errUtils.ErrRemoteIdentityLoginRequired).WithHint("Run `atmos auth login --identity ops`").Err()
	})
	t.Cleanup(func() { downloader.RegisterIdentityEnvironmentResolver(nil) })

	_, _, err := fetchToTempDir(context.Background(), &schema.AtmosConfiguration{}, fakeRemote, PkgTypeRemote, filepath.Join(t.TempDir(), "dst"), fetchOptions{ClientMode: downloader.ClientModeDir, Identity: "ops"})

	require.ErrorIs(t, err, errUtils.ErrRemoteIdentityLoginRequired)
}

func TestComponentInstallerIdentity(t *testing.T) {
	assert.Equal(t, "", (&componentVendorInstaller{}).identity())
	assert.Equal(t, "ops", (&componentVendorInstaller{spec: &schema.VendorComponentSpec{Source: schema.VendorComponentSource{Identity: "ops"}}}).identity())
}
