package secret

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunSecretGet_Formats(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "text default", args: []string{"get", "API_KEY", "--stack", "dev", "--component", "api"}},
		{name: "json format", args: []string{"get", "API_KEY", "--format", "json", "--stack", "dev", "--component", "api"}},
		{name: "env format", args: []string{"get", "API_KEY", "--format", "env", "--stack", "dev", "--component", "api"}},
		{name: "raw", args: []string{"get", "API_KEY", "--raw", "--stack", "dev", "--component", "api"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupIO(t)
			svc := newFakeSecretService()
			svc.getValues["API_KEY"] = "s3cr3t"
			installService(t, svc, nil)

			err := runSecretSubcommand(t, tt.args...)
			require.NoError(t, err)

			require.Len(t, svc.getCalls, 1)
			assert.Equal(t, "API_KEY", svc.getCalls[0].name)
		})
	}
}

func TestRunSecretGet_PathPropagated(t *testing.T) {
	setupIO(t)
	svc := newFakeSecretService()
	svc.getValues["DB"] = "host"
	installService(t, svc, nil)

	err := runSecretSubcommand(t, "get", "DB", "--path", ".host", "--stack", "dev", "--component", "api")
	require.NoError(t, err)

	require.Len(t, svc.getCalls, 1)
	assert.Equal(t, ".host", svc.getCalls[0].opts.Path)
}

func TestRunSecretGet_RawFormatConflict(t *testing.T) {
	svc := newFakeSecretService()
	installService(t, svc, nil)

	err := runSecretSubcommand(t, "get", "API_KEY", "--raw", "--format", "json", "--stack", "dev", "--component", "api")
	require.ErrorIs(t, err, ErrRawFormatConflict)
	assert.Empty(t, svc.getCalls)
}

func TestRunSecretGet_GetError(t *testing.T) {
	svc := newFakeSecretService()
	svc.getErr = errors.New("not initialized")
	installService(t, svc, nil)

	err := runSecretSubcommand(t, "get", "API_KEY", "--stack", "dev", "--component", "api")
	require.ErrorIs(t, err, svc.getErr)
	require.Len(t, svc.getCalls, 1)
}

func TestRunSecretGet_LoadServiceError(t *testing.T) {
	loadErr := errors.New("load failed")
	installService(t, nil, loadErr)

	err := runSecretSubcommand(t, "get", "API_KEY", "--stack", "dev", "--component", "api")
	require.ErrorIs(t, err, loadErr)
}
