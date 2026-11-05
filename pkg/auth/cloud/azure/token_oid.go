package azure

import "github.com/cloudposse/atmos/pkg/perf"

// ExtractObjectIDFromToken returns the Azure AD object id (the `oid` claim) of the
// principal that a management access token was issued for. PIM role activation needs
// the principal object id to scope eligibility and self-activation requests, and the
// id travels inside the token the parent identity already acquired, so no extra
// directory call is required.
//
// It wraps the package-internal JWT claim extractor so callers outside this package
// (the azure/pim-role identity) can resolve the principal without duplicating the
// JWT-decoding logic.
func ExtractObjectIDFromToken(token string) (string, error) {
	defer perf.Track(nil, "azure.ExtractObjectIDFromToken")()

	return extractOIDFromToken(token)
}
