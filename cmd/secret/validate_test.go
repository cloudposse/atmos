package secret

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/secrets"
)

func TestRunSecretValidate_Valid(t *testing.T) {
	svc := newFakeSecretService()
	svc.validation = secrets.ValidationResult{} // No missing, no errored → valid.
	installService(t, svc, nil)

	err := runSecretSubcommand(t, "validate", "--stack", "dev", "--component", "api")
	require.NoError(t, err)
}

func TestRunSecretValidate_MissingRequired(t *testing.T) {
	svc := newFakeSecretService()
	svc.validation = secrets.ValidationResult{
		MissingRequired: []secrets.Status{
			{Declaration: secrets.Declaration{Name: "API_KEY", Required: true}},
		},
		Errored: []secrets.Status{
			{Declaration: secrets.Declaration{Name: "DB"}, Err: errors.New("access denied")},
		},
	}
	installService(t, svc, nil)

	err := runSecretSubcommand(t, "validate", "--stack", "dev", "--component", "api")
	require.ErrorIs(t, err, errUtils.ErrValidationFailed)
}

func TestRunSecretValidate_LoadServiceError(t *testing.T) {
	loadErr := errors.New("load failed")
	installService(t, nil, loadErr)

	err := runSecretSubcommand(t, "validate", "--stack", "dev", "--component", "api")
	require.ErrorIs(t, err, loadErr)
}

func TestRunSecretValidate_MissingScope(t *testing.T) {
	svc := newFakeSecretService()
	installService(t, svc, nil)

	err := runSecretSubcommand(t, "validate")
	require.ErrorIs(t, err, errUtils.ErrRequiredFlagNotProvided)
}
