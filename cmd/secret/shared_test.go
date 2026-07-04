package secret

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func TestParseScope_MissingComponent(t *testing.T) {
	svc := newFakeSecretService()
	installService(t, svc, nil)

	// --stack present but --component absent → parseScope rejects on the component check (the
	// existing MissingScope tests omit both flags and stop at the stack check).
	err := runSecretSubcommand(t, "set", "API_KEY=v1", "--stack", "dev")
	require.ErrorIs(t, err, errUtils.ErrRequiredFlagNotProvided)
	assert.Empty(t, svc.setCalls)
}

// TestCredentialFreeSkip pins the set of YAML functions that credential-free secret listing must
// skip. These all perform an authenticated backend read; if any were evaluated with auth disabled
// (as `secret list` does) the read would fall back to the default AWS chain and fail at the EC2
// IMDS endpoint. Referencing the u.AtmosYamlFunc* constants makes a rename a compile error here.
func TestCredentialFreeSkip(t *testing.T) {
	got := credentialFreeSkip()

	want := []string{
		strings.TrimPrefix(u.AtmosYamlFuncSecret, "!"),
		strings.TrimPrefix(u.AtmosYamlFuncStore, "!"),
		strings.TrimPrefix(u.AtmosYamlFuncStoreGet, "!"),
		strings.TrimPrefix(u.AtmosYamlFuncTerraformOutput, "!"),
		strings.TrimPrefix(u.AtmosYamlFuncTerraformState, "!"),
	}
	require.ElementsMatch(t, want, got, "credentialFreeSkip must skip every credentialed read function")

	// skipFunc compares against the tag with the "!" trimmed, so tokens must be bare.
	for _, tok := range got {
		require.NotEmpty(t, tok)
		assert.Falsef(t, strings.HasPrefix(tok, "!"), "skip token %q must not include the ! prefix", tok)
	}

	// The two functions that triggered the IMDS-fallback regression must be present.
	assert.Contains(t, got, "terraform.state")
	assert.Contains(t, got, "terraform.output")
}
