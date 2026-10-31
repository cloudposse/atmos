package artifacts

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/invoke"
	"github.com/cloudposse/atmos/pkg/artifactwire/wiretest"
	runnerstep "github.com/cloudposse/atmos/pkg/runner/step"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestCLIAndArtifactStepReportTheSamePublication runs the same publication through the real
// command and through the artifact step against real local repositories, and compares the
// structured output of the command with the step's typed result and named outputs.
func TestCLIAndArtifactStepReportTheSamePublication(t *testing.T) {
	f := wiretest.New(t)
	h, _ := newHarness(t)
	atmosConfigPtr = f.Config
	newService = func(*schema.AtmosConfiguration, ServiceOptions) (artifact.Service, error) { return f.Recorder, nil }

	cli := h.run("publish", f.Source, "--repository="+wiretest.Alias, "--name="+wiretest.Name, "--version="+wiretest.Version,
		"--labels=team=platform", "--format=json")
	require.NoError(t, cli.err)
	var fromCLI invoke.Result
	require.NoError(t, json.Unmarshal([]byte(cli.stdout), &fromCLI))

	// The step publishes to a sibling repository, so it is a fresh creation with the same content.
	sibling := wiretest.New(t)
	vars := runnerstep.NewVariables()
	vars.SetAtmosConfig(sibling.Config)
	step := &schema.WorkflowStep{
		Name: "publish", Type: schema.TaskTypeArtifact, Action: "publish", Source: sibling.Source, Repository: wiretest.Alias,
		ArtifactName: wiretest.Name, Version: wiretest.Version, Metadata: &schema.ArtifactStepMetadata{Labels: map[string]string{"team": "platform"}},
	}
	result, err := (&runnerstep.ArtifactHandler{}).Execute(context.Background(), step, vars)
	require.NoError(t, err)
	fromStep, ok := result.Metadata[runnerstep.ArtifactMetadataKey].(*invoke.Result)
	require.True(t, ok)

	assert.Equal(t, f.Normalize(&fromCLI.OperationResult), sibling.Normalize(&fromStep.OperationResult), "the command and the step report the same result")
	assert.Equal(t, fromCLI.Record, fromStep.Record)

	var named map[string]any
	require.NoError(t, json.Unmarshal([]byte(cli.stdout), &named))
	for name, value := range result.Outputs {
		if name == "location" {
			continue // Rooted in each fixture's own repository directory.
		}
		if name == "size" {
			assert.EqualValues(t, fromCLI.Size, mustSize(t, value), "size")
			continue
		}
		assert.Equal(t, value, named[name], "named output %q has the same name and value in the command's structured output", name)
	}
	assert.Contains(t, named, "location")
}

func mustSize(t *testing.T, value string) int64 {
	t.Helper()
	var size int64
	require.NoError(t, json.Unmarshal([]byte(value), &size))
	return size
}
