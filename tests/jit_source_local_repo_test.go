package tests

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// jitSourceUpstreams lists the public repositories the source-provisioner
// fixtures point at. The fixtures keep their real-world URIs (they double as
// documentation); the tests rewrite only the repository part of each `uri:`
// in their sandboxed copy, so `//subpath` and `version:` keep working exactly
// as they do against GitHub.
var jitSourceUpstreams = []string{
	"github.com/cloudposse/terraform-null-label",
	"github.com/cloudposse-archives/helmfiles",
	"github.com/aws-samples/amazon-eks-custom-amis",
}

// jitSourceRepoFiles is the content of the local stand-in repository: the
// `exports/` module the terraform fixtures vendor (only `context.tf` must
// exist; the README and examples/ exercise excluded_paths), the helmfile
// release directory, and packer templates at the repository root.
var jitSourceRepoFiles = map[string]string{
	"exports/context.tf":                   "variable \"enabled\" {\n  type    = bool\n  default = true\n}\n",
	"exports/README.md":                    "# exports\n",
	"exports/examples/complete/main.tf":    "# excluded by examples/**\n",
	"releases/nginx-ingress/helmfile.yaml": "releases: []\n",
	"releases/nginx-ingress/README.md":     "# nginx-ingress\n",
	"main.pkr.hcl":                         "source \"null\" \"example\" {\n  communicator = \"none\"\n}\n",
	"variables.pkrvars.hcl":                "ami_name = \"example\"\n",
	"README.md":                            "# stand-in for public sources\n",
}

// jitSourceRepoTags are the `version:` values the fixtures pin, all pointing
// at the single commit; `main` (the packer fixture's version) is the branch.
var jitSourceRepoTags = []string{"0.25.0", "0.126.0"}

// initJITSourceRepo creates the local git repository the source-provisioner
// tests clone from instead of GitHub, and returns a `git::file://` URI for it.
// Cloning over the network made these tests fail whenever a hosted runner's
// DNS blipped mid-clone -- on a code path (git clone through go-getter) that
// is identical for a file:// remote, minus the network.
func initJITSourceRepo(t *testing.T) string {
	t.Helper()

	repoDir := t.TempDir()
	runJITSourceGit(t, repoDir, "init")
	runJITSourceGit(t, repoDir, "checkout", "-b", "main")
	runJITSourceGit(t, repoDir, "config", "user.email", "test@example.com")
	runJITSourceGit(t, repoDir, "config", "user.name", "Test User")
	// Never sign commits in throwaway test repos: signing is slow, needs no
	// verification here, and hangs on dev machines whose global git config
	// enables commit.gpgsign (e.g. a 1Password agent).
	runJITSourceGit(t, repoDir, "config", "commit.gpgsign", "false")

	for name, content := range jitSourceRepoFiles {
		path := filepath.Join(repoDir, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}
	runJITSourceGit(t, repoDir, "add", ".")
	runJITSourceGit(t, repoDir, "commit", "-m", "initial")
	for _, tag := range jitSourceRepoTags {
		runJITSourceGit(t, repoDir, "tag", tag)
	}
	return "git::" + remoteImportsGitFileURI(repoDir)
}

func runJITSourceGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v failed: %s", args, string(out))
}

// rewriteJITSourceURIs points every fixture `uri:` under dir's stack catalog
// at repoURI, keeping any `//subpath` suffix. It only touches the sandboxed
// copy a test is about to run against, never the checked-in fixture.
func rewriteJITSourceURIs(t *testing.T, dir, repoURI string) {
	t.Helper()

	catalog := filepath.Join(dir, "stacks", "catalog")
	entries, err := os.ReadDir(catalog)
	require.NoError(t, err)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		path := filepath.Join(catalog, entry.Name())
		content, err := os.ReadFile(path)
		require.NoError(t, err)
		rewritten := string(content)
		for _, upstream := range jitSourceUpstreams {
			rewritten = strings.ReplaceAll(rewritten, upstream, repoURI)
		}
		if rewritten == string(content) {
			continue
		}
		require.NoError(t, os.WriteFile(path, []byte(rewritten), 0o644))
	}
}
