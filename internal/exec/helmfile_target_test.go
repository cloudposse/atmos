package exec

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/helmfile"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestDeliverHelmfileToTarget(t *testing.T) {
	original := renderAndDeliver
	t.Cleanup(func() { renderAndDeliver = original })

	var captured *helmfile.RenderDeliverInput
	renderAndDeliver = func(_ context.Context, in *helmfile.RenderDeliverInput) (string, error) {
		captured = in
		return "rendered manifests", nil
	}

	info := &schema.ConfigAndStacksInfo{
		Command:                "/bin/helmfile",
		GlobalOptions:          []string{"--no-color"},
		AdditionalArgsAndFlags: []string{"--selector", "name=app"},
		ComponentSection: map[string]any{
			cfg.ProvisionSectionName: map[string]any{
				"default": "deploy-repo",
				"targets": map[string]any{"deploy-repo": map[string]any{"kind": "git"}},
			},
		},
	}

	rendered, err := deliverHelmfileToTarget(&schema.AtmosConfiguration{}, info, helmfileTargetDelivery{
		varFile:       "values.yaml",
		componentPath: "/work/component",
		envVars:       []string{"FOO=bar"},
		flagTarget:    "deploy-repo",
	})
	require.NoError(t, err)
	assert.Equal(t, "rendered manifests", rendered)

	require.NotNil(t, captured)
	// The template args assemble as: --state-values-file <varFile>, global opts,
	// the `template` subcommand, then the remaining helmfile args.
	assert.Equal(t, []string{"--state-values-file", "values.yaml", "--no-color", "template", "--selector", "name=app"}, captured.Args)
	assert.Equal(t, "/bin/helmfile", captured.Command)
	assert.Equal(t, "/work/component", captured.WorkingDir)
	assert.Equal(t, []string{"FOO=bar"}, captured.EnvVars)
	assert.Equal(t, "deploy-repo", captured.FlagTarget)
	require.NotNil(t, captured.ProvisionSection)
	// A non-AuthManager value must not be forwarded as the env provider.
	assert.Nil(t, captured.EnvProvider)
}

func TestDeliverHelmfileToTarget_PropagatesError(t *testing.T) {
	original := renderAndDeliver
	t.Cleanup(func() { renderAndDeliver = original })

	sentinel := errors.New("delivery failed")
	renderAndDeliver = func(context.Context, *helmfile.RenderDeliverInput) (string, error) {
		return "", sentinel
	}

	_, err := deliverHelmfileToTarget(&schema.AtmosConfiguration{}, &schema.ConfigAndStacksInfo{}, helmfileTargetDelivery{})
	require.ErrorIs(t, err, sentinel)
}
