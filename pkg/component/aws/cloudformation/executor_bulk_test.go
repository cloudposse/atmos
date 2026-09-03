package cloudformation

import (
	"context"
	"errors"
	"testing"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/component"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// authManagerForBulk must return (nil, nil) without attempting real
// authentication when no --identity is configured — the common bulk path.
func TestAuthManagerForBulk_NoIdentity(t *testing.T) {
	mgr, err := authManagerForBulk(&schema.AtmosConfiguration{}, &schema.ConfigAndStacksInfo{})
	require.NoError(t, err)
	assert.Nil(t, mgr)
}

// authManagerForBulk must propagate a real authentication failure when
// --identity names an identity that isn't configured.
func TestAuthManagerForBulk_UnknownIdentityErrors(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	info := &schema.ConfigAndStacksInfo{Identity: "nonexistent-identity"}

	_, err := authManagerForBulk(atmosConfig, info)
	require.Error(t, err, "an identity absent from auth config must fail to authenticate")
}

// graphSelectionForBulk must return a nil selection (process everything) when
// --affected isn't set — the common bulk (--all) path.
func TestGraphSelectionForBulk_NotAffected(t *testing.T) {
	selection, err := graphSelectionForBulk(&component.ExecutionContext{}, &schema.AtmosConfiguration{}, &schema.ConfigAndStacksInfo{})
	require.NoError(t, err)
	assert.Nil(t, selection)
}

// graphSelectionForBulk must build node IDs only for aws/cloudformation,
// non-deleted affected components, and must thread include-dependents
// through from the flags.
func TestGraphSelectionForBulk_Affected(t *testing.T) {
	original := affectedCloudFormationComponentsFunc
	affectedCloudFormationComponentsFunc = func(_ *component.ExecutionContext, _ *schema.AtmosConfiguration, _ *schema.ConfigAndStacksInfo) ([]schema.Affected, error) {
		return []schema.Affected{
			{Component: "vpc", Stack: "dev", ComponentType: cfg.CloudFormationComponentType},
			{Component: "deleted-stack", Stack: "dev", ComponentType: cfg.CloudFormationComponentType, Deleted: true},
			{Component: "eks", Stack: "dev", ComponentType: "terraform"},
		}, nil
	}
	t.Cleanup(func() { affectedCloudFormationComponentsFunc = original })

	ctx := &component.ExecutionContext{Flags: map[string]any{"include-dependents": true}}
	selection, err := graphSelectionForBulk(ctx, &schema.AtmosConfiguration{}, &schema.ConfigAndStacksInfo{Affected: true})
	require.NoError(t, err)
	require.NotNil(t, selection)
	assert.Equal(t, []string{component.GraphNodeID("vpc", "dev")}, selection.NodeIDs,
		"deleted and non-cloudformation affected components must be excluded")
	assert.True(t, selection.IncludeDependents)
	assert.True(t, selection.IncludeDependencies)
}

// graphSelectionForBulk must propagate an error from the affected-components lookup.
func TestGraphSelectionForBulk_AffectedError(t *testing.T) {
	sentinel := errors.New("describe affected failed")
	original := affectedCloudFormationComponentsFunc
	affectedCloudFormationComponentsFunc = func(_ *component.ExecutionContext, _ *schema.AtmosConfiguration, _ *schema.ConfigAndStacksInfo) ([]schema.Affected, error) {
		return nil, sentinel
	}
	t.Cleanup(func() { affectedCloudFormationComponentsFunc = original })

	_, err := graphSelectionForBulk(&component.ExecutionContext{}, &schema.AtmosConfiguration{}, &schema.ConfigAndStacksInfo{Affected: true})
	require.Error(t, err)
	assert.ErrorIs(t, err, sentinel)
}

// applyAffectedFlags must copy each recognized flag into the corresponding
// DescribeAffectedCmdArgs field, leaving unset flags at their zero value.
func TestApplyAffectedFlags(t *testing.T) {
	args := &e.DescribeAffectedCmdArgs{}
	flags := map[string]any{
		"repo-path":        "/tmp/repo",
		"ref":              "refs/heads/main",
		"sha":              "abc123",
		"ssh-key":          "/tmp/key",
		"ssh-key-password": "secret",
		"clone-target-ref": true,
	}
	applyAffectedFlags(args, flags)

	assert.Equal(t, "/tmp/repo", args.RepoPath)
	assert.Equal(t, "refs/heads/main", args.Ref)
	assert.Equal(t, "abc123", args.SHA)
	assert.Equal(t, "/tmp/key", args.SSHKeyPath)
	assert.Equal(t, "secret", args.SSHKeyPassword)
	assert.True(t, args.CloneTargetRef)
}

// applyAffectedFlags must leave Ref/SHA unset when the flags are empty
// strings (only a non-empty value should override).
func TestApplyAffectedFlags_EmptyValuesDoNotOverride(t *testing.T) {
	args := &e.DescribeAffectedCmdArgs{Ref: "preset-ref", SHA: "preset-sha"}
	applyAffectedFlags(args, map[string]any{"ref": "", "sha": ""})

	assert.Equal(t, "preset-ref", args.Ref)
	assert.Equal(t, "preset-sha", args.SHA)
}

// applyAffectedBaseFlag must route a commit-SHA-shaped --base into args.SHA,
// and anything else (a ref name) into args.Ref.
func TestApplyAffectedBaseFlag(t *testing.T) {
	shaLike := "0123456789abcdef0123456789abcdef01234567"
	require.True(t, ci.IsCommitSHA(shaLike), "test fixture must actually look like a commit SHA")

	args := &e.DescribeAffectedCmdArgs{}
	applyAffectedBaseFlag(args, map[string]any{"base": shaLike})
	assert.Equal(t, shaLike, args.SHA)
	assert.Empty(t, args.Ref)

	args = &e.DescribeAffectedCmdArgs{}
	applyAffectedBaseFlag(args, map[string]any{"base": "main"})
	assert.Equal(t, "main", args.Ref)
	assert.Empty(t, args.SHA)
}

// applyAffectedBaseFlag must be a no-op when --base isn't set.
func TestApplyAffectedBaseFlag_NotSet(t *testing.T) {
	args := &e.DescribeAffectedCmdArgs{}
	applyAffectedBaseFlag(args, map[string]any{})
	assert.Empty(t, args.Ref)
	assert.Empty(t, args.SHA)
}

// dispatchAffected must route to executeAffectedWithRepoPath when RepoPath is set.
func TestDispatchAffected_RepoPath(t *testing.T) {
	original := executeAffectedWithRepoPath
	var gotRepoPath string
	executeAffectedWithRepoPath = func(_ *schema.AtmosConfiguration, repoPath string, _, _ bool, _ string, _, _ bool, _ []string, _ bool, _ auth.AuthManager, _ bool) ([]schema.Affected, *plumbing.Reference, *plumbing.Reference, string, error) {
		gotRepoPath = repoPath
		return []schema.Affected{{Component: "vpc"}}, nil, nil, "", nil
	}
	t.Cleanup(func() { executeAffectedWithRepoPath = original })

	args := &e.DescribeAffectedCmdArgs{RepoPath: "/tmp/repo"}
	affected, err := dispatchAffected(&schema.AtmosConfiguration{}, args, nil)
	require.NoError(t, err)
	assert.Equal(t, "/tmp/repo", gotRepoPath)
	assert.Equal(t, []schema.Affected{{Component: "vpc"}}, affected)
}

// dispatchAffected must route to executeAffectedWithRefClone when
// CloneTargetRef is set (and RepoPath is not).
func TestDispatchAffected_CloneTargetRef(t *testing.T) {
	original := executeAffectedWithRefClone
	called := false
	executeAffectedWithRefClone = func(_ *schema.AtmosConfiguration, _, _, _, _ string, _, _ bool, _ string, _, _ bool, _ []string, _ bool, _ auth.AuthManager, _ bool) ([]schema.Affected, *plumbing.Reference, *plumbing.Reference, string, error) {
		called = true
		return nil, nil, nil, "", nil
	}
	t.Cleanup(func() { executeAffectedWithRefClone = original })

	args := &e.DescribeAffectedCmdArgs{CloneTargetRef: true}
	_, err := dispatchAffected(&schema.AtmosConfiguration{}, args, nil)
	require.NoError(t, err)
	assert.True(t, called)
}

// dispatchAffected must fall through to executeAffectedWithRefCheckout by default.
func TestDispatchAffected_DefaultRefCheckout(t *testing.T) {
	original := executeAffectedWithRefCheckout
	called := false
	executeAffectedWithRefCheckout = func(_ *schema.AtmosConfiguration, _, _, _ string, _, _ bool, _ string, _, _ bool, _ []string, _ bool, _ auth.AuthManager, _ bool) ([]schema.Affected, *plumbing.Reference, *plumbing.Reference, string, error) {
		called = true
		return nil, nil, nil, "", nil
	}
	t.Cleanup(func() { executeAffectedWithRefCheckout = original })

	args := &e.DescribeAffectedCmdArgs{}
	_, err := dispatchAffected(&schema.AtmosConfiguration{}, args, nil)
	require.NoError(t, err)
	assert.True(t, called)
}

// executeBulk must resolve the (nil, since --all) auth manager, describe
// stacks, resolve a nil graph selection (not --affected), and dispatch to
// component.ExecuteGraph with the resolved options.
func TestExecuteBulk_All(t *testing.T) {
	origDescribe := executeDescribeStacks
	stacks := map[string]any{"dev": map[string]any{}}
	executeDescribeStacks = func(_ *schema.AtmosConfiguration, filterByStack string, _, componentTypes, _ []string, _, _, _, _ bool, _ []string, _ auth.AuthManager) (map[string]any, error) {
		assert.Equal(t, []string{cfg.CloudFormationComponentType}, componentTypes)
		return stacks, nil
	}
	t.Cleanup(func() { executeDescribeStacks = origDescribe })

	origGraph := executeGraph
	var gotOpts *component.GraphExecutionOptions
	executeGraph = func(_ context.Context, opts *component.GraphExecutionOptions) error {
		gotOpts = opts
		return nil
	}
	t.Cleanup(func() { executeGraph = origGraph })

	ctx := &component.ExecutionContext{Flags: map[string]any{"foo": "bar"}}
	atmosConfig := &schema.AtmosConfiguration{}
	info := &schema.ConfigAndStacksInfo{All: true}

	err := executeBulk(ctx, atmosConfig, info, OperationApply)
	require.NoError(t, err)
	require.NotNil(t, gotOpts)
	assert.Equal(t, stacks, gotOpts.Stacks)
	assert.Equal(t, cfg.CloudFormationComponentType, gotOpts.ComponentType)
	assert.Equal(t, string(OperationApply), gotOpts.SubCommand)
	assert.Nil(t, gotOpts.Selection, "--all (not --affected) must pass a nil selection")
	assert.Equal(t, ctx.Flags, gotOpts.Flags)
}

// executeBulk must propagate an executeDescribeStacks failure without
// reaching component.ExecuteGraph.
func TestExecuteBulk_DescribeStacksError(t *testing.T) {
	sentinel := errors.New("describe stacks failed")
	origDescribe := executeDescribeStacks
	executeDescribeStacks = func(_ *schema.AtmosConfiguration, _ string, _, _, _ []string, _, _, _, _ bool, _ []string, _ auth.AuthManager) (map[string]any, error) {
		return nil, sentinel
	}
	t.Cleanup(func() { executeDescribeStacks = origDescribe })

	origGraph := executeGraph
	executeGraph = func(_ context.Context, _ *component.GraphExecutionOptions) error {
		t.Fatal("executeGraph must not be called when describe-stacks fails")
		return nil
	}
	t.Cleanup(func() { executeGraph = origGraph })

	err := executeBulk(&component.ExecutionContext{}, &schema.AtmosConfiguration{}, &schema.ConfigAndStacksInfo{All: true}, OperationApply)
	require.Error(t, err)
	assert.ErrorIs(t, err, sentinel)
}

// executeBulk must propagate an authManagerForBulk failure (a configured but
// unresolvable --identity) before ever describing stacks.
func TestExecuteBulk_AuthManagerError(t *testing.T) {
	origDescribe := executeDescribeStacks
	executeDescribeStacks = func(_ *schema.AtmosConfiguration, _ string, _, _, _ []string, _, _, _, _ bool, _ []string, _ auth.AuthManager) (map[string]any, error) {
		t.Fatal("executeDescribeStacks must not be called when auth resolution fails")
		return nil, nil
	}
	t.Cleanup(func() { executeDescribeStacks = origDescribe })

	info := &schema.ConfigAndStacksInfo{All: true, Identity: "nonexistent-identity"}
	err := executeBulk(&component.ExecutionContext{}, &schema.AtmosConfiguration{}, info, OperationApply)
	require.Error(t, err)
}

// affectedCloudFormationComponents must wire ctx.Flags and info's auth state
// into DescribeAffectedCmdArgs, then dispatch — verified end-to-end via the
// default ref-checkout path.
func TestAffectedCloudFormationComponents_WiresArgsAndDispatches(t *testing.T) {
	original := executeAffectedWithRefCheckout
	var gotArgs *e.DescribeAffectedCmdArgs
	executeAffectedWithRefCheckout = func(atmosConfig *schema.AtmosConfiguration, ref, sha, targetBranch string, includeSpaceliftAdminStacks, includeSettings bool, stack string, processTemplates, processYamlFunctions bool, skip []string, excludeLocked bool, authManager auth.AuthManager, authDisabled bool) ([]schema.Affected, *plumbing.Reference, *plumbing.Reference, string, error) {
		gotArgs = &e.DescribeAffectedCmdArgs{
			Ref: ref, SHA: sha, TargetBranch: targetBranch, Stack: stack,
			AuthDisabled: authDisabled,
		}
		return []schema.Affected{}, nil, nil, "", nil
	}
	t.Cleanup(func() { executeAffectedWithRefCheckout = original })

	ctx := &component.ExecutionContext{Flags: map[string]any{"ref": "refs/heads/main"}}
	info := &schema.ConfigAndStacksInfo{Stack: "dev", AuthDisabled: true}

	_, err := affectedCloudFormationComponents(ctx, &schema.AtmosConfiguration{}, info)
	require.NoError(t, err)
	require.NotNil(t, gotArgs)
	assert.Equal(t, "refs/heads/main", gotArgs.Ref)
	assert.Equal(t, "dev", gotArgs.Stack)
	assert.True(t, gotArgs.AuthDisabled)
}
