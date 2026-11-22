package auth

import (
	"github.com/cloudposse/atmos/pkg/schema"
)

// redactedValue replaces configured secrets in `atmos auth list` output.
// It matches the placeholder the tree formatter prints for identity credentials.
const redactedValue = "[redacted]"

// structuredListOutput builds the document that the JSON and YAML formats
// marshal, with configured secrets redacted. Empty sections are omitted.
func structuredListOutput(providers map[string]schema.Provider, identities map[string]schema.Identity) map[string]interface{} {
	output := map[string]interface{}{}

	if len(providers) > 0 {
		output[providersKey] = redactProvidersForList(providers)
	}

	if len(identities) > 0 {
		output[identitiesKey] = redactIdentitiesForList(identities)
	}

	return output
}

// redactProvidersForList returns copies of the providers with configured
// secrets (the SAML password) replaced by redactedValue. The input map and its
// values are never modified.
func redactProvidersForList(providers map[string]schema.Provider) map[string]schema.Provider {
	if providers == nil {
		return nil
	}
	redacted := make(map[string]schema.Provider, len(providers))
	for name, provider := range providers {
		if provider.Password != "" {
			provider.Password = redactedValue
		}
		redacted[name] = provider
	}
	return redacted
}

// redactIdentitiesForList returns copies of the identities with every inline
// credential value (for example `aws/user` access keys, already resolved from
// `!env`) replaced by redactedValue. Credential keys stay visible so the output
// still shows which credentials are configured. The input maps are never modified.
func redactIdentitiesForList(identities map[string]schema.Identity) map[string]schema.Identity {
	if identities == nil {
		return nil
	}
	redacted := make(map[string]schema.Identity, len(identities))
	for name, identity := range identities {
		if identity.Credentials != nil {
			credentials := make(map[string]interface{}, len(identity.Credentials))
			for key := range identity.Credentials {
				credentials[key] = redactedValue
			}
			identity.Credentials = credentials
		}
		redacted[name] = identity
	}
	return redacted
}
