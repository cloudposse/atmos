package datafetcher

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xeipuuv/gojsonschema"
)

func TestManifestSchema_WorkflowWhenConditionForms(t *testing.T) {
	schemas := map[string][]byte{
		"embedded": loadEmbeddedSchemaBytes(t),
		"website":  loadWebsiteSchemaBytes(t),
		"fixture":  loadFixtureSchemaBytes(t),
	}

	validConditions := map[string]any{
		"scalar":     "ci",
		"cel":        "ci && stack == 'prod'",
		"cel-tag":    "!cel ci && status == 'success'",
		"list":       []any{"ci", "success"},
		"all":        map[string]any{"all": []any{"ci", "success"}},
		"all-scalar": map[string]any{"all": "ci"},
		"any":        map[string]any{"any": []any{"ci", "local"}},
		"not":        map[string]any{"not": "ci"},
	}

	for schemaName, schemaData := range schemas {
		for name, condition := range validConditions {
			t.Run(schemaName+"/"+name, func(t *testing.T) {
				assertSchemaValid(t, schemaData, workflowManifestWithWhen(condition))
			})
		}

		t.Run(schemaName+"/accepts failure scalar for runtime validation", func(t *testing.T) {
			assertSchemaValid(t, schemaData, workflowManifestWithWhen("failure"))
		})
	}
}

func TestManifestSchema_HookWhenConditionForms(t *testing.T) {
	schemas := map[string][]byte{
		"embedded":     loadEmbeddedSchemaBytes(t),
		"website":      loadWebsiteSchemaBytes(t),
		"fixture":      loadFixtureSchemaBytes(t),
		"stack-config": loadStackConfigSchemaBytes(t),
	}

	validConditions := map[string]any{
		"success":    "success",
		"failure":    "failure",
		"always":     "always",
		"ci":         "ci",
		"cel":        "status == 'failure' || ci",
		"cel-tag":    "!cel status == 'failure' || ci",
		"ci-always":  []any{"ci", "always"},
		"all-scalar": map[string]any{"all": "ci"},
		"compound":   map[string]any{"all": []any{"ci", map[string]any{"not": "never"}}},
	}

	for schemaName, schemaData := range schemas {
		for name, condition := range validConditions {
			t.Run(schemaName+"/"+name, func(t *testing.T) {
				assertSchemaValid(t, schemaData, hookManifestWithWhen(condition))
			})
		}

		t.Run(schemaName+"/accepts unknown scalar as CEL", func(t *testing.T) {
			assertSchemaValid(t, schemaData, hookManifestWithWhen("expr"))
		})
	}
}

func TestManifestSchema_HookRetryUsesWorkflowRetrySchema(t *testing.T) {
	schemas := map[string][]byte{
		"embedded":     loadEmbeddedSchemaBytes(t),
		"website":      loadWebsiteSchemaBytes(t),
		"fixture":      loadFixtureSchemaBytes(t),
		"stack-config": loadStackConfigSchemaBytes(t),
	}

	for schemaName, schemaData := range schemas {
		t.Run(schemaName+"/valid retry", func(t *testing.T) {
			assertSchemaValid(t, schemaData, hookManifestWithRetry(map[string]any{
				"max_attempts":  2,
				"initial_delay": "1s",
			}))
		})

		t.Run(schemaName+"/rejects unknown retry field", func(t *testing.T) {
			assertSchemaInvalid(t, schemaData, hookManifestWithRetry(map[string]any{
				"unknown": true,
			}))
		})
	}
}

func TestManifestSchema_WorkflowStepCastSimulationFields(t *testing.T) {
	schemas := map[string][]byte{
		"embedded": loadEmbeddedSchemaBytes(t),
		"website":  loadWebsiteSchemaBytes(t),
	}

	for schemaName, schemaData := range schemas {
		t.Run(schemaName+"/accepts cast write rate", func(t *testing.T) {
			assertSchemaValid(t, schemaData, workflowManifestWithStep(map[string]any{
				"type":       "cast",
				"mode":       "session",
				"write_rate": "35ms",
			}))
		})

		t.Run(schemaName+"/retains simulate rate", func(t *testing.T) {
			assertSchemaValid(t, schemaData, workflowManifestWithStep(map[string]any{
				"type": "simulate",
				"mode": "typed",
				"rate": "12ms",
				"text": "atmos version",
			}))
		})

		t.Run(schemaName+"/accepts simulate structured prompt", func(t *testing.T) {
			assertSchemaValid(t, schemaData, workflowManifestWithStep(map[string]any{
				"type": "simulate",
				"mode": "typed",
				"prompt": map[string]any{
					"text":  "> ",
					"style": "command",
				},
				"text": "atmos version",
			}))
		})

		t.Run(schemaName+"/rejects non-simulate structured prompt", func(t *testing.T) {
			assertSchemaInvalid(t, schemaData, workflowManifestWithStep(map[string]any{
				"type": "input",
				"prompt": map[string]any{
					"text":  "> ",
					"style": "command",
				},
			}))
		})
	}
}

func TestManifestSchema_TerraformTestFixturesHookShape(t *testing.T) {
	schemas := map[string][]byte{
		"embedded":     loadEmbeddedSchemaBytes(t),
		"website":      loadWebsiteSchemaBytes(t),
		"fixture":      loadFixtureSchemaBytes(t),
		"stack-config": loadStackConfigSchemaBytes(t),
	}

	for schemaName, schemaData := range schemas {
		t.Run(schemaName, func(t *testing.T) {
			assertSchemaValid(t, schemaData, terraformTestFixturesManifest())
		})
	}
}

func TestManifestSchema_TerraformComponentMocks(t *testing.T) {
	schemas := map[string][]byte{
		"embedded":     loadEmbeddedSchemaBytes(t),
		"website":      loadWebsiteSchemaBytes(t),
		"fixture":      loadFixtureSchemaBytes(t),
		"stack-config": loadStackConfigSchemaBytes(t),
	}

	for schemaName, schemaData := range schemas {
		t.Run(schemaName+"/accepts literal output map", func(t *testing.T) {
			assertSchemaValid(t, schemaData, terraformMocksManifest(map[string]any{
				"id":       "vpc-local",
				"subnets":  []any{"subnet-a", "subnet-b"},
				"network":  map[string]any{"cidr": "10.0.0.0/16"},
				"nullable": nil,
			}))
		})

		t.Run(schemaName+"/rejects non-map mocks", func(t *testing.T) {
			assertSchemaInvalid(t, schemaData, terraformMocksManifest("not-a-map"))
		})
	}
}

// TestManifestSchema_KubernetesComponentValidateField guards against the
// validate property drifting out of sync between the schema copies again: it
// was added to stack-config/1.0.json but omitted from atmos/manifest/1.0.json,
// the schema that atmos describe stacks and atmos validate stacks actually
// enforce by default, causing an additionalProperties rejection. The fixture
// copy under tests/fixtures/schemas predates the Kubernetes component feature
// entirely and is intentionally excluded here.
func TestManifestSchema_KubernetesComponentValidateField(t *testing.T) {
	schemas := map[string][]byte{
		"embedded":     loadEmbeddedSchemaBytes(t),
		"website":      loadWebsiteSchemaBytes(t),
		"stack-config": loadStackConfigSchemaBytes(t),
	}

	for schemaName, schemaData := range schemas {
		t.Run(schemaName+"/accepts validate false", func(t *testing.T) {
			assertSchemaValid(t, schemaData, kubernetesComponentManifestWithValidate(false))
		})

		t.Run(schemaName+"/accepts validate true", func(t *testing.T) {
			assertSchemaValid(t, schemaData, kubernetesComponentManifestWithValidate(true))
		})
	}
}

func TestManifestSchema_KubernetesComponentProvisionTargetSplitField(t *testing.T) {
	schemas := map[string][]byte{
		"embedded":     loadEmbeddedSchemaBytes(t),
		"website":      loadWebsiteSchemaBytes(t),
		"stack-config": loadStackConfigSchemaBytes(t),
	}

	for schemaName, schemaData := range schemas {
		t.Run(schemaName+"/accepts split true", func(t *testing.T) {
			assertSchemaValid(t, schemaData, kubernetesComponentManifestWithProvisionSplit(true))
		})

		t.Run(schemaName+"/accepts split false", func(t *testing.T) {
			assertSchemaValid(t, schemaData, kubernetesComponentManifestWithProvisionSplit(false))
		})

		t.Run(schemaName+"/rejects non-bool split", func(t *testing.T) {
			assertSchemaInvalid(t, schemaData, kubernetesComponentManifestWithProvisionSplit("yes"))
		})
	}
}

// TestManifestSchema_CloudFormationComponentManifest guards against the
// aws_cloudformation_component_manifest definition drifting out of sync
// between the schema copies (stack-config/1.0.json vs atmos/manifest/1.0.json)
// the same way TestManifestSchema_KubernetesComponentValidateField guards
// Kubernetes's validate field — a definition added to one copy but omitted
// from the other causes an additionalProperties rejection under the schema
// atmos describe stacks/atmos validate stacks actually enforce by default.
// The fixture copy under tests/fixtures/schemas predates the aws/cloudformation
// component feature entirely and is intentionally excluded here, same as the
// Kubernetes/container precedents above.
func TestManifestSchema_CloudFormationComponentManifest(t *testing.T) {
	schemas := map[string][]byte{
		"embedded":     loadEmbeddedSchemaBytes(t),
		"website":      loadWebsiteSchemaBytes(t),
		"stack-config": loadStackConfigSchemaBytes(t),
	}

	for schemaName, schemaData := range schemas {
		t.Run(schemaName+"/accepts a full component manifest", func(t *testing.T) {
			assertSchemaValid(t, schemaData, cloudFormationComponentManifest())
		})
	}
}

func cloudFormationComponentManifest() map[string]any {
	return map[string]any{
		"components": map[string]any{
			"aws/cloudformation": map[string]any{
				"vpc": map[string]any{
					"metadata": map[string]any{
						"type": "real",
					},
					"stack_name": "acme-plat-ue2-dev-vpc",
					"template":   "template.yaml",
					"parameters": map[string]any{
						"CidrBlock": "10.0.0.0/16",
					},
					"capabilities": []any{"CAPABILITY_IAM", "CAPABILITY_NAMED_IAM"},
					"tags": map[string]any{
						"Team": "platform",
					},
					"stack_policy": map[string]any{
						"file": "stack-policy.json",
					},
					"role_arn":               "arn:aws:iam::123456789012:role/cfn-deploy",
					"notification_arns":      []any{},
					"disable_rollback":       false,
					"termination_protection": true,
					"timeout_in_minutes":     30,
				},
			},
		},
	}
}

// TestManifestSchema_ContainerRuntimeProviderAuto guards against container_runtime.provider
// rejecting "auto" -- a value pkg/schema/container_config.go documents as a first-class supported
// option (distinct from "", though both mean the same thing at runtime: auto-detect Docker, then
// Podman) and website/docs/cli/commands/container/usage.mdx shows literally in examples. The
// fixture copy under tests/fixtures/schemas predates the container component feature entirely (it
// has no "container" key anywhere) and is intentionally excluded here, same as
// TestManifestSchema_KubernetesComponentValidateField.
func TestManifestSchema_ContainerRuntimeProviderAuto(t *testing.T) {
	// loadWebsiteSchemaBytes is intentionally omitted here: it returns the exact
	// same bytes as loadEmbeddedSchemaBytes (see its doc comment), so pairing it
	// alongside "embedded" would validate identical bytes twice under different
	// subtest names without adding any drift-detection value.
	schemas := map[string][]byte{
		"embedded": loadEmbeddedSchemaBytes(t),
	}

	for schemaName, schemaData := range schemas {
		for _, provider := range []string{"", "auto", "docker", "podman"} {
			t.Run(schemaName+"/accepts "+provider, func(t *testing.T) {
				assertSchemaValid(t, schemaData, containerComponentManifestWithRuntimeProvider(provider))
			})
		}

		t.Run(schemaName+"/rejects unknown provider", func(t *testing.T) {
			assertSchemaInvalid(t, schemaData, containerComponentManifestWithRuntimeProvider("crioengine"))
		})
	}
}

func containerComponentManifestWithRuntimeProvider(provider string) map[string]any {
	return map[string]any{
		"components": map[string]any{
			"container": map[string]any{
				"demo": map[string]any{
					"image": "alpine",
					"run": map[string]any{
						"runtime": map[string]any{
							"provider": provider,
						},
					},
				},
			},
		},
	}
}

// TestManifestSchema_SourceTTLField guards against the source.ttl field being rejected. Go reads
// it at pkg/provisioner/source/extract.go ("Optional: ttl") and website/docs/cli/commands/terraform/
// source/source.mdx documents it as a per-component override of the global cache TTL default.
func TestManifestSchema_SourceTTLField(t *testing.T) {
	// loadWebsiteSchemaBytes omitted: byte-identical to loadEmbeddedSchemaBytes.
	schemas := map[string][]byte{
		"embedded": loadEmbeddedSchemaBytes(t),
		"fixture":  loadFixtureSchemaBytes(t),
	}

	for schemaName, schemaData := range schemas {
		t.Run(schemaName+"/accepts ttl", func(t *testing.T) {
			assertSchemaValid(t, schemaData, terraformSourceManifest(map[string]any{
				"uri": "github.com/org/repo//path",
				"ttl": "24h",
			}))
		})
	}
}

func terraformSourceManifest(source any) map[string]any {
	return map[string]any{
		"components": map[string]any{
			"terraform": map[string]any{
				"vpc": map[string]any{
					"source": source,
				},
			},
		},
	}
}

// TestManifestSchema_RootOneOfAllowsWorkflowsWithStackFields guards against the root schema's
// oneOf rejecting a manifest that combines workflows with ordinary stack-manifest fields such as
// vars, settings, or terraform.
//
// Found during the #2919 field test. The root oneOf's second branch was an anyOf list. Its first
// entry alone matched any object lacking a workflows key, which made every other entry in that
// list dead weight. That caused two problems. First, whenever workflows was present alongside
// e.g. vars, branch 0 (which requires workflows) and branch 1 (via the unrelated vars entry)
// both matched at once, failing oneOf with an uninformative error: "(root): valid against
// schemas at indexes 0 and 1" at position 0:0. Second, the real, load-bearing rule was simply
// "not workflows-shaped": a mixin fragment with an arbitrary top-level key not covered by that
// anyOf list (see tests/test-cases/validate-type-mismatch/stacks/mixins/subnet-config.yaml,
// included into another manifest via merge) only ever validated because of that same catch-all
// entry.
//
// The fix collapses branch 1 to a single "not workflows" check, which resolves the ambiguity
// while preserving the schema's original permissiveness for arbitrary non-workflow shapes. An
// earlier attempt instead enumerated the allowed root keys explicitly, which broke that
// mixin-fragment fixture.
func TestManifestSchema_RootOneOfAllowsWorkflowsWithStackFields(t *testing.T) {
	// loadWebsiteSchemaBytes omitted: byte-identical to loadEmbeddedSchemaBytes.
	schemas := map[string][]byte{
		"embedded":     loadEmbeddedSchemaBytes(t),
		"fixture":      loadFixtureSchemaBytes(t),
		"stack-config": loadStackConfigSchemaBytes(t),
	}

	for schemaName, schemaData := range schemas {
		t.Run(schemaName+"/accepts vars+workflows together", func(t *testing.T) {
			assertSchemaValid(t, schemaData, map[string]any{
				"vars": map[string]any{"stage": "dev"},
				"workflows": map[string]any{
					"demo": map[string]any{
						"steps": []any{
							map[string]any{"command": "echo hi"},
						},
					},
				},
			})
		})

		t.Run(schemaName+"/accepts settings+workflows together", func(t *testing.T) {
			assertSchemaValid(t, schemaData, map[string]any{
				"settings": map[string]any{"foo": "bar"},
				"workflows": map[string]any{
					"demo": map[string]any{
						"steps": []any{
							map[string]any{"command": "echo hi"},
						},
					},
				},
			})
		})

		t.Run(schemaName+"/still accepts workflows alone", func(t *testing.T) {
			assertSchemaValid(t, schemaData, workflowManifestWithStep(map[string]any{"command": "echo hi"}))
		})

		t.Run(schemaName+"/still accepts stack fields alone", func(t *testing.T) {
			assertSchemaValid(t, schemaData, map[string]any{
				"vars": map[string]any{"stage": "dev"},
			})
		})

		t.Run(schemaName+"/still accepts an arbitrary non-workflow mixin fragment", func(t *testing.T) {
			// Mirrors tests/test-cases/validate-type-mismatch/stacks/mixins/subnet-config.yaml: a
			// merge-only fragment whose only top-level key isn't a recognized manifest section at
			// all. additionalProperties:true at the root already allows this; the oneOf must not
			// re-impose a closed allow-list on top of it.
			assertSchemaValid(t, schemaData, map[string]any{
				"subnets": []any{
					map[string]any{"cidr": "10.0.1.0/24", "az": "us-east-1a", "type": "public"},
				},
			})
		})
	}
}

// TestManifestSchema_OverridesFieldCoverage guards against terraform.overrides rejecting fields
// that internal/exec/stack_processor_process_stacks_helpers_overrides.go actually merges (hooks,
// generate, secrets, auth, retry, required_providers, required_version), which previously only
// allowed command/vars/env/settings/providers -- see website/docs/stacks/overrides.mdx, which
// documents overrides.hooks and overrides.generate directly.
func TestManifestSchema_OverridesFieldCoverage(t *testing.T) {
	// loadWebsiteSchemaBytes omitted: byte-identical to loadEmbeddedSchemaBytes.
	schemas := map[string][]byte{
		"embedded": loadEmbeddedSchemaBytes(t),
		"fixture":  loadFixtureSchemaBytes(t),
	}

	for schemaName, schemaData := range schemas {
		t.Run(schemaName+"/accepts hooks/generate/secrets/auth/retry/required_*", func(t *testing.T) {
			assertSchemaValid(t, schemaData, map[string]any{
				"terraform": map[string]any{
					"overrides": map[string]any{
						"hooks": map[string]any{
							"test": map[string]any{
								"kind":    "command",
								"command": "echo",
							},
						},
						"generate": map[string]any{
							"backend": map[string]any{
								"enabled": true,
							},
						},
						"secrets": map[string]any{
							"vars": map[string]any{
								"API_KEY": map[string]any{
									"store":     "ssm",
									"reference": "/api-key",
								},
							},
						},
						"auth": map[string]any{
							"realm": "readonly",
						},
						"retry": map[string]any{
							"max_attempts": 3,
							"conditions":   []any{"rate limit"},
						},
						"required_version": ">= 1.5",
						"required_providers": map[string]any{
							"aws": map[string]any{
								"source":  "hashicorp/aws",
								"version": "~> 5.0",
							},
						},
					},
				},
				"components": map[string]any{
					"terraform": map[string]any{
						"vpc": map[string]any{},
					},
				},
			})
		})
	}
}

// TestManifestSchema_ComponentLevelRetry guards against the component-level retry: block being
// entirely unsupported (issue found during the #2919 field test: retry is documented at
// website/docs/stacks/components/terraform/retry.mdx and extracted for every component type by
// internal/exec/stack_processor_process_stacks_helpers_extraction.go, but the embedded schema had
// no retry definition at all prior to this fix).
func TestManifestSchema_ComponentLevelRetry(t *testing.T) {
	// loadWebsiteSchemaBytes omitted: byte-identical to loadEmbeddedSchemaBytes.
	schemas := map[string][]byte{
		"embedded": loadEmbeddedSchemaBytes(t),
		"fixture":  loadFixtureSchemaBytes(t),
	}
	retry := map[string]any{
		"max_attempts": 3,
		"conditions":   []any{"rate limit exceeded"},
	}

	for schemaName, schemaData := range schemas {
		t.Run(schemaName+"/terraform", func(t *testing.T) {
			assertSchemaValid(t, schemaData, map[string]any{
				"components": map[string]any{
					"terraform": map[string]any{
						"vpc": map[string]any{"retry": retry},
					},
				},
			})
		})

		t.Run(schemaName+"/helmfile", func(t *testing.T) {
			assertSchemaValid(t, schemaData, map[string]any{
				"components": map[string]any{
					"helmfile": map[string]any{
						"app": map[string]any{"retry": retry},
					},
				},
			})
		})

		t.Run(schemaName+"/packer", func(t *testing.T) {
			assertSchemaValid(t, schemaData, map[string]any{
				"components": map[string]any{
					"packer": map[string]any{
						"ami": map[string]any{"retry": retry},
					},
				},
			})
		})
	}
}

// TestManifestSchema_RequiredVersionAndProviders guards against terraform.required_version /
// required_providers being rejected at the component level -- documented at
// website/docs/cli/configuration/describe.mdx and extracted by
// internal/exec/stack_processor_process_stacks_helpers_extraction.go (DEV-3124).
func TestManifestSchema_RequiredVersionAndProviders(t *testing.T) {
	// loadWebsiteSchemaBytes omitted: byte-identical to loadEmbeddedSchemaBytes.
	schemas := map[string][]byte{
		"embedded": loadEmbeddedSchemaBytes(t),
		"fixture":  loadFixtureSchemaBytes(t),
	}

	for schemaName, schemaData := range schemas {
		t.Run(schemaName+"/accepts required_version and required_providers", func(t *testing.T) {
			assertSchemaValid(t, schemaData, map[string]any{
				"components": map[string]any{
					"terraform": map[string]any{
						"vpc": map[string]any{
							"required_version": ">= 1.5",
							"required_providers": map[string]any{
								"aws": map[string]any{
									"source":  "hashicorp/aws",
									"version": "~> 5.0",
								},
							},
						},
					},
				},
			})
		})
	}
}

// TestManifestSchema_KubernetesComponentSecrets guards against components.kubernetes.<c>.secrets
// being rejected: secrets declarations are extracted for all component types ("Available for all
// component types", stack_processor_process_stacks_helpers_extraction.go), and kubernetes already
// supports the sibling auth section -- secrets was the one omission. The fixture copy predates the
// Kubernetes component feature entirely (see TestManifestSchema_KubernetesComponentValidateField)
// and is intentionally excluded here; stack-config/1.0.json has no component_secrets definition at
// all (component-level `secrets:` is unwired there for every component type, not just kubernetes --
// a separate, pre-existing structural gap in that schema copy, out of scope for this fix since
// stack-config.json isn't what `atmos describe stacks`/`validate stacks` enforce by default), and is
// also excluded here.
func TestManifestSchema_KubernetesComponentSecrets(t *testing.T) {
	// loadWebsiteSchemaBytes omitted: byte-identical to loadEmbeddedSchemaBytes.
	schemas := map[string][]byte{
		"embedded": loadEmbeddedSchemaBytes(t),
	}

	for schemaName, schemaData := range schemas {
		t.Run(schemaName+"/accepts secrets", func(t *testing.T) {
			assertSchemaValid(t, schemaData, map[string]any{
				"components": map[string]any{
					"kubernetes": map[string]any{
						"app": map[string]any{
							"metadata": map[string]any{"type": "real"},
							"manifests": []any{
								map[string]any{"apiVersion": "v1", "kind": "ConfigMap"},
							},
							"secrets": map[string]any{
								"vars": map[string]any{
									"API_KEY": map[string]any{
										"store":     "ssm",
										"reference": "/api-key",
									},
								},
							},
						},
					},
				},
			})
		})
	}
}

// TestManifestSchema_BackendTypeCoverage guards against the backend_type /
// remote_state_backend_type enum and the backend_manifest.properties allow-list
// drifting apart -- either between the three schema copies (embedded,
// stack-config, fixture) or between the enum and the properties within a
// single copy. See https://github.com/cloudposse/atmos/issues/2919: the
// enum previously only listed local/s3/remote/vault/static/azurerm/gcs/cloud,
// silently rejecting real Terraform/OpenTofu backends -- consul/cos/http/
// kubernetes/oss/pg -- that website/docs/components/terraform/backends.mdx
// already documents as supported.
func TestManifestSchema_BackendTypeCoverage(t *testing.T) {
	// loadWebsiteSchemaBytes omitted: byte-identical to loadEmbeddedSchemaBytes.
	schemas := map[string][]byte{
		"embedded":     loadEmbeddedSchemaBytes(t),
		"fixture":      loadFixtureSchemaBytes(t),
		"stack-config": loadStackConfigSchemaBytes(t),
	}

	backendTypes := []string{
		"local", "s3", "remote", "vault", "static", "azurerm", "gcs", "cloud",
		"consul", "cos", "http", "kubernetes", "oss", "pg",
	}

	for schemaName, schemaData := range schemas {
		for _, backendType := range backendTypes {
			t.Run(schemaName+"/backend/"+backendType, func(t *testing.T) {
				assertSchemaValid(t, schemaData, terraformBackendManifest(backendType))
			})

			t.Run(schemaName+"/remote_state_backend/"+backendType, func(t *testing.T) {
				assertSchemaValid(t, schemaData, terraformRemoteStateBackendManifest(backendType))
			})
		}

		t.Run(schemaName+"/rejects unknown backend_type value", func(t *testing.T) {
			// Isolate the enum check: "s3" is a valid, recognized key under backend:, so
			// the only possible cause of rejection is backend_type's own enum -- unlike
			// asserting on an unrecognized backend: key (which additionalProperties:false
			// would also reject on its own, even with the enum check deleted entirely).
			manifest := terraformBackendManifest("s3")
			manifest["components"].(map[string]any)["terraform"].(map[string]any)["vpc"].(map[string]any)["backend_type"] = "not-a-real-backend"
			assertSchemaInvalid(t, schemaData, manifest)
		})

		t.Run(schemaName+"/rejects unrecognized backend key", func(t *testing.T) {
			// Isolate the backend_manifest allow-list: backend_type stays valid, so the
			// only possible cause of rejection is the unrecognized key under backend:.
			manifest := terraformBackendManifest("s3")
			component := manifest["components"].(map[string]any)["terraform"].(map[string]any)["vpc"].(map[string]any)
			component["backend"] = map[string]any{
				"not-a-real-backend": map[string]any{"address": "https://example.com/state"},
			}
			assertSchemaInvalid(t, schemaData, manifest)
		})
	}
}

func terraformBackendManifest(backendType string) map[string]any {
	return map[string]any{
		"components": map[string]any{
			"terraform": map[string]any{
				"vpc": map[string]any{
					"backend_type": backendType,
					"backend": map[string]any{
						backendType: map[string]any{
							"address": "https://example.com/state",
						},
					},
				},
			},
		},
	}
}

func terraformRemoteStateBackendManifest(backendType string) map[string]any {
	return map[string]any{
		"components": map[string]any{
			"terraform": map[string]any{
				"vpc": map[string]any{
					"backend_type": backendType,
					"backend": map[string]any{
						backendType: map[string]any{
							"address": "https://example.com/state",
						},
					},
					"remote_state_backend_type": backendType,
					"remote_state_backend": map[string]any{
						backendType: map[string]any{
							"address": "https://example.com/state",
						},
					},
				},
			},
		},
	}
}

func kubernetesComponentManifestWithProvisionSplit(split any) map[string]any {
	return map[string]any{
		"components": map[string]any{
			"kubernetes": map[string]any{
				"gitops-target": map[string]any{
					"metadata": map[string]any{
						"type": "real",
					},
					"manifests": []any{
						map[string]any{
							"apiVersion": "v1",
							"kind":       "ConfigMap",
						},
					},
					"provision": map[string]any{
						"targets": map[string]any{
							"deployment-repo": map[string]any{
								"kind":  "git",
								"path":  "clusters/dev/demo",
								"split": split,
							},
						},
					},
				},
			},
		},
	}
}

func kubernetesComponentManifestWithValidate(validate bool) map[string]any {
	return map[string]any{
		"components": map[string]any{
			"kubernetes": map[string]any{
				"legacy-manifests": map[string]any{
					"metadata": map[string]any{
						"type": "real",
					},
					"validate": validate,
					"manifests": []any{
						map[string]any{
							"apiVersion": "v1",
							"kind":       "ConfigMap",
						},
					},
				},
			},
		},
	}
}

func workflowManifestWithWhen(condition any) map[string]any {
	return workflowManifestWithStep(map[string]any{
		"command": "echo ok",
		"when":    condition,
	})
}

func workflowManifestWithStep(step map[string]any) map[string]any {
	return map[string]any{
		"workflows": map[string]any{
			"test": map[string]any{
				"steps": []any{
					step,
				},
			},
		},
	}
}

func hookManifestWithWhen(condition any) map[string]any {
	return map[string]any{
		"hooks": map[string]any{
			"test": map[string]any{
				"kind":    "command",
				"command": "echo",
				"when":    condition,
			},
		},
	}
}

func hookManifestWithRetry(retry any) map[string]any {
	manifest := hookManifestWithWhen("always")
	manifest["hooks"].(map[string]any)["test"].(map[string]any)["retry"] = retry
	return manifest
}

func terraformTestFixturesManifest() map[string]any {
	return map[string]any{
		"components": map[string]any{
			"terraform": map[string]any{
				"app": map[string]any{
					"metadata": map[string]any{
						"type": "real",
					},
					"hooks": map[string]any{
						"test-fixtures-up": map[string]any{
							"kind": "steps",
							"events": []any{
								"before.terraform.test",
							},
							"with": []any{
								map[string]any{
									"type":      "emulator",
									"component": "aws",
									"action":    "up",
								},
								map[string]any{
									"type":    "atmos",
									"command": "terraform apply vpc -s fixtures -auto-approve",
								},
							},
						},
					},
					"test": map[string]any{
						"vars": map[string]any{
							"fixture_vpc_id": "vpc-123",
						},
					},
				},
			},
		},
	}
}

func terraformMocksManifest(mocks any) map[string]any {
	return map[string]any{
		"components": map[string]any{
			"terraform": map[string]any{
				"vpc": map[string]any{
					"mocks": mocks,
				},
			},
		},
	}
}

func loadEmbeddedSchemaBytes(t *testing.T) []byte {
	t.Helper()

	data, err := (&atmosFetcher{}).FetchData("atmos://schema/atmos/manifest/1.0")
	require.NoError(t, err)
	return data
}

// loadWebsiteSchemaBytes returns the schema bytes served at atmos.tools. The website copy is
// generated from the embedded schema at build time (see `atmos stack schema`), so it is byte-identical
// to the embedded schema — this reads the embedded schema directly rather than depending on a
// generated file being present on disk.
func loadWebsiteSchemaBytes(t *testing.T) []byte {
	t.Helper()
	return loadEmbeddedSchemaBytes(t)
}

func loadFixtureSchemaBytes(t *testing.T) []byte {
	t.Helper()
	return loadSchemaFile(t, "../../tests/fixtures/schemas/atmos/atmos-manifest/1.0/atmos-manifest.json")
}

func loadStackConfigSchemaBytes(t *testing.T) []byte {
	t.Helper()
	return loadSchemaFile(t, "schema/stacks/stack-config/1.0.json")
}

func loadSchemaFile(t *testing.T, path string) []byte {
	t.Helper()

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return data
}

func assertSchemaValid(t *testing.T, schemaData []byte, manifest map[string]any) {
	t.Helper()

	result := validateManifestAgainstSchema(t, schemaData, manifest)
	if !result.Valid() {
		for _, desc := range result.Errors() {
			t.Logf("validation error: %s", desc)
		}
	}
	assert.True(t, result.Valid(), "expected valid manifest")
}

func assertSchemaInvalid(t *testing.T, schemaData []byte, manifest map[string]any) {
	t.Helper()

	result := validateManifestAgainstSchema(t, schemaData, manifest)
	assert.False(t, result.Valid(), "expected invalid manifest")
}

func validateManifestAgainstSchema(t *testing.T, schemaData []byte, manifest map[string]any) *gojsonschema.Result {
	t.Helper()

	docJSON, err := json.Marshal(manifest)
	require.NoError(t, err)

	result, err := gojsonschema.Validate(
		gojsonschema.NewBytesLoader(schemaData),
		gojsonschema.NewBytesLoader(docJSON),
	)
	require.NoError(t, err, "schema validation should not error")
	return result
}
