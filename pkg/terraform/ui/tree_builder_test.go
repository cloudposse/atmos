package ui

import (
	"context"
	"os"
	"testing"

	tfjson "github.com/hashicorp/terraform-json"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestBuildTreeFromPlan_Empty(t *testing.T) {
	plan := &tfjson.Plan{}

	tree := buildTreeFromPlan(plan, "dev", "vpc")
	require.NotNil(t, tree)
	assert.Equal(t, "dev", tree.Stack)
	assert.Equal(t, "vpc", tree.Component)
	assert.Empty(t, tree.Root.Children)
}

func TestBuildTreeFromPlan_SingleResource(t *testing.T) {
	plan := &tfjson.Plan{
		ResourceChanges: []*tfjson.ResourceChange{
			{
				Address: "aws_vpc.main",
				Mode:    tfjson.ManagedResourceMode,
				Change:  &tfjson.Change{Actions: []tfjson.Action{tfjson.ActionCreate}},
			},
		},
	}

	tree := buildTreeFromPlan(plan, "dev", "vpc")
	require.Len(t, tree.Root.Children, 1)
	assert.Equal(t, "aws_vpc.main", tree.Root.Children[0].Address)
	assert.Equal(t, "create", tree.Root.Children[0].Action)
}

func TestBuildTreeFromPlan_SkipsDataSources(t *testing.T) {
	plan := &tfjson.Plan{
		ResourceChanges: []*tfjson.ResourceChange{
			{
				Address: "aws_vpc.main",
				Mode:    tfjson.ManagedResourceMode,
				Change:  &tfjson.Change{Actions: []tfjson.Action{tfjson.ActionCreate}},
			},
			{
				Address: "data.aws_ami.latest",
				Mode:    tfjson.DataResourceMode,
				Change:  &tfjson.Change{Actions: []tfjson.Action{tfjson.ActionRead}},
			},
		},
	}

	tree := buildTreeFromPlan(plan, "dev", "vpc")
	// Only managed resource should be included.
	require.Len(t, tree.Root.Children, 1)
	assert.Equal(t, "aws_vpc.main", tree.Root.Children[0].Address)
}

func TestBuildTreeFromPlan_SkipsNoOp(t *testing.T) {
	plan := &tfjson.Plan{
		ResourceChanges: []*tfjson.ResourceChange{
			{
				Address: "aws_vpc.main",
				Mode:    tfjson.ManagedResourceMode,
				Change:  &tfjson.Change{Actions: []tfjson.Action{tfjson.ActionCreate}},
			},
			{
				Address: "aws_vpc.unchanged",
				Mode:    tfjson.ManagedResourceMode,
				Change:  &tfjson.Change{Actions: []tfjson.Action{tfjson.ActionNoop}},
			},
		},
	}

	tree := buildTreeFromPlan(plan, "dev", "vpc")
	// Only create action should be included, no-op should be skipped.
	require.Len(t, tree.Root.Children, 1)
	assert.Equal(t, "aws_vpc.main", tree.Root.Children[0].Address)
}

func TestBuildTreeFromPlan_ReplaceAction(t *testing.T) {
	plan := &tfjson.Plan{
		ResourceChanges: []*tfjson.ResourceChange{
			{
				Address: "aws_instance.web",
				Mode:    tfjson.ManagedResourceMode,
				Change:  &tfjson.Change{Actions: []tfjson.Action{tfjson.ActionDelete, tfjson.ActionCreate}},
			},
		},
	}

	tree := buildTreeFromPlan(plan, "dev", "app")
	require.Len(t, tree.Root.Children, 1)
	// Delete+Create should be "replace".
	assert.Equal(t, "replace", tree.Root.Children[0].Action)
}

func TestBuildTreeFromPlan_AllActionTypes(t *testing.T) {
	plan := &tfjson.Plan{
		ResourceChanges: []*tfjson.ResourceChange{
			{
				Address: "aws_vpc.create",
				Mode:    tfjson.ManagedResourceMode,
				Change:  &tfjson.Change{Actions: []tfjson.Action{tfjson.ActionCreate}},
			},
			{
				Address: "aws_vpc.update",
				Mode:    tfjson.ManagedResourceMode,
				Change:  &tfjson.Change{Actions: []tfjson.Action{tfjson.ActionUpdate}},
			},
			{
				Address: "aws_vpc.delete",
				Mode:    tfjson.ManagedResourceMode,
				Change:  &tfjson.Change{Actions: []tfjson.Action{tfjson.ActionDelete}},
			},
			{
				Address: "aws_vpc.replace",
				Mode:    tfjson.ManagedResourceMode,
				Change:  &tfjson.Change{Actions: []tfjson.Action{tfjson.ActionDelete, tfjson.ActionCreate}},
			},
		},
	}

	tree := buildTreeFromPlan(plan, "dev", "vpc")
	require.Len(t, tree.Root.Children, 4)

	// Find each action type.
	actions := make(map[string]string)
	for _, child := range tree.Root.Children {
		actions[child.Address] = child.Action
	}

	assert.Equal(t, "create", actions["aws_vpc.create"])
	assert.Equal(t, "update", actions["aws_vpc.update"])
	assert.Equal(t, "delete", actions["aws_vpc.delete"])
	assert.Equal(t, "replace", actions["aws_vpc.replace"])
}

func TestBuildTreeFromPlan_ModulePrefix(t *testing.T) {
	plan := &tfjson.Plan{
		ResourceChanges: []*tfjson.ResourceChange{
			{
				Address: "module.vpc.aws_subnet.main",
				Mode:    tfjson.ManagedResourceMode,
				Change:  &tfjson.Change{Actions: []tfjson.Action{tfjson.ActionCreate}},
			},
		},
	}

	tree := buildTreeFromPlan(plan, "dev", "vpc")
	require.Len(t, tree.Root.Children, 1)
	// module.vpc.aws_subnet.main is a resource within a module, not a module itself.
	assert.False(t, tree.Root.Children[0].IsModule)
	assert.Equal(t, "module.vpc.aws_subnet.main", tree.Root.Children[0].Address)
}

func TestBuildTreeFromPlan_SortedByAddress(t *testing.T) {
	plan := &tfjson.Plan{
		ResourceChanges: []*tfjson.ResourceChange{
			{
				Address: "z_resource.last",
				Mode:    tfjson.ManagedResourceMode,
				Change:  &tfjson.Change{Actions: []tfjson.Action{tfjson.ActionCreate}},
			},
			{
				Address: "a_resource.first",
				Mode:    tfjson.ManagedResourceMode,
				Change:  &tfjson.Change{Actions: []tfjson.Action{tfjson.ActionCreate}},
			},
			{
				Address: "m_resource.middle",
				Mode:    tfjson.ManagedResourceMode,
				Change:  &tfjson.Change{Actions: []tfjson.Action{tfjson.ActionCreate}},
			},
		},
	}

	tree := buildTreeFromPlan(plan, "dev", "test")
	require.Len(t, tree.Root.Children, 3)
	assert.Equal(t, "a_resource.first", tree.Root.Children[0].Address)
	assert.Equal(t, "m_resource.middle", tree.Root.Children[1].Address)
	assert.Equal(t, "z_resource.last", tree.Root.Children[2].Address)
}

// TestBuildDependencyTree_Success verifies BuildDependencyTree shells out to `terraform show
// -json`, parses the output, and builds a tree from it. The test binary itself stands in for
// terraform (see testmain_test.go's _ATMOS_TEST_TF_SHOW_JSON handling) so the test stays
// cross-platform and doesn't depend on a real terraform install.
func TestBuildDependencyTree_Success(t *testing.T) {
	exePath, err := os.Executable()
	require.NoError(t, err)

	planJSON := `{"format_version":"1.2","resource_changes":[{"address":"aws_vpc.main","mode":"managed","change":{"actions":["create"]}}]}`
	t.Setenv("_ATMOS_TEST_TF_SHOW_JSON", planJSON)

	opts := &TreeBuildOptions{
		PlanfilePath:  "plan.tfplan",
		TerraformPath: exePath,
		WorkingDir:    t.TempDir(),
		Stack:         "dev",
		Component:     "vpc",
	}

	tree, err := BuildDependencyTree(context.Background(), opts)

	require.NoError(t, err)
	require.NotNil(t, tree)
	assert.Equal(t, "dev", tree.Stack)
	assert.Equal(t, "vpc", tree.Component)
	require.Len(t, tree.Root.Children, 1)
	assert.Equal(t, "aws_vpc.main", tree.Root.Children[0].Address)
}

// TestBuildDependencyTree_CommandFailure verifies a non-zero exit from `terraform show` is
// wrapped in errUtils.ErrCommandStart rather than silently ignored.
func TestBuildDependencyTree_CommandFailure(t *testing.T) {
	exePath, err := os.Executable()
	require.NoError(t, err)

	t.Setenv("_ATMOS_TEST_EXIT_ONE", "1")

	opts := &TreeBuildOptions{
		PlanfilePath:  "plan.tfplan",
		TerraformPath: exePath,
		WorkingDir:    t.TempDir(),
		Stack:         "dev",
		Component:     "vpc",
	}

	tree, err := BuildDependencyTree(context.Background(), opts)

	require.Error(t, err)
	assert.Nil(t, tree)
	assert.ErrorIs(t, err, errUtils.ErrCommandStart)
}

// TestBuildDependencyTree_InvalidJSON verifies malformed `terraform show -json` output is
// wrapped in errUtils.ErrParseTerraformOutput rather than panicking.
func TestBuildDependencyTree_InvalidJSON(t *testing.T) {
	exePath, err := os.Executable()
	require.NoError(t, err)

	t.Setenv("_ATMOS_TEST_TF_SHOW_JSON", "{not valid json")

	opts := &TreeBuildOptions{
		PlanfilePath:  "plan.tfplan",
		TerraformPath: exePath,
		WorkingDir:    t.TempDir(),
		Stack:         "dev",
		Component:     "vpc",
	}

	tree, err := BuildDependencyTree(context.Background(), opts)

	require.Error(t, err)
	assert.Nil(t, tree)
	assert.ErrorIs(t, err, errUtils.ErrParseTerraformOutput)
}

// TestResourceChangeAction covers the composite-action (replace) and edge-case branches of
// resourceChangeAction that aren't already exercised indirectly via buildTreeFromPlan tests.
func TestResourceChangeAction(t *testing.T) {
	tests := []struct {
		name     string
		rc       *tfjson.ResourceChange
		expected string
	}{
		{
			name:     "nil change",
			rc:       &tfjson.ResourceChange{Change: nil},
			expected: noOpAction,
		},
		{
			name:     "no actions",
			rc:       &tfjson.ResourceChange{Change: &tfjson.Change{Actions: []tfjson.Action{}}},
			expected: noOpAction,
		},
		{
			name:     "single read action",
			rc:       &tfjson.ResourceChange{Change: &tfjson.Change{Actions: []tfjson.Action{tfjson.ActionRead}}},
			expected: "read",
		},
		{
			name:     "two actions is replace",
			rc:       &tfjson.ResourceChange{Change: &tfjson.Change{Actions: []tfjson.Action{tfjson.ActionCreate, tfjson.ActionDelete}}},
			expected: "replace",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := resourceChangeAction(tt.rc)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestExtractForcesReplacement covers the malformed-path branches (non-slice entries, empty
// slices, non-string first elements) that plan JSON should never produce but the function
// defensively skips rather than panicking on.
func TestExtractForcesReplacement(t *testing.T) {
	tests := []struct {
		name         string
		replacePaths []interface{}
		expected     map[string]bool
	}{
		{
			name:         "nil paths",
			replacePaths: nil,
			expected:     map[string]bool{},
		},
		{
			name:         "valid top-level path",
			replacePaths: []interface{}{[]interface{}{"ami"}},
			expected:     map[string]bool{"ami": true},
		},
		{
			name:         "malformed path entry is skipped",
			replacePaths: []interface{}{"not-a-slice"},
			expected:     map[string]bool{},
		},
		{
			name:         "empty path slice is skipped",
			replacePaths: []interface{}{[]interface{}{}},
			expected:     map[string]bool{},
		},
		{
			name:         "non-string first element is skipped",
			replacePaths: []interface{}{[]interface{}{42}},
			expected:     map[string]bool{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractForcesReplacement(tt.replacePaths)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestExtractDependencies_NilModuleCall verifies a ModuleCall whose Module field wasn't
// resolved (nil) is skipped rather than causing a nil pointer dereference.
func TestExtractDependencies_NilModuleCall(t *testing.T) {
	module := &tfjson.ConfigModule{
		Resources: []*tfjson.ConfigResource{
			{Address: "aws_vpc.main"},
		},
		ModuleCalls: map[string]*tfjson.ModuleCall{
			"broken": {Module: nil},
		},
	}

	dependsOn := make(map[string][]string)

	extractDependencies(module, "", dependsOn)

	// aws_vpc.main has no DependsOn/expressions, and the nil module call contributes nothing.
	assert.Empty(t, dependsOn)
}

// TestBuildTreeFromPlan_WithConfig_UsesRelationships verifies buildTreeFromPlan's
// config-present branch delegates to buildRelationships (rather than attachAllToRoot) when the
// plan carries a resolved Config/RootModule, nesting a dependent resource under its dependency.
func TestBuildTreeFromPlan_WithConfig_UsesRelationships(t *testing.T) {
	plan := &tfjson.Plan{
		ResourceChanges: []*tfjson.ResourceChange{
			{
				Address: "aws_vpc.main",
				Mode:    tfjson.ManagedResourceMode,
				Change:  &tfjson.Change{Actions: []tfjson.Action{tfjson.ActionCreate}},
			},
			{
				Address: "aws_subnet.a",
				Mode:    tfjson.ManagedResourceMode,
				Change:  &tfjson.Change{Actions: []tfjson.Action{tfjson.ActionCreate}},
			},
		},
		Config: &tfjson.Config{
			RootModule: &tfjson.ConfigModule{
				Resources: []*tfjson.ConfigResource{
					{Address: "aws_vpc.main"},
					{Address: "aws_subnet.a", DependsOn: []string{"aws_vpc.main"}},
				},
			},
		},
	}

	tree := buildTreeFromPlan(plan, "dev", "vpc")

	require.Len(t, tree.Root.Children, 1, "only the root-level aws_vpc.main should be a direct child")
	vpcNode := tree.Root.Children[0]
	assert.Equal(t, "aws_vpc.main", vpcNode.Address)
	require.Len(t, vpcNode.Children, 1)
	assert.Equal(t, "aws_subnet.a", vpcNode.Children[0].Address)
}

// TestBuildRelationships_DependencyNotInChangeSet verifies a resource that depends on
// something outside the change set (e.g. an unchanged resource) falls through to being
// attached at the root, exercising the "dependency not tracked" continue branch.
func TestBuildRelationships_DependencyNotInChangeSet(t *testing.T) {
	tree := &DependencyTree{
		Root:  &TreeNode{Address: "root"},
		nodes: map[string]*TreeNode{},
	}

	subnetNode := &TreeNode{Address: "aws_subnet.a", Action: "create"}
	tree.nodes["aws_subnet.a"] = subnetNode

	plan := &tfjson.Plan{
		Config: &tfjson.Config{
			RootModule: &tfjson.ConfigModule{
				Resources: []*tfjson.ConfigResource{
					// aws_vpc.main is not part of this change set (e.g. already exists/unchanged).
					{Address: "aws_subnet.a", DependsOn: []string{"aws_vpc.main"}},
				},
			},
		},
	}

	buildRelationships(tree, plan)

	assert.Contains(t, tree.Root.Children, subnetNode, "resource with an untracked dependency must attach to root")
	assert.Equal(t, tree.Root, subnetNode.Parent)
}

// TestExtractDependencies_ImplicitFromExpressions verifies dependencies referenced through
// expression attributes (not just explicit depends_on) are captured.
func TestExtractDependencies_ImplicitFromExpressions(t *testing.T) {
	module := &tfjson.ConfigModule{
		Resources: []*tfjson.ConfigResource{
			{
				Address: "aws_subnet.a",
				Expressions: map[string]*tfjson.Expression{
					"vpc_id": {
						ExpressionData: &tfjson.ExpressionData{
							References: []string{"aws_vpc.main.id"},
						},
					},
				},
			},
		},
	}

	dependsOn := make(map[string][]string)
	extractDependencies(module, "", dependsOn)

	require.Contains(t, dependsOn, "aws_subnet.a")
	assert.Contains(t, dependsOn["aws_subnet.a"], "aws_vpc.main")
}

// TestExtractDependencies_NestedModuleWithExistingPrefix verifies the nested child-module
// prefix is joined onto an already-non-empty prefix (as happens on recursive calls beyond the
// first nesting level), rather than only ever starting from an empty prefix.
func TestExtractDependencies_NestedModuleWithExistingPrefix(t *testing.T) {
	module := &tfjson.ConfigModule{
		ModuleCalls: map[string]*tfjson.ModuleCall{
			"subnet": {
				Module: &tfjson.ConfigModule{
					Resources: []*tfjson.ConfigResource{
						{Address: "aws_subnet.private", DependsOn: []string{"aws_vpc.main"}},
					},
				},
			},
		},
	}

	dependsOn := make(map[string][]string)
	extractDependencies(module, "module.network", dependsOn)

	require.Contains(t, dependsOn, "module.network.module.subnet.aws_subnet.private")
}

// TestNormalizeModuleReference_NoModuleKeyword covers the otherwise-unreachable-in-practice
// default branch: a reference with fewer than 2 dot-separated parts and no "module" keyword
// at all falls through the moduleCount and length checks and is returned unchanged.
func TestNormalizeModuleReference_NoModuleKeyword(t *testing.T) {
	result := normalizeModuleReference("weird")
	assert.Equal(t, "weird", result)
}

func TestBuildRelationships_WithDependencies(t *testing.T) {
	// Build tree manually for testing.
	tree := &DependencyTree{
		Root:  &TreeNode{Address: "root"},
		nodes: map[string]*TreeNode{},
	}

	// Create nodes for two resources.
	vpcNode := &TreeNode{Address: "aws_vpc.main", Action: "create"}
	subnetNode := &TreeNode{Address: "aws_subnet.a", Action: "create"}
	tree.nodes["aws_vpc.main"] = vpcNode
	tree.nodes["aws_subnet.a"] = subnetNode

	// Create plan with config showing subnet depends on vpc.
	plan := &tfjson.Plan{
		Config: &tfjson.Config{
			RootModule: &tfjson.ConfigModule{
				Resources: []*tfjson.ConfigResource{
					{Address: "aws_vpc.main"},
					{Address: "aws_subnet.a", DependsOn: []string{"aws_vpc.main"}},
				},
			},
		},
	}

	buildRelationships(tree, plan)

	// VPC should be at root.
	assert.Contains(t, tree.Root.Children, vpcNode)
	// Subnet should be child of VPC.
	assert.Contains(t, vpcNode.Children, subnetNode)
	assert.Equal(t, vpcNode, subnetNode.Parent)
}

// TestBuildRelationships_ForEachInstanceAddress is a regression test: a for_each/count
// resource's ResourceChanges address carries an instance key (e.g. aws_subnet.a["public"]),
// but plan.Config.RootModule.Resources — where extractDependencies reads depends_on from —
// only ever has the base, uninstanced address (aws_subnet.a). Without stripping the instance
// key before the dependsOn lookup, every for_each/count instance misses its dependency entry
// and falls back to root regardless of its real chain, silently flattening the tree and
// breaking the rendered gutters for anything nested under it.
func TestBuildRelationships_ForEachInstanceAddress(t *testing.T) {
	tree := &DependencyTree{
		Root:  &TreeNode{Address: "root"},
		nodes: map[string]*TreeNode{},
	}

	vpcNode := &TreeNode{Address: "aws_vpc.main", Action: "create"}
	subnetPublic := &TreeNode{Address: `aws_subnet.a["public"]`, Action: "create"}
	subnetPrivate := &TreeNode{Address: `aws_subnet.a["private"]`, Action: "create"}
	tree.nodes["aws_vpc.main"] = vpcNode
	tree.nodes[`aws_subnet.a["public"]`] = subnetPublic
	tree.nodes[`aws_subnet.a["private"]`] = subnetPrivate

	plan := &tfjson.Plan{
		Config: &tfjson.Config{
			RootModule: &tfjson.ConfigModule{
				Resources: []*tfjson.ConfigResource{
					{Address: "aws_vpc.main"},
					// Config-level resource address never carries a for_each instance key.
					{Address: "aws_subnet.a", DependsOn: []string{"aws_vpc.main"}},
				},
			},
		},
	}

	buildRelationships(tree, plan)

	assert.Contains(t, tree.Root.Children, vpcNode)
	assert.Contains(t, vpcNode.Children, subnetPublic,
		"for_each instance must attach to its real dependency parent, not fall back to root")
	assert.Contains(t, vpcNode.Children, subnetPrivate,
		"for_each instance must attach to its real dependency parent, not fall back to root")
	assert.Equal(t, vpcNode, subnetPublic.Parent)
	assert.Equal(t, vpcNode, subnetPrivate.Parent)
}

// TestBuildRelationships_DependsOnForEachCollection is a regression test for the mirror
// case of TestBuildRelationships_ForEachInstanceAddress: a resource that depends on an
// *entire* count/for_each resource (Terraform has no syntax to depend on just one
// instance) has a DependsOn/reference address with no instance key at all (e.g.
// "aws_subnet.a"), which never exactly matches any instanced node address in the change
// set (only "aws_subnet.a[\"public\"]" etc. exist). Without resolving it to a
// representative instance, the dependent falls back to root instead of nesting under the
// collection it actually depends on.
func TestBuildRelationships_DependsOnForEachCollection(t *testing.T) {
	tree := &DependencyTree{
		Root:  &TreeNode{Address: "root"},
		nodes: map[string]*TreeNode{},
	}

	subnetPublic := &TreeNode{Address: `aws_subnet.a["public"]`, Action: "create"}
	subnetPrivate := &TreeNode{Address: `aws_subnet.a["private"]`, Action: "create"}
	rtaNode := &TreeNode{Address: "aws_route_table_association.b", Action: "create"}
	tree.nodes[`aws_subnet.a["public"]`] = subnetPublic
	tree.nodes[`aws_subnet.a["private"]`] = subnetPrivate
	tree.nodes["aws_route_table_association.b"] = rtaNode

	plan := &tfjson.Plan{
		Config: &tfjson.Config{
			RootModule: &tfjson.ConfigModule{
				Resources: []*tfjson.ConfigResource{
					{Address: "aws_subnet.a"},
					// Depends on the whole for_each collection, not one instance.
					{Address: "aws_route_table_association.b", DependsOn: []string{"aws_subnet.a"}},
				},
			},
		},
	}

	buildRelationships(tree, plan)

	assert.NotEqual(t, tree.Root, rtaNode.Parent,
		"a dependency on an entire for_each collection must not fall back to root")
	assert.Contains(t, []*TreeNode{subnetPublic, subnetPrivate}, rtaNode.Parent,
		"must anchor under one of the collection's real instances")
	// Deterministic: "private" sorts before "public".
	assert.Equal(t, subnetPrivate, rtaNode.Parent)
}

func TestStripInstanceKey(t *testing.T) {
	tests := []struct {
		name string
		addr string
		want string
	}{
		{name: "no instance key", addr: "aws_vpc.main", want: "aws_vpc.main"},
		{name: "for_each string key", addr: `aws_subnet.a["public"]`, want: "aws_subnet.a"},
		{name: "count numeric key", addr: "aws_subnet.a[0]", want: "aws_subnet.a"},
		{name: "module-nested with instance key", addr: `module.vpc.aws_subnet.a["public"]`, want: "module.vpc.aws_subnet.a"},
		{
			name: "instance key on the module segment itself",
			addr: `module.network["blue"].aws_subnet.main`,
			want: "module.network.aws_subnet.main",
		},
		{
			name: "instance keys on both the module and the resource segments",
			addr: `module.network["blue"].aws_subnet.main["public"]`,
			want: "module.network.aws_subnet.main",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, stripInstanceKey(tt.addr))
		})
	}
}

func TestExtractDependencies_ExplicitDependsOn(t *testing.T) {
	module := &tfjson.ConfigModule{
		Resources: []*tfjson.ConfigResource{
			{Address: "aws_vpc.main"},
			{Address: "aws_subnet.a", DependsOn: []string{"aws_vpc.main", "aws_security_group.default"}},
		},
	}

	dependsOn := make(map[string][]string)
	extractDependencies(module, "", dependsOn)

	require.Contains(t, dependsOn, "aws_subnet.a")
	assert.Contains(t, dependsOn["aws_subnet.a"], "aws_vpc.main")
	assert.Contains(t, dependsOn["aws_subnet.a"], "aws_security_group.default")
}

func TestExtractDependencies_WithPrefix(t *testing.T) {
	module := &tfjson.ConfigModule{
		Resources: []*tfjson.ConfigResource{
			{Address: "aws_subnet.main", DependsOn: []string{"aws_vpc.main"}},
		},
	}

	dependsOn := make(map[string][]string)
	extractDependencies(module, "module.network", dependsOn)

	// Address should be prefixed.
	require.Contains(t, dependsOn, "module.network.aws_subnet.main")
}

func TestExtractDependencies_NestedModules(t *testing.T) {
	module := &tfjson.ConfigModule{
		Resources: []*tfjson.ConfigResource{
			{Address: "aws_vpc.main", DependsOn: []string{"data.aws_availability_zones.available"}},
		},
		ModuleCalls: map[string]*tfjson.ModuleCall{
			"subnet": {
				Module: &tfjson.ConfigModule{
					Resources: []*tfjson.ConfigResource{
						{Address: "aws_subnet.private", DependsOn: []string{"var.vpc_id"}},
					},
				},
			},
		},
	}

	dependsOn := make(map[string][]string)
	extractDependencies(module, "", dependsOn)

	// Both root and nested module resources should be processed.
	// Root resource should have its dependencies.
	require.Contains(t, dependsOn, "aws_vpc.main", "root resource should be processed")
	assert.Equal(t, []string{"data.aws_availability_zones.available"}, dependsOn["aws_vpc.main"])

	// The nested module resource should be prefixed with module name.
	require.Contains(t, dependsOn, "module.subnet.aws_subnet.private", "nested module resource should be prefixed")
	assert.Equal(t, []string{"var.vpc_id"}, dependsOn["module.subnet.aws_subnet.private"])
}

func TestExtractDependencies_NilModule(t *testing.T) {
	dependsOn := make(map[string][]string)
	extractDependencies(nil, "", dependsOn)
	assert.Empty(t, dependsOn)
}

func TestSortChildren_Nil(t *testing.T) {
	// Should not panic on nil.
	sortChildren(nil)
}

func TestSortChildren_NoChildren(t *testing.T) {
	node := &TreeNode{Address: "root"}
	sortChildren(node)
	assert.Empty(t, node.Children)
}

func TestSortChildren_Recursive(t *testing.T) {
	root := &TreeNode{
		Address: "root",
		Children: []*TreeNode{
			{
				Address: "z_vpc",
				Children: []*TreeNode{
					{Address: "z_subnet"},
					{Address: "a_subnet"},
				},
			},
			{Address: "a_instance"},
		},
	}

	sortChildren(root)

	// Root children should be sorted.
	assert.Equal(t, "a_instance", root.Children[0].Address)
	assert.Equal(t, "z_vpc", root.Children[1].Address)
	// Nested children should also be sorted.
	assert.Equal(t, "a_subnet", root.Children[1].Children[0].Address)
	assert.Equal(t, "z_subnet", root.Children[1].Children[1].Address)
}

func TestExtractAttributeChanges_NoChange(t *testing.T) {
	rc := &tfjson.ResourceChange{
		Address: "aws_vpc.main",
		Change:  nil,
	}

	changes := extractAttributeChanges(rc)
	assert.Nil(t, changes)
}

func TestExtractAttributeChanges_Create(t *testing.T) {
	rc := &tfjson.ResourceChange{
		Address: "aws_vpc.main",
		Change: &tfjson.Change{
			Actions: []tfjson.Action{tfjson.ActionCreate},
			Before:  nil,
			After: map[string]interface{}{
				"cidr_block": "10.0.0.0/16",
				"tags":       map[string]interface{}{"Name": "main"},
			},
		},
	}

	changes := extractAttributeChanges(rc)
	require.Len(t, changes, 2)

	// Find cidr_block change.
	var cidrChange *AttributeChange
	for _, c := range changes {
		if c.Key == "cidr_block" {
			cidrChange = c
			break
		}
	}

	require.NotNil(t, cidrChange)
	assert.Nil(t, cidrChange.Before)
	assert.Equal(t, "10.0.0.0/16", cidrChange.After)
}

func TestExtractAttributeChanges_Update(t *testing.T) {
	rc := &tfjson.ResourceChange{
		Address: "aws_vpc.main",
		Change: &tfjson.Change{
			Actions: []tfjson.Action{tfjson.ActionUpdate},
			Before: map[string]interface{}{
				"cidr_block": "10.0.0.0/16",
			},
			After: map[string]interface{}{
				"cidr_block": "10.0.0.0/8",
			},
		},
	}

	changes := extractAttributeChanges(rc)
	require.Len(t, changes, 1)
	assert.Equal(t, "cidr_block", changes[0].Key)
	assert.Equal(t, "10.0.0.0/16", changes[0].Before)
	assert.Equal(t, "10.0.0.0/8", changes[0].After)
}

func TestExtractAttributeChanges_Delete(t *testing.T) {
	rc := &tfjson.ResourceChange{
		Address: "aws_vpc.main",
		Change: &tfjson.Change{
			Actions: []tfjson.Action{tfjson.ActionDelete},
			Before: map[string]interface{}{
				"cidr_block": "10.0.0.0/16",
			},
			After: nil,
		},
	}

	changes := extractAttributeChanges(rc)
	require.Len(t, changes, 1)
	assert.Equal(t, "cidr_block", changes[0].Key)
	assert.Equal(t, "10.0.0.0/16", changes[0].Before)
	assert.Nil(t, changes[0].After)
}

func TestExtractAttributeChanges_UnknownValue(t *testing.T) {
	rc := &tfjson.ResourceChange{
		Address: "aws_instance.web",
		Change: &tfjson.Change{
			Actions: []tfjson.Action{tfjson.ActionCreate},
			Before:  nil,
			After: map[string]interface{}{
				"id": nil,
			},
			AfterUnknown: map[string]interface{}{
				"id": true,
			},
		},
	}

	changes := extractAttributeChanges(rc)
	require.Len(t, changes, 1)
	assert.Equal(t, "id", changes[0].Key)
	assert.True(t, changes[0].Unknown)
}

func TestExtractAttributeChanges_SensitiveValue(t *testing.T) {
	rc := &tfjson.ResourceChange{
		Address: "aws_db_instance.main",
		Change: &tfjson.Change{
			Actions: []tfjson.Action{tfjson.ActionCreate},
			Before:  nil,
			After: map[string]interface{}{
				"password": "secret123",
			},
			AfterSensitive: map[string]interface{}{
				"password": true,
			},
		},
	}

	changes := extractAttributeChanges(rc)
	require.Len(t, changes, 1)
	assert.Equal(t, "password", changes[0].Key)
	assert.True(t, changes[0].Sensitive)
}

func TestExtractAttributeChanges_UnchangedNotIncluded(t *testing.T) {
	rc := &tfjson.ResourceChange{
		Address: "aws_vpc.main",
		Change: &tfjson.Change{
			Actions: []tfjson.Action{tfjson.ActionUpdate},
			Before: map[string]interface{}{
				"cidr_block": "10.0.0.0/16",
				"name":       "unchanged",
			},
			After: map[string]interface{}{
				"cidr_block": "10.0.0.0/8",
				"name":       "unchanged",
			},
		},
	}

	changes := extractAttributeChanges(rc)
	// Only cidr_block changed, name should not be included.
	require.Len(t, changes, 1)
	assert.Equal(t, "cidr_block", changes[0].Key)
}

func TestExtractAttributeChanges_SortedKeys(t *testing.T) {
	rc := &tfjson.ResourceChange{
		Address: "aws_vpc.main",
		Change: &tfjson.Change{
			Actions: []tfjson.Action{tfjson.ActionCreate},
			Before:  nil,
			After: map[string]interface{}{
				"z_attribute": "z",
				"a_attribute": "a",
				"m_attribute": "m",
			},
		},
	}

	changes := extractAttributeChanges(rc)
	require.Len(t, changes, 3)
	// Should be sorted alphabetically.
	assert.Equal(t, "a_attribute", changes[0].Key)
	assert.Equal(t, "m_attribute", changes[1].Key)
	assert.Equal(t, "z_attribute", changes[2].Key)
}
