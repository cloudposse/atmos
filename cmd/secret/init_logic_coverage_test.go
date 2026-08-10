package secret

import (
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/secrets"
)

func TestInitSummaryAddAndOutcomeForStatus(t *testing.T) {
	var summary initSummary
	summary.add(initOutcomeInitialized)
	summary.add(initOutcomeRotated)
	summary.add(initOutcomeUnaffected)

	assert.Equal(t, initSummary{initialized: 1, rotated: 1, unaffected: 1}, summary)
	assert.Equal(t, initOutcomeInitialized, initOutcomeForStatus(false))
	assert.Equal(t, initOutcomeRotated, initOutcomeForStatus(true))
}

func TestValidateInitInput(t *testing.T) {
	_, stderr := setupIOCapture(t)
	api := newFakeSecretService()
	api.declared["API_KEY"] = true
	database := newFakeSecretService()
	database.declared["DATABASE_URL"] = true

	assert.NoError(t, validateInitInput([]secretService{api}, nil, "warn"))
	require.ErrorIs(t, validateInitInput([]secretService{api}, map[string]string{"API_KEY": "value"}, "unknown"), errUtils.ErrValidationFailed)
	assert.NoError(t, validateInitInput([]secretService{api, database}, map[string]string{
		"API_KEY":      "value",
		"DATABASE_URL": "postgres://example",
	}, "warn"))
	require.ErrorIs(t, validateInitInput([]secretService{api}, map[string]string{"UNDECLARED": "value"}, "strict"), errUtils.ErrValidationFailed)
	assert.NoError(t, validateInitInput([]secretService{api}, map[string]string{"UNDECLARED": "value"}, "warn"))
	assert.Contains(t, stripANSI(stderr.String()), "Skipping undeclared input key UNDECLARED")
}

func TestProvisionSecretWithInput(t *testing.T) {
	setupIO(t)
	svc := newFakeSecretService()

	missingValue := &secrets.Status{Declaration: secrets.Declaration{Name: "MISSING"}}
	outcome, err := provisionSecret(svc, missingValue, initOptions{values: map[string]string{"OTHER": "value"}, mode: "warn"})
	require.NoError(t, err)
	assert.Equal(t, initOutcomeUnaffected, outcome)
	assert.Empty(t, svc.setCalls)

	provided := &secrets.Status{Declaration: secrets.Declaration{Name: "API_KEY"}}
	outcome, err = provisionSecret(svc, provided, initOptions{values: map[string]string{"API_KEY": "from-env"}, mode: "warn"})
	require.NoError(t, err)
	assert.Equal(t, initOutcomeInitialized, outcome)
	require.Len(t, svc.setCalls, 1)
	assert.Equal(t, "from-env", svc.setCalls[0].value)

	alreadySet := &secrets.Status{Declaration: secrets.Declaration{Name: "API_KEY"}, Initialized: true}
	outcome, err = provisionSecret(svc, alreadySet, initOptions{values: map[string]string{"API_KEY": "new-value"}, mode: "warn"})
	require.NoError(t, err)
	assert.Equal(t, initOutcomeUnaffected, outcome)
	assert.Len(t, svc.setCalls, 1, "dotenv input never rotates an initialized value unless forced")

	outcome, err = provisionSecret(svc, alreadySet, initOptions{dryRun: true, values: map[string]string{"API_KEY": "new-value"}, mode: "warn"})
	require.NoError(t, err)
	assert.Equal(t, initOutcomeRotated, outcome)
	assert.Len(t, svc.setCalls, 1, "dry-run never writes")

	outcome, err = provisionSecret(svc, alreadySet, initOptions{force: true, values: map[string]string{"API_KEY": "new-value"}, mode: "warn"})
	require.NoError(t, err)
	assert.Equal(t, initOutcomeRotated, outcome)
	assert.Len(t, svc.setCalls, 2, "force permits dotenv rotation")
}

func TestProvisionSecretPromptAndWriteFailures(t *testing.T) {
	setupIO(t)
	status := &secrets.Status{Declaration: secrets.Declaration{Name: "API_KEY"}}

	t.Run("prompt failure", func(t *testing.T) {
		svc := newFakeSecretService()
		sentinel := errors.New("prompt failed")
		overridePromptForValue(t, "", sentinel)

		outcome, err := provisionSecret(svc, status, initOptions{mode: "warn"})
		require.ErrorIs(t, err, sentinel)
		assert.Equal(t, initOutcomeUnaffected, outcome)
	})

	t.Run("write failure", func(t *testing.T) {
		svc := newFakeSecretService()
		svc.setErr = errors.New("write failed")
		overridePromptForValue(t, "value", nil)

		outcome, err := provisionSecret(svc, status, initOptions{mode: "warn"})
		require.ErrorIs(t, err, svc.setErr)
		assert.Equal(t, initOutcomeUnaffected, outcome)
	})
}

func TestRotateSecretStatusesDeduplicatesAndSummarizes(t *testing.T) {
	setupIO(t)
	svc := newFakeSecretService()
	statuses := []secrets.Status{
		{Declaration: secrets.Declaration{Name: "SHARED", Scope: secrets.ScopeStack}},
		{Declaration: secrets.Declaration{Name: "INSTANCE", Scope: secrets.ScopeInstance}, Initialized: true},
	}
	seen := map[string]bool{"prod\x00SHARED": true}

	summary, err := rotateSecretStatuses(svc, statuses, "prod", seen, initOptions{dryRun: true, mode: "warn"})
	require.NoError(t, err)
	assert.Equal(t, initSummary{rotated: 1}, summary)
}

func TestReadInitInputTTYAndEmptyPipe(t *testing.T) {
	originalIsTTY := initStdinIsTTY
	t.Cleanup(func() { initStdinIsTTY = originalIsTTY })

	initStdinIsTTY = func() bool { return true }
	values, err := readInitInput("")
	require.NoError(t, err)
	assert.Nil(t, values)

	r, w, err := os.Pipe()
	require.NoError(t, err)
	originalStdin := os.Stdin
	os.Stdin = r
	t.Cleanup(func() {
		_ = r.Close()
		os.Stdin = originalStdin
	})
	require.NoError(t, w.Close())
	initStdinIsTTY = func() bool { return false }

	values, err = readInitInput("")
	require.NoError(t, err)
	assert.Nil(t, values)
}
