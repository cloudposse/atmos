package secret

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/secrets"
)

func TestRunSecretExec_NoCommand(t *testing.T) {
	svc := newFakeSecretService()
	installServiceAndConfig(t, svc, nil, nil)

	// No "--" separator → no command to run.
	err := runSecretSubcommand(t, "exec", "--stack", "dev", "--component", "api")
	require.ErrorIs(t, err, errUtils.ErrNoCommandSpecified)
}

func TestRunSecretExec_Happy(t *testing.T) {
	setupIO(t)
	svc := newFakeSecretService()
	svc.declarations = []secrets.Declaration{{Name: "API_KEY"}, {Name: "DB"}}
	svc.getValues["API_KEY"] = "s3cr3t"
	svc.getValues["DB"] = "pw"
	installServiceAndConfig(t, svc, &schema.AtmosConfiguration{}, nil)
	gotArgs, gotEnv := overrideRunCommand(t, nil)

	err := runSecretSubcommand(t, "exec", "--stack", "dev", "--component", "api", "--", "dummy-cmd", "--flag")
	require.NoError(t, err)

	assert.Equal(t, []string{"dummy-cmd", "--flag"}, *gotArgs)
	// Injected secrets are present in the child environment.
	assert.Contains(t, *gotEnv, "API_KEY=s3cr3t")
	assert.Contains(t, *gotEnv, "DB=pw")
}

func TestRunSecretExec_RunCommandError(t *testing.T) {
	setupIO(t)
	svc := newFakeSecretService()
	svc.declarations = []secrets.Declaration{{Name: "API_KEY"}}
	svc.getValues["API_KEY"] = "s3cr3t"
	installServiceAndConfig(t, svc, &schema.AtmosConfiguration{}, nil)
	runErr := errors.New("command failed")
	overrideRunCommand(t, runErr)

	err := runSecretSubcommand(t, "exec", "--stack", "dev", "--component", "api", "--", "dummy-cmd")
	require.ErrorIs(t, err, runErr)
}

func TestRunSecretExec_LoadServiceError(t *testing.T) {
	loadErr := errors.New("load failed")
	installServiceAndConfig(t, nil, nil, loadErr)

	err := runSecretSubcommand(t, "exec", "--stack", "dev", "--component", "api", "--", "dummy-cmd")
	require.ErrorIs(t, err, loadErr)
}
