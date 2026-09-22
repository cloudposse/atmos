package datafetcher

import (
	"testing"
)

// TestManifestSchema_AuthIdentityNamesWithColon reproduces cloudposse/atmos#3185.
//
// Namespaced identity conventions such as `example/prod:terraform_applier` are accepted by the
// atmos.yaml (config) schema and by the runtime identity model (identity names are opaque map keys,
// resolved case-insensitively with no character validation). The stack-manifest schema, however,
// restricted `auth.identities` / `auth.providers` keys to `^[a-zA-Z0-9/_-]+$`, which excludes `:`.
// Combined with `additionalProperties: false`, that made an otherwise valid global identity
// unusable as a component-level default. This test asserts colon-containing names validate in every
// on-disk copy of the manifest schema, at both the top level and the component level.
func TestManifestSchema_AuthIdentityNamesWithColon(t *testing.T) {
	schemas := map[string][]byte{
		"embedded": loadEmbeddedSchemaBytes(t),
		"website":  loadWebsiteSchemaBytes(t),
		"fixture":  loadFixtureSchemaBytes(t),
	}

	const colonIdentity = "example/prod:terraform_applier"
	const colonProvider = "example/prod:sso"

	// Component-level auth referencing a namespaced identity as the component default (the reported case).
	componentIdentityDefault := map[string]any{
		"components": map[string]any{
			"terraform": map[string]any{
				"vpc": map[string]any{
					"auth": map[string]any{
						"identities": map[string]any{
							colonIdentity: map[string]any{
								"default": true,
							},
						},
					},
				},
			},
		},
	}

	// Component-level auth defining a namespaced provider and identity, exercising both relaxed
	// key patterns (auth_providers and auth_identities) where they are actually schema-validated.
	componentProviderAndIdentity := map[string]any{
		"components": map[string]any{
			"terraform": map[string]any{
				"vpc": map[string]any{
					"auth": map[string]any{
						"providers": map[string]any{
							colonProvider: map[string]any{
								"kind": "aws/iam-identity-center",
							},
						},
						"identities": map[string]any{
							colonIdentity: map[string]any{
								"kind": "aws/ambient",
							},
						},
					},
				},
			},
		},
	}

	for schemaName, schemaData := range schemas {
		t.Run(schemaName+"/component-level auth identity default marker with colon", func(t *testing.T) {
			assertSchemaValid(t, schemaData, componentIdentityDefault)
		})

		t.Run(schemaName+"/component-level auth provider and identity names with colon", func(t *testing.T) {
			assertSchemaValid(t, schemaData, componentProviderAndIdentity)
		})
	}
}

// TestManifestSchema_SecretProviderNamesWithColon covers the sibling case to #3185: `secrets.providers`
// keys shared the same restrictive pattern and the same root-vs-manifest inconsistency (the atmos.yaml
// config schema accepts any `secrets.providers` key, and provider names are opaque map keys at runtime).
// A namespaced SOPS provider name such as `team/prod:sops` must validate at the component level.
func TestManifestSchema_SecretProviderNamesWithColon(t *testing.T) {
	schemas := map[string][]byte{
		"embedded": loadEmbeddedSchemaBytes(t),
		"website":  loadWebsiteSchemaBytes(t),
		"fixture":  loadFixtureSchemaBytes(t),
	}

	const colonSecretProvider = "team/prod:sops"

	manifest := map[string]any{
		"components": map[string]any{
			"terraform": map[string]any{
				"vpc": map[string]any{
					"secrets": map[string]any{
						"providers": map[string]any{
							colonSecretProvider: map[string]any{
								"kind": "sops/age",
								"spec": map[string]any{
									"file": "secrets.yaml",
								},
							},
						},
					},
				},
			},
		},
	}

	for schemaName, schemaData := range schemas {
		t.Run(schemaName+"/component-level secret provider name with colon", func(t *testing.T) {
			assertSchemaValid(t, schemaData, manifest)
		})
	}
}

// TestManifestSchema_AuthIdentityNamesRejectInvalidChars guards the negative path: relaxing the key
// pattern to include `:` must not turn the pattern into a free-for-all. Characters outside the
// allowed set (e.g. spaces) must still be rejected, so `additionalProperties: false` keeps catching
// genuinely malformed identity keys.
func TestManifestSchema_AuthIdentityNamesRejectInvalidChars(t *testing.T) {
	schemas := map[string][]byte{
		"embedded": loadEmbeddedSchemaBytes(t),
		"website":  loadWebsiteSchemaBytes(t),
		"fixture":  loadFixtureSchemaBytes(t),
	}

	// A space is not part of the allowed identity-name character class and must remain invalid.
	invalidManifest := map[string]any{
		"components": map[string]any{
			"terraform": map[string]any{
				"vpc": map[string]any{
					"auth": map[string]any{
						"identities": map[string]any{
							"invalid name": map[string]any{
								"default": true,
							},
						},
					},
				},
			},
		},
	}

	for schemaName, schemaData := range schemas {
		t.Run(schemaName+"/rejects identity name with space", func(t *testing.T) {
			assertSchemaInvalid(t, schemaData, invalidManifest)
		})
	}
}
