package source

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/config/homedir"
	"github.com/cloudposse/atmos/pkg/downloader"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestAdHocBundledLifecycle(t *testing.T) {
	e, _ := fixture(t)
	e.AdHoc = true
	e.Config.AI.Skills = map[string]*schema.AISkillConfig{"ad-hoc:atmos-terraform": {Source: "bundled:atmos-terraform", Clients: []string{"gemini"}, Scope: scopeUser}}
	run(t, e, Options{})
	config, err := e.AdHocConfig("")
	require.NoError(t, err)
	require.Equal(t, "bundled:atmos-terraform", config.AI.Skills["ad-hoc:atmos-terraform"].Source)
	owner, err := e.ResolveInstalled("atmos-terraform", scopeUser)
	require.NoError(t, err)
	require.Equal(t, "ad-hoc:atmos-terraform", owner)
	installed, err := e.InstalledConfig("")
	require.NoError(t, err)
	require.Len(t, installed.AI.Skills, 1)
	statuses, err := e.Status(context.Background(), Options{})
	require.NoError(t, err)
	require.Len(t, statuses, 2)
	e.Config = config
	run(t, e, Options{Update: true})
	run(t, e, Options{Uninstall: true})
	owner, err = e.ResolveInstalled("atmos-terraform", scopeUser)
	require.NoError(t, err)
	require.Empty(t, owner)
}

func TestStatusAndRemovedDeclaration(t *testing.T) {
	e, _ := fixture(t)
	statuses, err := e.Status(context.Background(), Options{})
	require.NoError(t, err)
	require.Equal(t, "stale", statuses[0].Status)
	run(t, e, Options{})
	statuses, err = e.Status(context.Background(), Options{})
	require.NoError(t, err)
	require.Len(t, statuses, 3)
	for _, status := range statuses {
		require.Equal(t, "current", status.Status)
	}
	path, _ := e.destination(scopeProject, "gemini", "demo")
	write(t, filepath.Join(path, "mine.txt"), "local")
	path, _ = e.destination(scopeProject, "claude-code", "demo")
	require.NoError(t, os.RemoveAll(path))
	delete(e.Config.AI.Skills, "test")
	statuses, err = e.Status(context.Background(), Options{})
	require.NoError(t, err)
	states := map[string]bool{}
	for _, status := range statuses {
		states[status.Status] = true
	}
	require.True(t, states["obsolete"])
	require.True(t, states["missing"])
	require.True(t, states["drifted"])
	_, err = e.Run(context.Background(), Options{Prune: true})
	require.ErrorIs(t, err, ErrDrift)
	run(t, e, Options{Prune: true, Force: true})
}

func TestNewEngineCanonicalRootsAndReadOnly(t *testing.T) {
	base, home := realTemp(t), realTemp(t)
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	homedir.Reset()
	t.Cleanup(homedir.Reset)
	for _, config := range []*schema.AtmosConfiguration{{BasePath: base}, {CliConfigPath: base}, {}} {
		t.Chdir(base)
		e, err := New(config)
		require.NoError(t, err)
		require.Equal(t, base, e.Project)
		require.Equal(t, home, e.Home)
		require.NoDirExists(t, filepath.Join(home, ".atmos"))
	}
}

func TestInvalidOptionsAndState(t *testing.T) {
	for _, options := range []Options{{Check: true, Update: true}, {Check: true, DryRun: true}, {Frozen: true, Update: true}, {Scope: "invalid"}} {
		e, _ := fixture(t)
		_, err := e.Run(context.Background(), options)
		require.ErrorIs(t, err, ErrInvalid)
	}
	for _, raw := range []string{`{"version":2}`, `{"version":1,"records":[null]}`, `{"version":1,"records":[{"scope":"project","name":"../escape"}]}`, `invalid`} {
		e, _ := fixture(t)
		write(t, filepath.Join(e.stateDir(scopeProject), "installations.json"), raw)
		_, err := e.loadStates()
		require.Error(t, err)
	}
	e, _ := fixture(t)
	write(t, e.lockPath(), "version: 2\n")
	_, err := e.Run(context.Background(), Options{Check: true})
	require.ErrorIs(t, err, ErrInvalid)
}

func TestRecoveryRejectsForeignAndEscapingJournals(t *testing.T) {
	e, _ := fixture(t)
	require.NoError(t, e.Recover())
	for _, j := range []journal{{Project: "somewhere-else"}, {Project: e.Project, Entries: []journalEntry{{Target: filepath.Join(e.Home, "private"), Stage: "/tmp/foreign"}}}} {
		raw, err := json.Marshal(j)
		require.NoError(t, err)
		write(t, e.journals()[0], string(raw))
		require.Error(t, e.Recover())
		require.NoError(t, os.Remove(e.journals()[0]))
	}
	j := journal{Project: e.Project, Committed: true}
	require.NoError(t, e.saveJournal(j))
	require.NoError(t, e.Recover())
	require.NoError(t, e.pending())
}

func TestStagingFailureLeavesOriginalTrees(t *testing.T) {
	e, _ := fixture(t)
	run(t, e, Options{})
	target, _ := e.destination(scopeProject, canonicalClient, "demo")
	before, err := treeDigest(target)
	require.NoError(t, err)
	err = e.apply([]operation{{Target: filepath.Join(e.Project, "new-file"), Data: []byte("staged")}, {Target: target, Source: filepath.Join(e.Project, "absent")}})
	require.Error(t, err)
	after, err := treeDigest(target)
	require.NoError(t, err)
	require.Equal(t, before, after)
	require.NoFileExists(t, filepath.Join(e.Project, "new-file"))
}

type downloadStub struct {
	fetch func(context.Context, string, string) (downloader.FetchMetadata, error)
}

func (d downloadStub) FetchWithMetadataContext(ctx context.Context, src, dest string, _ downloader.ClientMode, _ time.Duration) (downloader.FetchMetadata, error) {
	return d.fetch(ctx, src, dest)
}

func TestGitFetcherUsesRequestedRefAndExactCommit(t *testing.T) {
	commit := strings.Repeat("a", 40)
	for _, repo := range []Repository{{Source: "org/repo", Ref: "main"}, {Source: "https://git.example/repo", Ref: "v1"}, {Source: "git@git.example:org/repo", Commit: commit}} {
		fetch := &GitFetcher{Config: &schema.AtmosConfiguration{}, Downloader: downloadStub{fetch: func(ctx context.Context, src, dest string) (downloader.FetchMetadata, error) {
			require.NotNil(t, ctx)
			require.Contains(t, src, "git::")
			require.Contains(t, src, "ref=")
			if repo.Commit != "" {
				require.Contains(t, src, commit)
			} else {
				require.Contains(t, src, repo.Ref)
			}
			return downloader.FetchMetadata{GitCommit: commit}, nil
		}}}
		resolved, err := fetch.Fetch(context.Background(), repo, filepath.Join(realTemp(t), "fetch"))
		require.NoError(t, err)
		require.Equal(t, commit, resolved.Commit)
	}
}

func TestGitFetcherRejectsCredentialsMismatchAndDownloadFailure(t *testing.T) {
	f := &GitFetcher{Config: &schema.AtmosConfiguration{}, Downloader: downloadStub{fetch: func(context.Context, string, string) (downloader.FetchMetadata, error) {
		return downloader.FetchMetadata{GitCommit: strings.Repeat("b", 40)}, nil
	}}}
	_, err := f.Fetch(context.Background(), Repository{Source: "https://user:secret@git.example/repo"}, realTemp(t))
	require.ErrorIs(t, err, ErrInvalid)
	_, err = f.Fetch(context.Background(), Repository{Source: "org/repo", Commit: strings.Repeat("a", 40)}, realTemp(t))
	require.ErrorIs(t, err, ErrDrift)
	failure := errors.New("download failed")
	f.Downloader = downloadStub{fetch: func(context.Context, string, string) (downloader.FetchMetadata, error) {
		return downloader.FetchMetadata{}, failure
	}}
	_, err = f.Fetch(context.Background(), Repository{Source: "org/repo"}, realTemp(t))
	require.ErrorIs(t, err, failure)
}

func TestPluginCustomPathsAndStrictConflict(t *testing.T) {
	e, repo := fixture(t)
	write(t, filepath.Join(repo, ".claude-plugin", "marketplace.json"), `{"plugins":[{"name":"custom","source":".","skills":["./extra/"]}]}`)
	write(t, filepath.Join(repo, ".claude-plugin", "plugin.json"), `{"name":"custom","skills":"./more/"}`)
	for _, name := range []string{"extra", "more"} {
		write(t, filepath.Join(repo, name, name, "SKILL.md"), "---\nname: "+name+"\ndescription: Example\n---\n# Example\n")
	}
	run(t, e, Options{})
	var lock Lock
	require.NoError(t, readYAML(e.lockPath(), &lock))
	require.Len(t, lock.Tracks["default"]["test"].Skills, 3)
	write(t, filepath.Join(repo, ".claude-plugin", "marketplace.json"), `{"plugins":[{"name":"custom","source":".","strict":false,"skills":["./extra/"]}]}`)
	_, err := e.Run(context.Background(), Options{Update: true})
	require.ErrorContains(t, err, "conflicting")
}

func TestInvalidDeclarationsAndDuplicateNames(t *testing.T) {
	for _, d := range []schema.AISkillConfig{{Source: ""}, {Source: "x", Kind: "bad"}, {Source: "x", Scope: "bad"}, {Source: "x", SystemPrompt: "mixed"}} {
		normalized := normalize(&d)
		require.Error(t, validateDeclaration(&normalized))
	}
	e, repo := fixture(t)
	e.Config.AI.Skills["second"] = &schema.AISkillConfig{Source: repo, Clients: []string{"claude-code"}}
	_, err := e.Run(context.Background(), Options{})
	require.ErrorContains(t, err, "colliding")
	delete(e.Config.AI.Skills, "second")
	e.Config.AI.Skills["test"].Include = []string{"[invalid"}
	_, err = e.Run(context.Background(), Options{})
	require.Error(t, err)
	e.Config.AI.Skills["test"].Include = nil
	_, err = e.Run(context.Background(), Options{Name: "unknown"})
	require.ErrorContains(t, err, "not selected")
}

func TestLocalURLAndContentSelection(t *testing.T) {
	e, repo := fixture(t)
	e.Config.AI.Skills["test"].Source = "file://" + repo
	run(t, e, Options{})
	raw, err := os.ReadFile(e.lockPath())
	require.NoError(t, err)
	var lock Lock
	require.NoError(t, yaml.Unmarshal(raw, &lock))
	require.Empty(t, lock.Tracks["default"]["test"].Repositories[0].Commit)
}

func TestManualDestinationUsesOwnedTransactions(t *testing.T) {
	e, repo := fixture(t)
	manual := filepath.Join(realTemp(t), "custom-skills")
	run(t, e, Options{Path: manual})
	target := filepath.Join(manual, "demo", "SKILL.md")
	require.FileExists(t, target)
	write(t, filepath.Join(repo, "skills", "demo", "new.txt"), "new")
	run(t, e, Options{Update: true})
	require.FileExists(t, filepath.Join(manual, "demo", "new.txt"))
	states, err := e.loadStates()
	require.NoError(t, err)
	manualRecord := false
	for _, r := range states["project"].Records {
		if r.Path == filepath.Dir(target) {
			manualRecord = r.ManualRoot == manual
		}
	}
	require.True(t, manualRecord)
	write(t, target, "changed by user")
	_, err = e.Run(context.Background(), Options{Uninstall: true})
	require.ErrorIs(t, err, ErrDrift)
	run(t, e, Options{Uninstall: true, Force: true})
	require.NoDirExists(t, filepath.Dir(target))
	write(t, target, "unowned")
	_, err = e.Run(context.Background(), Options{Path: manual, Force: true})
	require.ErrorIs(t, err, ErrOwnership)
}

func TestAdHocGenericSourcesAndCredentialRejection(t *testing.T) {
	for _, raw := range []string{"https://git.example/team/repo.git", "ssh://git@git.example/team/repo", "git@git.example:team/repo", "org/repo@v1"} {
		d, err := ParseAdHoc(raw)
		require.NoError(t, err)
		require.NotEmpty(t, d.Source)
	}
	for _, raw := range []string{"https://user:secret@git.example/repo", "ssh://git:secret@git.example/repo", "https://git.example/repo?token=secret", "https://git.example/repo#secret"} {
		_, err := ParseAdHoc(raw)
		require.ErrorIs(t, err, ErrInvalid)
		require.NotContains(t, err.Error(), "secret")
	}
}
