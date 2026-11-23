package auth

import (
	"regexp"
	"strings"

	"github.com/cloudposse/atmos/pkg/schema"
)

// redactedValue replaces configured secrets in `atmos auth list` output.
// It matches the placeholder the tree formatter prints for identity credentials.
const redactedValue = "[redacted]"

// secretListKeyFragments is the denylist of key-name fragments whose values are
// secrets. A key is secret when its lower-cased name, with `-` read as `_`,
// contains any fragment, for example `secret_access_key`, `client_secret`,
// `session_token`, `refresh_token`, `password`, `access_key_id`, and
// `private_key`. Everything else (region, endpoint URLs, ARNs, durations, flags
// such as `webflow_enabled`) is configuration and is shown unchanged.
var secretListKeyFragments = []string{
	"secret",
	"password",
	"passwd",
	"passphrase",
	"token",
	"access_key",
	"private_key",
	"api_key",
	"apikey",
	"client_assertion",
}

// awsAccessKeyIDPattern matches an AWS access key ID (long-lived AKIA or temporary
// ASIA), which is redacted whatever key it appears under.
var awsAccessKeyIDPattern = regexp.MustCompile(`^(AKIA|ASIA)[A-Z0-9]{16}$`)

// isSecretListKey reports whether a configuration key names a secret value.
func isSecretListKey(key string) bool {
	normalized := strings.ReplaceAll(strings.ToLower(key), "-", "_")
	for _, fragment := range secretListKeyFragments {
		if strings.Contains(normalized, fragment) {
			return true
		}
	}
	return false
}

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
// secrets (the SAML password and secret `spec` values) replaced by
// redactedValue. The input map and its values are never modified.
func redactProvidersForList(providers map[string]schema.Provider) map[string]schema.Provider {
	redacted := make(map[string]schema.Provider, len(providers))
	for name := range providers {
		provider := providers[name]
		if provider.Password != "" {
			provider.Password = redactedValue
		}
		provider.Spec = redactSecretMap(provider.Spec)
		redacted[name] = provider
	}
	return redacted
}

// redactIdentitiesForList returns copies of the identities with secret values
// replaced by redactedValue: inline credential secrets (for example `aws/user`
// access keys, already resolved from `!env`), secret `spec` values, and `env`
// entries whose key names a secret. Non-secret values such as `region` or
// `webflow_enabled` are shown unchanged and keep their type. Credential keys
// stay visible so the output still shows which credentials are configured.
// The input maps are never modified.
func redactIdentitiesForList(identities map[string]schema.Identity) map[string]schema.Identity {
	redacted := make(map[string]schema.Identity, len(identities))
	for name := range identities {
		identity := identities[name]
		identity.Credentials = redactSecretMap(identity.Credentials)
		identity.Spec = redactSecretMap(identity.Spec)
		identity.Env = redactSecretEnv(identity.Env)
		redacted[name] = identity
	}
	return redacted
}

// redactSecretMap returns a deep copy of m with secret values redacted. It returns
// nil for a nil map so omitted sections stay omitted.
func redactSecretMap(m map[string]interface{}) map[string]interface{} {
	if m == nil {
		return nil
	}
	out := make(map[string]interface{}, len(m))
	for key, value := range m {
		out[key] = redactSecretValue(isSecretListKey(key), value)
	}
	return out
}

// redactSecretValue redacts value when it is a secret: a string under a secret
// key, or a string shaped like an AWS access key ID. Maps and lists are copied and
// redacted recursively; booleans, numbers, and other scalars keep their value and
// type because they are not secrets.
func redactSecretValue(secretKey bool, value interface{}) interface{} {
	switch typed := value.(type) {
	case string:
		return redactSecretString(secretKey, typed)
	case map[string]interface{}:
		return redactSecretNestedMap(secretKey, typed)
	case []interface{}:
		return redactSecretList(secretKey, typed)
	default:
		return value
	}
}

// redactSecretString redacts a non-empty string under a secret key or shaped like an AWS access key ID.
func redactSecretString(secretKey bool, value string) string {
	if value != "" && (secretKey || awsAccessKeyIDPattern.MatchString(value)) {
		return redactedValue
	}
	return value
}

// redactSecretNestedMap copies and redacts a nested map. Every string under a secret
// key is secret, e.g. `token: {value: ...}`.
func redactSecretNestedMap(secretKey bool, m map[string]interface{}) map[string]interface{} {
	nested := redactSecretMap(m)
	if secretKey {
		for key, nestedValue := range nested {
			nested[key] = redactSecretValue(true, nestedValue)
		}
	}
	return nested
}

// redactSecretList copies and redacts a list of values.
func redactSecretList(secretKey bool, items []interface{}) []interface{} {
	list := make([]interface{}, len(items))
	for i, item := range items {
		list[i] = redactSecretValue(secretKey, item)
	}
	return list
}

// redactSecretEnv returns a copy of env with the values of secret-named
// variables (for example `VAULT_TOKEN`) redacted.
func redactSecretEnv(env []schema.EnvironmentVariable) []schema.EnvironmentVariable {
	if env == nil {
		return nil
	}
	out := make([]schema.EnvironmentVariable, len(env))
	for i := range env {
		out[i] = env[i]
		out[i].Value = redactSecretValue(isSecretListKey(env[i].Key), env[i].Value).(string)
	}
	return out
}
