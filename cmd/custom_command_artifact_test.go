package cmd

import (
	"errors"
	"testing"
	"time"

	"github.com/go-viper/mapstructure/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifactwire/wiretest"
	"github.com/cloudposse/atmos/pkg/schema"
)

// decodeCustomCommandSteps decodes steps the way atmos.yaml custom commands are decoded, through
// the mapstructure path that Viper's merged configuration takes.
func decodeCustomCommandSteps(t *testing.T, steps ...map[string]any) schema.Tasks {
	t.Helper()
	var holder struct {
		Steps schema.Tasks `mapstructure:"steps"`
	}
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{Result: &holder, WeaklyTypedInput: true, DecodeHook: schema.TasksDecodeHook()})
	require.NoError(t, err)
	items := make([]any, 0, len(steps))
	for _, step := range steps {
		items = append(items, step)
	}
	require.NoError(t, decoder.Decode(map[string]any{"steps": items}))
	for index := range holder.Steps {
		require.NoError(t, holder.Steps[index].LoadError)
	}
	return holder.Steps
}

// runCustomCommand registers a custom command with the steps and runs it.
func runCustomCommand(t *testing.T, f *wiretest.Fixture, name string, steps schema.Tasks) {
	t.Helper()
	config := *f.Config
	config.BasePath = t.TempDir()
	config.Commands = []schema.Command{{Name: name, WorkingDirectory: config.BasePath, Steps: steps}}
	require.NoError(t, processCustomCommands(config, config.Commands, RootCmd))
	command, _, err := RootCmd.Find([]string{name})
	require.NoError(t, err)
	command.Run(command, nil)
}

func TestCustomCommandArtifactStepMatchesTheSharedImplementation(t *testing.T) {
	_ = NewTestKit(t)
	f := wiretest.New(t)
	fields := f.StepFields()
	fields["type"] = "artifact"
	fields["name"] = "publish"

	runCustomCommand(t, f, "artifact-publish", decodeCustomCommandSteps(t, fields))

	results := f.Recorder.Results()
	require.Len(t, results, 1)
	baseline := f.Baseline(t)
	assert.Equal(t, f.Normalize(&baseline.OperationResult), f.Normalize(&results[0]), "same result as the CLI implementation")
	assert.Equal(t, artifact.OutcomeCreated, results[0].Outcome)
	assert.Contains(t, results[0].Location, f.Root)

	requests := f.Recorder.Requests()
	require.Len(t, requests, 1)
	assert.Equal(t, f.Source, requests[0].Source)
	assert.Equal(t, map[string]string{"team": "platform"}, requests[0].Metadata.Labels)
	assert.Equal(t, wiretest.OperationID, requests[0].Execution.OperationID)
}

func TestCustomCommandArtifactStepRetryKeepsTheOperationIdentity(t *testing.T) {
	_ = NewTestKit(t)
	f := wiretest.New(t)
	f.Recorder.FailPublications(2, errors.New("temporary registry failure"))
	fields := f.StepFields()
	fields["type"] = "artifact"
	fields["name"] = "publish"
	fields["retry"] = map[string]any{"max_attempts": 3, "backoff_strategy": "constant", "initial_delay": time.Millisecond.String()}

	runCustomCommand(t, f, "artifact-retry", decodeCustomCommandSteps(t, fields))

	requests := f.Recorder.Requests()
	require.Len(t, requests, 3)
	for _, request := range requests {
		assert.Equal(t, requests[0].Execution, request.Execution, "every attempt is the same logical operation")
		assert.Equal(t, 1, request.Execution.MaxAttempts)
	}
	results := f.Recorder.Results()
	assert.Equal(t, artifact.OutcomeCreated, results[2].Outcome)
}
