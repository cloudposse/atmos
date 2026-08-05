package composition

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func compositions() map[string]schema.Composition {
	return map[string]schema.Composition{
		"storefront": {
			Description: "Storefront system",
			Services:    []string{"api", "worker", "database"},
		},
	}
}

func TestValidateMembership(t *testing.T) {
	comps := compositions()

	// Valid membership.
	require.NoError(t, ValidateMembership("api", "storefront", comps))
	// Empty composition is valid (no membership).
	require.NoError(t, ValidateMembership("anything", "", comps))

	// Unknown composition is a hard error.
	err := ValidateMembership("api", "nope", comps)
	require.ErrorIs(t, err, errUtils.ErrUnknownComposition)

	// Member not declared in services is a hard error.
	err = ValidateMembership("frontend", "storefront", comps)
	require.ErrorIs(t, err, errUtils.ErrUnknownCompositionMembership)
}

func TestValidate_Report(t *testing.T) {
	comps := compositions()

	// `api` and `worker` are provided; `database` is declared but not provided.
	report, err := Validate("storefront", []string{"api", "worker"}, comps)
	require.NoError(t, err)
	assert.Equal(t, "storefront", report.Composition)
	assert.Equal(t, []string{"api", "worker"}, report.Fulfilled)
	assert.Equal(t, []string{"database"}, report.NotProvided)
	assert.Empty(t, report.Unknown)
}

func TestValidate_UnknownMember(t *testing.T) {
	comps := compositions()

	report, err := Validate("storefront", []string{"api", "frontend"}, comps)
	require.NoError(t, err)
	assert.Equal(t, []string{"frontend"}, report.Unknown)
	assert.Equal(t, []string{"api"}, report.Fulfilled)
}

func TestValidate_UnknownComposition(t *testing.T) {
	_, err := Validate("missing", nil, compositions())
	require.ErrorIs(t, err, errUtils.ErrUnknownComposition)
}
