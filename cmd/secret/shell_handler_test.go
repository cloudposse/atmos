package secret

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/secrets"
)

func TestRunSecretShell_Happy(t *testing.T) {
	setupIO(t)
	svc := newFakeSecretService()
	svc.declarations = []secrets.Declaration{{Name: "API_KEY"}}
	svc.getValues["API_KEY"] = "s3cr3t"
	installServiceAndConfig(t, svc, &schema.AtmosConfiguration{}, nil)
	gotCmd, gotArgs, gotEnv := overrideStartShell(t, nil)

	err := runSecretSubcommand(t, "shell", "--stack", "dev", "--component", "api")
	require.NoError(t, err)

	// A shell binary is resolved (from $SHELL/bash/sh) and launched.
	assert.NotEmpty(t, *gotCmd)
	// No `--` args were passed, so only shell.Determine's own default args (e.g. a login flag)
	// reach the shell — none of our own tokens leak through.
	assert.NotContains(t, *gotArgs, "bash")
	// The resolved secret is injected into the shell environment.
	assert.Contains(t, *gotEnv, "API_KEY=s3cr3t")
}

func TestRunSecretShell_StartShellError(t *testing.T) {
	setupIO(t)
	svc := newFakeSecretService()
	svc.declarations = []secrets.Declaration{{Name: "API_KEY"}}
	svc.getValues["API_KEY"] = "s3cr3t"
	installServiceAndConfig(t, svc, &schema.AtmosConfiguration{}, nil)
	startErr := errors.New("shell exited non-zero")
	overrideStartShell(t, startErr)

	err := runSecretSubcommand(t, "shell", "--stack", "dev", "--component", "api")
	require.ErrorIs(t, err, startErr)
}

func TestRunSecretShell_LoadServiceError(t *testing.T) {
	loadErr := errors.New("load failed")
	installServiceAndConfig(t, nil, nil, loadErr)

	err := runSecretSubcommand(t, "shell", "--stack", "dev", "--component", "api")
	require.ErrorIs(t, err, loadErr)
}

func TestRunSecretShell_InvalidArgs(t *testing.T) {
	svc := newFakeSecretService()
	installServiceAndConfig(t, svc, nil, nil)

	// A positional arg before "--" is rejected by validateShellArgs.
	err := runSecretSubcommand(t, "shell", "bash", "--stack", "dev", "--component", "api")
	require.Error(t, err)
}
