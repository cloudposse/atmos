package describe

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/cloudposse/atmos/tests"
)

// Use a small local origin so race tests exercise real pack transfer and ref selection
// without downloading and indexing the entire history of the live Atmos repository.
func TestDescribeAffectedWithTargetRefClone(t *testing.T) {
	originPath := t.TempDir()
	origin, err := git.PlainInitWithOptions(originPath, &git.PlainInitOptions{
		InitOptions: git.InitOptions{DefaultBranch: plumbing.NewBranchReferenceName("main")},
	})
	require.NoError(t, err)
	base := commitDescribeAffectedFixture(t, originPath, origin, 1)
	require.NoError(t, origin.Storer.SetReference(plumbing.NewHashReference(
		plumbing.NewBranchReferenceName("baseline"), base,
	)))
	head := commitDescribeAffectedFixture(t, originPath, origin, 2)

	// A host makes the file URL valid for the same remote metadata parser used by
	// hosted repositories. The file transport still transfers from this local path.
	originURL := (&url.URL{Scheme: "file", Host: "localhost", Path: filepath.ToSlash(originPath)}).String()
	localPath := t.TempDir()
	_, err = git.PlainClone(localPath, false, &git.CloneOptions{URL: originURL})
	require.NoError(t, err)
	t.Chdir(localPath)

	for _, tt := range []struct {
		name     string
		ref      string
		sha      string
		wantBase plumbing.Hash
		changed  bool
	}{
		{name: "current branch", ref: "refs/heads/main", wantBase: head},
		{name: "older branch", ref: "refs/heads/baseline", wantBase: base, changed: true},
		{name: "explicit commit", ref: "refs/heads/main", sha: base.String(), wantBase: base, changed: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("ATMOS_CLI_CONFIG_PATH", localPath)
			t.Setenv("ATMOS_BASE_PATH", localPath)
			atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
			require.NoError(t, err)
			affected, localHead, remoteHead, remoteURL, err := e.ExecuteDescribeAffectedWithTargetRefClone(
				&atmosConfig, tt.ref, tt.sha, "", "", true, true, "", true, true, nil, false, nil, false,
			)
			require.NoError(t, err)
			require.NotNil(t, localHead)
			require.NotNil(t, remoteHead)
			assert.Equal(t, head, localHead.Hash())
			assert.Equal(t, tt.wantBase, remoteHead.Hash())
			assert.Equal(t, originURL, remoteURL)
			if !tt.changed {
				assert.Empty(t, affected)
				return
			}
			require.Len(t, affected, 1, "only the changed component should be affected")
			assert.Equal(t, "app", affected[0].Component)
			assert.Equal(t, "terraform", affected[0].ComponentType)
			assert.Equal(t, "prod", affected[0].Stack)
			assert.Equal(t, "stack.vars", affected[0].Affected)
		})
	}
}

func commitDescribeAffectedFixture(t *testing.T, root string, repo *git.Repository, replicas int) plumbing.Hash {
	t.Helper()
	files := map[string]string{
		"atmos.yaml": `base_path: .
components:
  terraform:
    base_path: components/terraform
stacks:
  base_path: stacks
  included_paths: ["*.yaml"]
  name_template: "{{ .vars.stage }}"
`,
		"stacks/prod.yaml": fmt.Sprintf(`vars:
  stage: prod
components:
  terraform:
    app:
      vars:
        replicas: %d
    unchanged:
      vars:
        replicas: 1
`, replicas),
		"components/terraform/app/main.tf":       "# app fixture\n",
		"components/terraform/unchanged/main.tf": "# unchanged fixture\n",
	}
	for name, content := range files {
		path := filepath.Join(root, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	}
	worktree, err := repo.Worktree()
	require.NoError(t, err)
	_, err = worktree.Add(".")
	require.NoError(t, err)
	hash, err := worktree.Commit("fixture", &git.CommitOptions{Author: &object.Signature{
		Name: "Test", Email: "test@example.com", When: time.Unix(int64(replicas), 0),
	}})
	require.NoError(t, err)
	return hash
}

func TestDescribeAffectedWithTargetRepoPath(t *testing.T) {
	t.Parallel()

	// Check for Git repository with valid remotes precondition
	tests.RequireGitRemoteWithValidURL(t)

	configAndStacksInfo := schema.ConfigAndStacksInfo{}

	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	assert.Nil(t, err)

	// We are using `atmos.yaml` from this dir. This `atmos.yaml` has set base_path: "../../tests/fixtures/scenarios/complete",
	// which will be wrong for the remote repo which is cloned into a temp dir.
	// Set the correct base path for the cloned remote repo
	atmosConfig.BasePath = "./tests/fixtures/scenarios/complete"

	// Point to the same local repository
	// This will compare this local repository with itself as the remote target, which should result in an empty `affected` list
	repoPath := "../../"

	affected, _, _, _, err := e.ExecuteDescribeAffectedWithTargetRepoPath(
		&atmosConfig,
		repoPath,
		true,
		true,
		"",
		true,
		true,
		nil,
		false,
		nil,   // authManager
		false, // authDisabled
	)
	assert.Nil(t, err)

	affectedYaml, err := u.ConvertToYAML(affected)
	assert.Nil(t, err)
	t.Cleanup(func() {
		if t.Failed() {
			if affectedYaml != "" {
				t.Logf("Affected components and stacks:\n%s", affectedYaml)
			} else {
				t.Logf("Affected components and stacks (raw): %+v", affected)
			}
		}
	})
}
