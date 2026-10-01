package exec

import (
	"context"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/vendoring/install"
	"github.com/cloudposse/atmos/pkg/vendoring/lockfile"
	"github.com/cloudposse/atmos/tests"
	"github.com/cloudposse/atmos/tests/testhelpers/gitconfigenv"
)

const tripleSlashRepository = "github.com/terraform-aws-modules/terraform-aws-s3-bucket.git"

// TestVendorPullWithTripleSlashPattern tests the vendor pull command with the triple-slash pattern.
// The pattern indicates cloning from the root of a repository (e.g., github.com/repo.git///?ref=v1.0).
// This pattern was broken after go-getter v1.7.9 due to changes in subdirectory path handling.
func TestVendorPullWithTripleSlashPattern(t *testing.T) {
	fixture := newTripleSlashFixture(t)
	cmd := fixture.command(t)
	require.NoError(t, cmd.Flags().Set("component", "s3-bucket"))
	plan, err := PlanVendorPull(cmd, nil)
	require.NoError(t, err)
	require.Len(t, plan.Packages, 1)
	assert.Equal(t, install.PkgTypeRemote, plan.Packages[0].PkgType(), "the fixture must exercise a real Git fetch, not the local copy path")
	assert.Equal(t, tripleSlashRepository+"//.?ref=v5.7.0", plan.Packages[0].URI())
	require.NoError(t, ExecuteVendorPullCommand(cmd, nil))
	fixture.assertVendoredFiles(t)
}

// TestVendorPullWithMultipleVendorFiles verifies the canonical manifest and tag filter.
// A neighboring, unimported vendor-test.yaml must not contribute its matching-tag source.
func TestVendorPullWithMultipleVendorFiles(t *testing.T) {
	fixture := newTripleSlashFixture(t)
	for _, name := range []string{"vendor.yaml", "vendor-test.yaml"} {
		require.FileExists(t, filepath.Join(fixture.project, name))
	}
	cmd := fixture.command(t)
	require.NoError(t, cmd.Flags().Set("tags", "aws"))
	plan, err := PlanVendorPull(cmd, nil)
	require.NoError(t, err)
	require.Len(t, plan.Packages, 1, "only the aws-tagged source from vendor.yaml should be selected")
	assert.Equal(t, "s3-bucket", plan.Packages[0].Name)
	require.NoError(t, ExecuteVendorPullCommand(cmd, nil))
	fixture.assertVendoredFiles(t)
	require.NoDirExists(t, filepath.Join(fixture.project, "components", "terraform", "not-aws"))
	require.NoDirExists(t, filepath.Join(fixture.project, "components", "terraform", "alternate-manifest"))
	receipts, err := lockfile.Load(&plan.Config)
	require.NoError(t, err)
	require.Len(t, receipts.Artifacts, 1)
	for _, artifact := range receipts.Artifacts {
		assert.Equal(t, "s3-bucket", artifact.Name)
	}
}

type tripleSlashFixture struct {
	project      string
	taggedCommit string
	expected     map[string]string
}

func newTripleSlashFixture(t *testing.T) *tripleSlashFixture {
	t.Helper()
	tests.RequireExecutable(t, "git", "triple-slash vendoring regression")
	fixtureDir, err := filepath.Abs("../../tests/fixtures/scenarios/vendor-triple-slash")
	require.NoError(t, err)
	fixture := &tripleSlashFixture{project: t.TempDir(), expected: map[string]string{
		"main.tf": "# pinned root main\n", "outputs.tf": "# outputs\n", "variables.tf": "# variables\n", "versions.tf": "# versions\n",
		"README.md": "# Root README\n", "CHANGELOG.md": "# Changelog\n", "LICENSE": "Fixture license\n",
		"modules/notification/main.tf": "# notification main\n", "modules/notification/variables.tf": "# notification variables\n",
		"modules/notification/outputs.tf": "# notification outputs\n", "modules/notification/versions.tf": "# notification versions\n",
		"modules/notification/template.tftpl": "module template\n", "docs/README.md": "# Nested README\n",
	}}
	for _, name := range []string{"atmos.yaml", "vendor.yaml", "vendor-test.yaml"} {
		data, readErr := os.ReadFile(filepath.Join(fixtureDir, name))
		require.NoError(t, readErr)
		require.NoError(t, os.WriteFile(filepath.Join(fixture.project, name), data, 0o644))
	}
	fixture.addSelectionSources(t)
	source := fixture.localRepository(t)
	fixture.isolateEnvironment(t, source)
	t.Chdir(fixture.project)
	t.Run("local Git mirror prerequisite", func(t *testing.T) {
		output, err := exec.CommandContext(t.Context(), "git", "ls-remote", "https://"+tripleSlashRepository, "refs/tags/v5.7.0").CombinedOutput()
		require.NoError(t, err, string(output))
		require.Equal(t, fixture.taggedCommit+"\trefs/tags/v5.7.0", strings.TrimSpace(string(output)))
	})
	return fixture
}

func (f *tripleSlashFixture) addSelectionSources(t *testing.T) {
	t.Helper()
	path := filepath.Join(f.project, "vendor.yaml")
	contents, err := os.ReadFile(path)
	require.NoError(t, err)
	var manifest schema.AtmosVendorConfig
	require.NoError(t, yaml.Unmarshal(contents, &manifest))
	require.Len(t, manifest.Spec.Sources, 1)
	require.Equal(t, tripleSlashRepository+"///?ref={{.Version}}", manifest.Spec.Sources[0].Source)
	original := manifest.Spec.Sources[0]
	unrelated := original
	unrelated.Component = "not-aws"
	unrelated.Tags = []string{"azure"}
	unrelated.Targets = schema.AtmosVendorTargets{{Path: "components/terraform/not-aws"}}
	manifest.Spec.Sources = append(manifest.Spec.Sources, unrelated)
	contents, err = yaml.Marshal(manifest)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, contents, 0o644))
	alternate := original
	alternate.Component = "alternate-manifest"
	alternate.Targets = schema.AtmosVendorTargets{{Path: "components/terraform/alternate-manifest"}}
	manifest.Spec.Sources = []schema.AtmosVendorSource{alternate}
	contents, err = yaml.Marshal(manifest)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(f.project, "vendor-test.yaml"), contents, 0o644))
}

func (f *tripleSlashFixture) localRepository(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	repo, err := git.PlainInit(directory, false)
	require.NoError(t, err)
	for name, contents := range f.expected {
		path := filepath.Join(directory, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(contents), 0o644))
	}
	require.NoError(t, os.WriteFile(filepath.Join(directory, "excluded.txt"), []byte("not included\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "docs", "excluded.txt"), []byte("not included\n"), 0o644))
	tree, err := repo.Worktree()
	require.NoError(t, err)
	require.NoError(t, tree.AddWithOptions(&git.AddOptions{All: true}))
	commit, err := tree.Commit("tagged fixture", &git.CommitOptions{Author: &object.Signature{Name: "Atmos Test", Email: "test@example.com", When: time.Unix(1, 0)}})
	require.NoError(t, err)
	f.taggedCommit = commit.String()
	_, err = repo.CreateTag("v5.7.0", commit, nil)
	require.NoError(t, err)
	// Different HEAD content proves the requested ref is honored, rather than merely cloning latest.
	require.NoError(t, os.WriteFile(filepath.Join(directory, "main.tf"), []byte("# unpinned HEAD must not be installed\n"), 0o644))
	_, err = tree.Add("main.tf")
	require.NoError(t, err)
	_, err = tree.Commit("newer untagged revision", &git.CommitOptions{Author: &object.Signature{Name: "Atmos Test", Email: "test@example.com", When: time.Unix(2, 0)}})
	require.NoError(t, err)
	return directory
}

func (f *tripleSlashFixture) isolateEnvironment(t *testing.T, repository string) {
	t.Helper()
	viper.Reset()
	t.Cleanup(viper.Reset)
	gitConfig := filepath.Join(t.TempDir(), "gitconfig")
	require.NoError(t, os.WriteFile(gitConfig, []byte("[core]\n\tautocrlf = false\n"), 0o644))
	t.Setenv("GIT_CONFIG_GLOBAL", gitConfig)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	// All actual Git transports must be local. A broken redirect fails immediately, without network access.
	t.Setenv("GIT_ALLOW_PROTOCOL", "file")
	path := filepath.ToSlash(repository)
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	mirror := (&url.URL{Scheme: "file", Path: path}).String()
	overrides := map[string]string{}
	gitconfigenv.Append(overrides, os.Environ(), gitconfigenv.GitConfigEntry{Key: "url." + mirror + ".insteadOf", Value: "https://" + tripleSlashRepository})
	for key, value := range overrides {
		t.Setenv(key, value)
	}
	t.Setenv("ATMOS_CLI_CONFIG_PATH", f.project)
	t.Setenv("ATMOS_BASE_PATH", f.project)
	t.Setenv("ATMOS_LOGS_LEVEL", "Warning")
	t.Setenv("ATMOS_GITHUB_CLI", "")
	t.Setenv("ATMOS_GITHUB_TOKEN", "")
	t.Setenv("ATMOS_PRO_GITHUB_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")
}

func (f *tripleSlashFixture) command(t *testing.T) *cobra.Command {
	t.Helper()
	cmd := newTestCommandWithGlobalFlags("pull")
	cmd.Flags().AddFlagSet(newVendorPullFlagSet(true))
	cmd.SetContext(context.Background())
	return cmd
}

func (f *tripleSlashFixture) assertVendoredFiles(t *testing.T) {
	t.Helper()
	target := filepath.Join(f.project, "components", "terraform", "s3-bucket")
	require.DirExists(t, target)
	for name, want := range f.expected {
		actual, err := os.ReadFile(filepath.Join(target, filepath.FromSlash(name)))
		require.NoError(t, err, "included file must be installed: %s", name)
		assert.Equal(t, want, string(actual), "tagged fixture content: %s", name)
	}
	for _, name := range []string{"excluded.txt", "docs/excluded.txt"} {
		require.NoFileExists(t, filepath.Join(target, filepath.FromSlash(name)), "unmatched files must stay excluded")
	}
}
