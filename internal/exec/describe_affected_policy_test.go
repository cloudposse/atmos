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
	cfg "github.com/cloudposse/atmos/pkg/config"
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

func TestAffectedPolicyInstanceCountComponentTypes(t *testing.T) {
	stackWith := func(kind string) map[string]any {
		return map[string]any{"prod": map[string]any{"components": map[string]any{
			kind: map[string]any{"first": map[string]any{}, "second": map[string]any{}},
		}}}
	}
	for _, kind := range affectedPolicyComponentTypes {
		t.Run(kind, func(t *testing.T) {
			count, err := affectedPolicyInstanceCount(stackWith(kind))
			require.NoError(t, err)
			require.Equal(t, 2, count)
		})
	}
	require.Contains(t, affectedPolicyComponentTypes, cfg.CloudFormationComponentType,
		"aws/cloudformation instances must be inspectable by affected policies")

	_, err := affectedPolicyInstanceCount(stackWith("unknown/type"))
	require.ErrorIs(t, err, errUtils.ErrPolicyInputUnavailable, "an unlisted component type fails closed")
}

// A source-only change must select the instances of every inspectable component type, not only
// those whose directory the original matching knew about.
func TestAffectedPolicySourceOnlyChangeForEveryComponentType(t *testing.T) {
	original := affectedPolicyEmulatorBasePath
	affectedPolicyEmulatorBasePath = func(*schema.AtmosConfiguration) string { return filepath.Join("components", "emulator") }
	t.Cleanup(func() { affectedPolicyEmulatorBasePath = original })

	tests := []struct {
		kind      string
		directory string
		configure func(*schema.AtmosConfiguration)
	}{
		{kind: cfg.CloudFormationComponentType, directory: filepath.Join("components", "cloudformation"), configure: func(c *schema.AtmosConfiguration) {
			c.CloudFormationDirAbsolutePath = filepath.Join(c.BasePathAbsolute, "components", "cloudformation")
		}},
		{kind: cfg.EmulatorComponentType, directory: filepath.Join("components", "emulator")},
	}
	for _, tc := range tests {
		t.Run(tc.kind, func(t *testing.T) {
			manifest := "name: prod\ncomponents:\n  " + tc.kind + ":\n    app:\n      metadata:\n        labels: {owner: platform}\n"
			source := filepath.Join(tc.directory, "app", "main.txt")
			basePath, base := affectedPolicyRepo(t, map[string]string{"stacks/prod.yaml": manifest, source: "old"})
			headPath, head := affectedPolicyRepo(t, map[string]string{"stacks/prod.yaml": manifest, source: "new"})
			config := affectedPolicyTestConfig(headPath)
			if tc.configure != nil {
				tc.configure(config)
			}

			request, err := collectAffectedPolicyRequest(config, head, base, headPath, basePath)

			require.NoError(t, err)
			var input policy.AffectedInput
			require.NoError(t, json.Unmarshal(request.Input, &input))
			require.Len(t, input.Instances, 1)
			require.Equal(t, tc.kind, input.Instances[0].Identity.ComponentType)
			require.Equal(t, []string{"component_files"}, input.Instances[0].Reasons)

			unchanged, err := collectAffectedPolicyRequest(config, head, head, headPath, headPath)
			require.NoError(t, err)
			require.NoError(t, json.Unmarshal(unchanged.Input, &input))
			require.Empty(t, input.Instances, "identical source selects nothing")
		})
	}
}

func TestAffectedPolicyComponentDirectories(t *testing.T) {
	original := affectedPolicyEmulatorBasePath
	t.Cleanup(func() { affectedPolicyEmulatorBasePath = original })
	root := t.TempDir()
	config := &schema.AtmosConfiguration{BasePathAbsolute: root, CloudFormationDirAbsolutePath: filepath.Join(root, "cfn")}

	t.Run("relative emulator base path resolves against the project", func(t *testing.T) {
		affectedPolicyEmulatorBasePath = func(*schema.AtmosConfiguration) string { return "emu" }
		directories := affectedPolicyComponentDirectories(config)
		require.Equal(t, filepath.Join(root, "emu"), directories[cfg.EmulatorComponentType])
		require.Equal(t, filepath.Join(root, "cfn"), directories[cfg.CloudFormationComponentType])
	})
	t.Run("absolute emulator base path is kept", func(t *testing.T) {
		absolute := filepath.Join(root, "absolute-emu")
		affectedPolicyEmulatorBasePath = func(*schema.AtmosConfiguration) string { return absolute }
		require.Equal(t, absolute, affectedPolicyComponentDirectories(config)[cfg.EmulatorComponentType])
	})
	t.Run("no emulator provider leaves the type unmatched", func(t *testing.T) {
		affectedPolicyEmulatorBasePath = func(*schema.AtmosConfiguration) string { return "" }
		require.NotContains(t, affectedPolicyComponentDirectories(config), cfg.EmulatorComponentType)
	})
	t.Run("registry lookup finds no provider in this package", func(t *testing.T) {
		affectedPolicyEmulatorBasePath = original
		require.NotPanics(t, func() { _ = affectedPolicyComponentDirectories(config) })
	})
}

// The base-component cache key has no checkout path, so collection must clear it on every exit,
// including a configuration error, before HEAD is resolved.
func TestAffectedPolicyInventoryClearsBaseComponentCacheOnError(t *testing.T) {
	baseComponentConfigCache.Store("stale:component:base", &schema.BaseComponentConfig{})
	t.Cleanup(ClearBaseComponentConfigCache)

	_, err := collectAffectedPolicyInventory(&schema.AtmosConfiguration{BasePathAbsolute: t.TempDir()}, t.TempDir(), filepath.Join(t.TempDir(), "missing-checkout"))
	require.Error(t, err)

	_, found := baseComponentConfigCache.Load("stale:component:base")
	require.False(t, found, "the cache is cleared even when collection fails")
}
