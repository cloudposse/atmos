package component

import (
	"bytes"
	"context"
	"testing"

	metricsprocess "github.com/cloudposse/atmos/pkg/metrics/process"
	"github.com/cloudposse/atmos/pkg/process"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecutionOptionsPreserveStreamsAndReporting(t *testing.T) {
	var stdout, stderr, override, metadataOut, metadataErr bytes.Buffer
	command := &cobra.Command{Use: "plan"}
	var metrics *metricsprocess.ProcessMetrics
	parser := func(command string, code int, output string) any { return []any{command, code, output} }
	options := []ExecutionOption{
		WithInvokingCommand(command), WithExecMetadataParser(parser),
		WithMetricsCallback(func(value *metricsprocess.ProcessMetrics) { metrics = value }),
		WithStdoutCapture(&stdout), WithStderrCapture(&stderr), WithStdoutOverride(&override),
		WithMetadataOutputCapture(&metadataOut, &metadataErr),
		WithProcessStreams(process.Streams{Stdin: &bytes.Buffer{}, Stdout: &stdout, Stderr: &stderr}),
		WithProcessContext(t.Context()), WithEnvironment([]string{"TOKEN=component"}),
	}
	var resolved ExecutionOptions
	for _, option := range options {
		option(&resolved)
	}
	require.Same(t, &stdout, resolved.StdoutCapture)
	require.Same(t, &stderr, resolved.StderrCapture)
	require.Same(t, &override, resolved.StdoutOverride)
	require.Same(t, &stdout, resolved.Streams.Stdout)
	require.Same(t, &stderr, resolved.Streams.Stderr)
	require.NotNil(t, resolved.Streams.Stdin)
	assert.Equal(t, []string{"TOKEN=component"}, resolved.ProcessEnv)
	require.Same(t, command, InvokingCommandFromOptions(options...))
	assert.Equal(t, []any{"plan", 2, "changes"}, MetadataParserFromOptions(options...)("plan", 2, "changes"))
	out, errOut := MetadataOutputCaptureFromOptions(options...)
	require.Same(t, &metadataOut, out)
	require.Same(t, &metadataErr, errOut)
	require.Same(t, t.Context(), ContextFromOptions(options...))
	value := &metricsprocess.ProcessMetrics{}
	resolved.MetricsCallback(value)
	require.Same(t, value, metrics)
}

func TestExecutionOptionPrecedenceAndEmptyEnvironment(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	options := []ExecutionOption{WithProcessContext(context.Background()), WithProcessContext(ctx)}
	cancel()
	require.ErrorIs(t, ContextFromOptions(options...).Err(), context.Canceled)
	assert.Equal(t, context.Background(), ContextFromOptions())
	assert.Nil(t, InvokingCommandFromOptions())
	assert.Nil(t, MetadataParserFromOptions())
	out, errOut := MetadataOutputCaptureFromOptions()
	assert.Nil(t, out)
	assert.Nil(t, errOut)
	var resolved ExecutionOptions
	WithEnvironment([]string{})(&resolved)
	require.NotNil(t, resolved.ProcessEnv, "an explicitly empty environment must not become ambient inheritance")
	WithEnvironment(nil)(&resolved)
	assert.Nil(t, resolved.ProcessEnv)
}
