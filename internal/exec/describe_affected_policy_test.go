package exec

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/policy"
	"github.com/cloudposse/atmos/pkg/schema"
)

const affectedTestManifest = `name: prod
components:
  helm:
    app:
      metadata:
        labels: {owner: platform}
      vars: {enabled: false}
`

func affectedPolicyRepo(t *testing.T, files map[string]string) (string, *git.Repository) {
	t.Helper()
	root := t.TempDir()
	repo, err := git.PlainInit(root, false)
	require.NoError(t, err)
	for path, content := range files {
		path = filepath.Join(root, path)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	}
	worktree, err := repo.Worktree()
	require.NoError(t, err)
	_, err = worktree.Add(".")
	require.NoError(t, err)
	_, err = worktree.Commit("fixture", &git.CommitOptions{Author: &object.Signature{Name: "Test", Email: "test@example.com", When: time.Unix(1, 0)}})
	require.NoError(t, err)
	return root, repo
}

func affectedPolicyTestConfig(root string) *schema.AtmosConfiguration {
	config := &schema.AtmosConfiguration{
		BasePath: root, BasePathAbsolute: root,
		StacksBaseAbsolutePath: filepath.Join(root, "stacks"),
		HelmDirAbsolutePath:    filepath.Join(root, "components", "helm"),
	}
	config.Stacks.IncludedPaths = []string{"**/*"}
	return config
}

func TestAffectedPolicyCollection(t *testing.T) {
	basePath, base := affectedPolicyRepo(t, map[string]string{"stacks/prod.yaml": affectedTestManifest})
	headPath, head := affectedPolicyRepo(t, map[string]string{"stacks/prod.yaml": "name: prod\ncomponents: {}\n"})
	request, err := collectAffectedPolicyRequest(affectedPolicyTestConfig(headPath), head, base, headPath, basePath)
	require.NoError(t, err)
	var input policy.AffectedInput
	require.NoError(t, json.Unmarshal(request.Input, &input))
	require.True(t, input.Complete)
	require.Len(t, input.Instances, 1, "disabled instance deletion is included")
	require.Equal(t, policy.Removed, input.Instances[0].Action)
	require.Equal(t, "prod", input.Instances[0].Identity.Stack)
	require.Equal(t, "helm", input.Instances[0].Identity.ComponentType)
	require.Equal(t, policy.Modified, input.Files[0].Action)
	rules := []policy.Rule{{
		ID: "no-removals", On: policy.AtmosAffected, Engine: policy.CELEngine,
		Expression: `input.instances.exists(i, i.action == "remove")`, Effects: policy.Effects{Deny: true},
	}}
	require.ErrorIs(t, evaluateAffectedPolicies(request, rules), errUtils.ErrPolicyDenied)
	rules[0].Expression = "false"
	require.NoError(t, evaluateAffectedPolicies(request, rules))
	rules[0].Expression = "input.unavailable == true"
	require.ErrorIs(t, evaluateAffectedPolicies(request, rules), errUtils.ErrPolicyEvaluation)
}

func TestAffectedPolicyUnchangedAndSourceOnly(t *testing.T) {
	basePath, base := affectedPolicyRepo(t, map[string]string{"stacks/prod.yaml": affectedTestManifest, "components/helm/app/Chart.yaml": "name: app"})
	headPath, head := affectedPolicyRepo(t, map[string]string{"stacks/prod.yaml": affectedTestManifest, "components/helm/app/Chart.yaml": "name: changed"})
	config := affectedPolicyTestConfig(headPath)
	request, err := collectAffectedPolicyRequest(config, head, base, headPath, basePath)
	require.NoError(t, err)
	var input policy.AffectedInput
	require.NoError(t, json.Unmarshal(request.Input, &input))
	require.Len(t, input.Instances, 1)
	require.Equal(t, []string{"component_files"}, input.Instances[0].Reasons)
	require.Empty(t, input.Changes)
	request, err = collectAffectedPolicyRequest(config, head, head, headPath, headPath)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(request.Input, &input))
	require.Empty(t, input.Instances)
	require.Empty(t, input.Files)
}

func TestAffectedPolicyGateBeforeFilters(t *testing.T) {
	basePath, base := affectedPolicyRepo(t, map[string]string{"stacks/prod.yaml": affectedTestManifest})
	headPath, head := affectedPolicyRepo(t, map[string]string{"README.md": "stack removed"})
	directory := t.TempDir()
	t.Setenv("ATMOS_POLICY_DIR", directory)
	require.NoError(t, os.WriteFile(filepath.Join(directory, "guard.yaml"), []byte(`apiVersion: atmos/v1
kind: Policy
metadata: {name: guard}
spec:
  policies:
    no-removals:
      on: atmos.affected
      rule: input.instances.exists(i, i.action == "remove")
      action: deny
      reason: Removed instances require a migration.
`), 0o600))
	_, _, _, err := executeDescribeAffected(affectedPolicyTestConfig(headPath), headPath, basePath, head, base,
		false, false, "not-prod", false, false, nil, AffectedFilter{}, nil, true, DescribeStacksErrorOptions{OnError: OnErrorWarn})
	require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
}

func TestAffectedPolicyCollectionFailsClosed(t *testing.T) {
	basePath, base := affectedPolicyRepo(t, map[string]string{"stacks/prod.yaml": affectedTestManifest})
	for name, manifest := range map[string]string{
		"missing import": "import: [missing]\n" + affectedTestManifest,
		"unknown type":   "name: prod\ncomponents: {future: {app: {}}}\n",
	} {
		t.Run(name, func(t *testing.T) {
			headPath, head := affectedPolicyRepo(t, map[string]string{"stacks/prod.yaml": manifest})
			_, err := collectAffectedPolicyRequest(affectedPolicyTestConfig(headPath), head, base, headPath, basePath)
			require.Error(t, err)
		})
	}
	require.NoError(t, os.WriteFile(filepath.Join(basePath, "untracked"), []byte("dirty"), 0o600))
	_, _, err := affectedPolicyTree(base)
	require.ErrorIs(t, err, errUtils.ErrPolicyInputUnavailable)
}

func TestAffectedPolicyInheritedLabelChange(t *testing.T) {
	manifest := "name: prod\nimport: [catalog/defaults]\ncomponents:\n  helm:\n    app:\n      vars:\n        deferred: '!exec policy_must_not_execute'\n"
	baseFiles := map[string]string{
		"stacks/prod.yaml":             manifest,
		"stacks/catalog/defaults.yaml": "metadata:\n  labels: {owner: platform}\n",
	}
	headFiles := map[string]string{
		"stacks/prod.yaml":             manifest,
		"stacks/catalog/defaults.yaml": "metadata:\n  labels: {owner: application}\n",
	}
	basePath, base := affectedPolicyRepo(t, baseFiles)
	headPath, head := affectedPolicyRepo(t, headFiles)
	config := affectedPolicyTestConfig(headPath)
	config.Stacks.ExcludedPaths = []string{"catalog/**"}
	request, err := collectAffectedPolicyRequest(config, head, base, headPath, basePath)
	require.NoError(t, err)
	var input policy.AffectedInput
	require.NoError(t, json.Unmarshal(request.Input, &input))
	require.Len(t, input.Instances, 1)
	require.Contains(t, string(input.Instances[0].Before), `"owner":"platform"`)
	require.Contains(t, string(input.Instances[0].After), `"owner":"application"`)
	require.Contains(t, string(input.Instances[0].After), "!exec policy_must_not_execute")
	for _, change := range input.Changes {
		require.False(t, strings.Contains(change.Path, "deferred"))
	}
}

func TestAffectedPolicyFileMovesAndRevisions(t *testing.T) {
	_, base := affectedPolicyRepo(t, map[string]string{"old.bin": "same data", "mode": "data"})
	_, head := affectedPolicyRepo(t, map[string]string{"new.bin": "same data", "mode": "updated"})
	before, _, err := affectedPolicyTree(base)
	require.NoError(t, err)
	after, revision, err := affectedPolicyTree(head)
	require.NoError(t, err)
	files, err := affectedPolicyFiles(before, after)
	require.NoError(t, err)
	require.ElementsMatch(t, []policy.FileChange{
		{Path: "old.bin", Action: policy.Removed, Ambiguous: true},
		{Path: "new.bin", Action: policy.Added, Ambiguous: true},
		{Path: "mode", Action: policy.Modified},
	}, files)
	require.NoError(t, verifyAffectedPolicyRevision(head, revision))
	require.ErrorIs(t, verifyAffectedPolicyRevision(head, "wrong-revision"), errUtils.ErrPolicyInputUnavailable)
}
