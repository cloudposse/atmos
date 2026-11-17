package atmosbridge

import (
	"io"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/codegen"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/stretchr/testify/require"
)

func TestDiscoveryClassifiesDeniedEffects(t *testing.T) {
	t.Parallel()
	env := Discovery(Environment(map[string]string{}, io.Discard, io.Discard), false)
	err := env.Exec.Command(t.Context(), "disallowed-program").Run()
	require.ErrorIs(t, err, errDiscoveryExecution)
	require.Contains(t, err.Error(), "disallowed-program")
	data, err := env.Sops.DecryptFile(env.Env, "private.yaml", "yaml")
	require.ErrorIs(t, err, errDiscoveryDecryption)
	require.Nil(t, data)
	require.Contains(t, err.Error(), "private.yaml")
}

func TestRenderGeneratedPreservesCallerDisableSetting(t *testing.T) {
	t.Parallel()
	config := codegen.GenerateConfig{
		Path: "generated.tf", Contents: "locals { value = 1 }\n",
		IfExists: codegen.ExistsOverwrite, Disable: true, DisableSignature: true,
	}
	before := config
	content, err := RenderGenerated(t.Context(), log.New(log.WithOutput(io.Discard)), config)
	require.NoError(t, err)
	require.Contains(t, string(content), "value = 1")
	require.Equal(t, before, config, "in-memory rendering must not enable the caller's disabled generation block")
}
