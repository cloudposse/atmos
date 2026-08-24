package secret

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/secrets"
)

func TestRunSecretList_Happy(t *testing.T) {
	setupIO(t)
	svc := newFakeSecretService()
	svc.statuses = []secrets.Status{
		{
			Declaration: secrets.Declaration{Name: "API_KEY", BackendType: secrets.BackendSops, BackendName: "dev-sops"},
			Initialized: true,
		},
		{
			Declaration: secrets.Declaration{Name: "DB_PASSWORD", BackendType: secrets.BackendSops, BackendName: "dev-sops"},
			Initialized: false,
		},
	}
	installService(t, svc, nil)

	err := runSecretSubcommand(t, "list", "--format", "json", "--stack", "dev", "--component", "api")
	require.NoError(t, err)
}

func TestRunSecretList_Empty(t *testing.T) {
	setupIO(t)
	svc := newFakeSecretService()
	installService(t, svc, nil)

	err := runSecretSubcommand(t, "list", "--stack", "dev", "--component", "api")
	require.NoError(t, err)
}

func TestRunSecretList_Verbose(t *testing.T) {
	setupIO(t)
	svc := newFakeSecretService()
	svc.statuses = []secrets.Status{
		{Declaration: secrets.Declaration{Name: "API_KEY", Description: "key"}, Initialized: true},
	}
	installService(t, svc, nil)

	err := runSecretSubcommand(t, "list", "--verbose", "--format", "csv", "--stack", "dev", "--component", "api")
	require.NoError(t, err)
}

func TestRunSecretList_LoadServiceError(t *testing.T) {
	setupIO(t)
	loadErr := errors.New("load failed")
	installService(t, nil, loadErr)

	err := runSecretSubcommand(t, "list", "--stack", "dev", "--component", "api")
	require.ErrorIs(t, err, loadErr)
}
