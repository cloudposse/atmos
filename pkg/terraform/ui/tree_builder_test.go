package ui

import (
	"testing"

	tfjson "github.com/hashicorp/terraform-json"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildTreeFromPlan_Empty(t *testing.T) {
	plan := &tfjson.Plan{}

	tree, err := buildTreeFromPlan(plan, "dev", "vpc")
	require.NoError(t, err)
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

	tree, err := buildTreeFromPlan(plan, "dev", "vpc")
	require.NoError(t, err)
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

	tree, err := buildTreeFromPlan(plan, "dev", "vpc")
	require.NoError(t, err)
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

	tree, err := buildTreeFromPlan(plan, "dev", "vpc")
	require.NoError(t, err)
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

	tree, err := buildTreeFromPlan(plan, "dev", "app")
	require.NoError(t, err)
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

	tree, err := buildTreeFromPlan(plan, "dev", "vpc")
	require.NoError(t, err)
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

	tree, err := buildTreeFromPlan(plan, "dev", "vpc")
	require.NoError(t, err)
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

	tree, err := buildTreeFromPlan(plan, "dev", "test")
	require.NoError(t, err)
	require.Len(t, tree.Root.Children, 3)
	assert.Equal(t, "a_resource.first", tree.Root.Children[0].Address)
	assert.Equal(t, "m_resource.middle", tree.Root.Children[1].Address)
	assert.Equal(t, "z_resource.last", tree.Root.Children[2].Address)
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
