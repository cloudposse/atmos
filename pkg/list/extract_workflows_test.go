package list

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestExtractFromManifest(t *testing.T) {
	manifest := schema.WorkflowManifest{
		Name: "deploy-workflows",
		Workflows: map[string]schema.WorkflowDefinition{
			"deploy-all": {
				Description: "Deploy all components",
				Steps: []schema.WorkflowStep{
					{Name: "step1"},
					{Name: "step2"},
				},
			},
			"destroy-all": {
				Description: "Destroy all components",
				Steps: []schema.WorkflowStep{
					{Name: "step1"},
				},
			},
		},
	}

	workflows := extractFromManifest(manifest)
	require.Len(t, workflows, 2)

	// Verify structure.
	for _, wf := range workflows {
		assert.Contains(t, wf, "file")
		assert.Contains(t, wf, "workflow")
		assert.Contains(t, wf, "description")
		assert.Contains(t, wf, "steps")
		assert.Equal(t, "deploy-workflows", wf["file"])
	}

	// Find deploy-all workflow.
	var deployAll map[string]any
	for _, wf := range workflows {
		if wf["workflow"] == "deploy-all" {
			deployAll = wf
			break
		}
	}

	require.NotNil(t, deployAll)
	assert.Equal(t, "deploy-all", deployAll["workflow"])
	assert.Equal(t, "Deploy all components", deployAll["description"])
	assert.Equal(t, 2, deployAll["steps"])
}

func TestExtractFromManifest_EmptyWorkflows(t *testing.T) {
	manifest := schema.WorkflowManifest{
		Name:      "empty-workflows",
		Workflows: nil,
	}

	workflows := extractFromManifest(manifest)
	assert.Empty(t, workflows)
}

func TestExtractFromManifest_NoDescription(t *testing.T) {
	manifest := schema.WorkflowManifest{
		Name: "test-workflows",
		Workflows: map[string]schema.WorkflowDefinition{
			"test": {
				Description: "",
				Steps:       []schema.WorkflowStep{},
			},
		},
	}

	workflows := extractFromManifest(manifest)
	require.Len(t, workflows, 1)

	assert.Equal(t, "", workflows[0]["description"])
	assert.Equal(t, 0, workflows[0]["steps"])
}

func TestExtractFromManifest_MultipleWorkflows(t *testing.T) {
	manifest := schema.WorkflowManifest{
		Name: "multi-workflows",
		Workflows: map[string]schema.WorkflowDefinition{
			"wf1": {Description: "Workflow 1", Steps: []schema.WorkflowStep{{Name: "s1"}}},
			"wf2": {Description: "Workflow 2", Steps: []schema.WorkflowStep{{Name: "s1"}, {Name: "s2"}}},
			"wf3": {Description: "Workflow 3", Steps: []schema.WorkflowStep{{Name: "s1"}, {Name: "s2"}, {Name: "s3"}}},
		},
	}

	workflows := extractFromManifest(manifest)
	assert.Len(t, workflows, 3)

	// Verify all have file field.
	for _, wf := range workflows {
		assert.Equal(t, "multi-workflows", wf["file"])
	}
}
