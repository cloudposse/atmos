//go:build pact

package pro

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/pact-foundation/pact-go/v2/consumer"
	"github.com/pact-foundation/pact-go/v2/matchers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/pro/dtos"
	"github.com/cloudposse/atmos/pkg/schema"
)

// body is a shorthand alias for matchers.StructMatcher used in pact interaction bodies.
// StructMatcher implements matchers.Matcher, enabling it to be nested inside MapMatcher.
type body = matchers.StructMatcher

// TestPact_UploadAffectedStacks verifies the consumer contract for POST /api/v1/affected-stacks.
func TestPact_UploadAffectedStacks(t *testing.T) {
	mockProvider := newHTTPMockProvider(t)

	err := mockProvider.
		AddInteraction().
		Given("workspace exists and accepts affected stacks").
		UponReceiving("a request to upload affected stacks").
		WithRequest("POST", "/api/v1/affected-stacks", func(b *consumer.V2RequestBuilder) {
			b.Header("Authorization", matchers.Like("Bearer test-token")).
				Header("Content-Type", matchers.S("application/json")).
				JSONBody(body{
					"head_sha":   matchers.Like("abc123def456"),
					"base_sha":   matchers.Like("111222333444"),
					"repo_url":   matchers.Like("https://github.com/org/repo"),
					"repo_name":  matchers.Like("repo"),
					"repo_owner": matchers.Like("org"),
					"repo_host":  matchers.Like("github.com"),
					// All non-omitempty fields from schema.Affected are always serialized
					// and must be included for pact V2 strict body matching.
					// Values reflect post-StripAffectedForUpload state: component_type,
					// component_path, affected, and stack_slug are zeroed; affected_all is
					// nil (null); dependents is always [] (never null).
					"stacks": matchers.EachLike(body{
						"component":              matchers.Like("vpc"),
						"stack":                  matchers.Like("dev-us-east-1"),
						"component_type":         matchers.Like(""),
						"component_path":         matchers.Like(""),
						"stack_slug":             matchers.Like(""),
						"affected":               matchers.Like(""),
						"affected_all":           nil,
						"dependents":             []interface{}{},
						"included_in_dependents": matchers.Like(false),
						"settings":               nil,
					}, 1),
				})
		}).
		WillRespondWith(200, func(b *consumer.V2ResponseBuilder) {
			b.JSONBody(body{
				"success": matchers.Like(true),
			})
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			client := newPactClient(config)
			return client.UploadAffectedStacks(&dtos.UploadAffectedStacksRequest{
				HeadSHA:   "abc123def456",
				BaseSHA:   "111222333444",
				RepoURL:   "https://github.com/org/repo",
				RepoName:  "repo",
				RepoOwner: "org",
				RepoHost:  "github.com",
				// Stacks mirror the output of StripAffectedForUpload: only Component,
				// Stack, IncludedInDependents, Dependents, Settings, Deleted, and
				// DeletionType are preserved; all other fields are zero values.
				Stacks: []schema.Affected{
					{
						Component:  "vpc",
						Stack:      "dev-us-east-1",
						Dependents: []schema.Dependent{},
					},
				},
			})
		})
	require.NoError(t, err)
}

// TestPact_LockStack verifies the consumer contract for POST /api/v1/locks.
func TestPact_LockStack(t *testing.T) {
	mockProvider := newHTTPMockProvider(t)

	err := mockProvider.
		AddInteraction().
		Given("workspace exists and stack is unlocked").
		UponReceiving("a request to lock a stack").
		WithRequest("POST", "/api/v1/locks", func(b *consumer.V2RequestBuilder) {
			b.Header("Authorization", matchers.Like("Bearer test-token")).
				Header("Content-Type", matchers.S("application/json")).
				JSONBody(body{
					"key": matchers.Like("org/repo/dev/vpc"),
					"ttl": matchers.Like(int32(3600)),
				})
		}).
		WillRespondWith(200, func(b *consumer.V2ResponseBuilder) {
			b.JSONBody(body{
				"success": matchers.Like(true),
				"data": body{
					"id":        matchers.Like("lock-id-uuid"),
					"key":       matchers.Like("org/repo/dev/vpc"),
					"expiresAt": matchers.Term("2026-06-09T10:00:00Z", `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$`),
				},
			})
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			client := newPactClient(config)
			resp, err := client.LockStack(&dtos.LockStackRequest{
				Key: "org/repo/dev/vpc",
				TTL: 3600,
			})
			if err != nil {
				return err
			}
			assert.NotEmpty(t, resp.Data.ID)
			return nil
		})
	require.NoError(t, err)
}

// TestPact_UnlockStack verifies the consumer contract for DELETE /api/v1/locks.
func TestPact_UnlockStack(t *testing.T) {
	mockProvider := newHTTPMockProvider(t)

	err := mockProvider.
		AddInteraction().
		Given("workspace exists and stack is locked").
		UponReceiving("a request to unlock a stack").
		WithRequest("DELETE", "/api/v1/locks", func(b *consumer.V2RequestBuilder) {
			b.Header("Authorization", matchers.Like("Bearer test-token")).
				Header("Content-Type", matchers.S("application/json")).
				JSONBody(body{
					"key": matchers.Like("org/repo/dev/vpc"),
				})
		}).
		WillRespondWith(200, func(b *consumer.V2ResponseBuilder) {
			b.JSONBody(body{
				"success": matchers.Like(true),
			})
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			client := newPactClient(config)
			_, err := client.UnlockStack(&dtos.UnlockStackRequest{Key: "org/repo/dev/vpc"})
			return err
		})
	require.NoError(t, err)
}

// TestPact_ExchangeOIDCToken verifies the consumer contract for POST /api/v1/auth/github-oidc.
func TestPact_ExchangeOIDCToken(t *testing.T) {
	mockProvider := newHTTPMockProvider(t)

	err := mockProvider.
		AddInteraction().
		Given("OIDC token is valid and workspace exists").
		UponReceiving("a request to exchange a GitHub OIDC token for an Atmos Pro token").
		WithRequest("POST", "/api/v1/auth/github-oidc", func(b *consumer.V2RequestBuilder) {
			b.Header("Content-Type", matchers.S("application/json")).
				JSONBody(body{
					"token":       matchers.Like("eyJhbGciOiJSUzI1NiJ9.oidcpayload"),
					"workspaceId": matchers.Like("workspace-uuid-1234"),
				})
		}).
		WillRespondWith(200, func(b *consumer.V2ResponseBuilder) {
			b.JSONBody(body{
				"success": matchers.Like(true),
				"data": body{
					"token": matchers.Like("atmos-jwt-session-token"),
				},
			})
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			// exchangeOIDCTokenForAtmosToken is package-private and accessible within package pro.
			baseURL := fmt.Sprintf("http://%s:%d", config.Host, config.Port)
			token, err := exchangeOIDCTokenForAtmosToken(baseURL, "api/v1", "eyJhbGciOiJSUzI1NiJ9.oidcpayload", "workspace-uuid-1234")
			if err != nil {
				return err
			}
			assert.NotEmpty(t, token)
			return nil
		})
	require.NoError(t, err)
}

// TestPact_UploadInstances verifies the consumer contract for POST /api/v1/instances.
func TestPact_UploadInstances(t *testing.T) {
	mockProvider := newHTTPMockProvider(t)

	err := mockProvider.
		AddInteraction().
		Given("workspace exists and accepts drift detection instances").
		UponReceiving("a request to upload drift detection instances").
		WithRequest("POST", "/api/v1/instances", func(b *consumer.V2RequestBuilder) {
			b.Header("Authorization", matchers.Like("Bearer test-token")).
				Header("Content-Type", matchers.S("application/json")).
				JSONBody(body{
					"repo_url":   matchers.Like("https://github.com/org/repo"),
					"repo_name":  matchers.Like("repo"),
					"repo_owner": matchers.Like("org"),
					"repo_host":  matchers.Like("github.com"),
					"instances": matchers.EachLike(body{
						"component":      matchers.Like("vpc"),
						"stack":          matchers.Like("dev-us-east-1"),
						"component_type": matchers.Like("terraform"),
					}, 1),
				})
		}).
		WillRespondWith(200, func(b *consumer.V2ResponseBuilder) {
			b.JSONBody(body{
				"success": matchers.Like(true),
			})
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			client := newPactClient(config)
			return client.UploadInstances(&dtos.InstancesUploadRequest{
				RepoURL:   "https://github.com/org/repo",
				RepoName:  "repo",
				RepoOwner: "org",
				RepoHost:  "github.com",
				Instances: []dtos.UploadInstance{
					{
						Component:     "vpc",
						Stack:         "dev-us-east-1",
						ComponentType: "terraform",
					},
				},
			})
		})
	require.NoError(t, err)
}

// TestPact_UploadInstanceStatus verifies the consumer contract for PATCH /api/v1/repos/{owner}/{repo}/instances.
func TestPact_UploadInstanceStatus(t *testing.T) {
	mockProvider := newHTTPMockProvider(t)

	err := mockProvider.
		AddInteraction().
		Given("workspace exists and instance exists for owner/repo").
		UponReceiving("a request to upload instance drift status").
		WithRequest("PATCH", "/api/v1/repos/org/repo/instances", func(b *consumer.V2RequestBuilder) {
			b.Header("Authorization", matchers.Like("Bearer test-token")).
				Header("Content-Type", matchers.S("application/json")).
				Query("stack", matchers.S("dev-us-east-1")).
				Query("component", matchers.S("vpc")).
				JSONBody(body{
					"command":   matchers.Term("plan", `^(plan|apply|remediate)$`),
					"exit_code": matchers.Like(0),
				})
		}).
		WillRespondWith(200, func(b *consumer.V2ResponseBuilder) {
			b.JSONBody(body{
				"success": matchers.Like(true),
			})
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			client := newPactClient(config)
			return client.UploadInstanceStatus(&dtos.InstanceStatusUploadRequest{
				RepoOwner: "org",
				RepoName:  "repo",
				Stack:     "dev-us-east-1",
				Component: "vpc",
				Command:   "plan",
				ExitCode:  0,
			})
		})
	require.NoError(t, err)
}

// TestPact_CreateCommit verifies the consumer contract for POST /api/v1/git/commit.
func TestPact_CreateCommit(t *testing.T) {
	mockProvider := newHTTPMockProvider(t)

	err := mockProvider.
		AddInteraction().
		Given("workspace exists and GitHub App is authorized").
		UponReceiving("a request to create a commit via Atmos Pro GitHub App").
		WithRequest("POST", "/api/v1/git/commit", func(b *consumer.V2RequestBuilder) {
			b.Header("Authorization", matchers.Like("Bearer test-token")).
				Header("Content-Type", matchers.S("application/json")).
				JSONBody(body{
					"branch":        matchers.Like("main"),
					"commitMessage": matchers.Like("test: pact contract verification"),
					"changes": body{
						"additions": matchers.EachLike(body{
							"path":     matchers.Like("file.txt"),
							"contents": matchers.Like("dGVzdA=="),
						}, 1),
						"deletions": matchers.EachLike(body{
							"path": matchers.Like("old.txt"),
						}, 0),
					},
				})
		}).
		WillRespondWith(200, func(b *consumer.V2ResponseBuilder) {
			b.JSONBody(body{
				"success": matchers.Like(true),
				"data": body{
					"sha": matchers.Like("abc123def456789"),
				},
			})
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			client := newPactClient(config)
			resp, err := client.CreateCommit(&dtos.CommitRequest{
				Branch:        "main",
				CommitMessage: "test: pact contract verification",
				Changes: dtos.CommitChanges{
					Additions: []dtos.CommitFileAddition{
						{Path: "file.txt", Contents: "dGVzdA=="},
					},
					Deletions: []dtos.CommitFileDeletion{
						{Path: "old.txt"},
					},
				},
			})
			if err != nil {
				return err
			}
			assert.NotEmpty(t, resp.Data.SHA)
			return nil
		})
	require.NoError(t, err)
}

// TestPact_GetGitHubOIDCToken verifies the consumer contract for GET ACTIONS_ID_TOKEN_REQUEST_URL.
// A TLS mock provider is required because buildOIDCRequestURL enforces the https:// scheme.
func TestPact_GetGitHubOIDCToken(t *testing.T) {
	mockProvider := newTLSMockProvider(t)

	err := mockProvider.
		AddInteraction().
		Given("GitHub Actions OIDC endpoint is available").
		UponReceiving("a request to retrieve a GitHub OIDC token").
		WithRequest("GET", "/token", func(b *consumer.V2RequestBuilder) {
			b.Header("Authorization", matchers.Like("Bearer test-request-token")).
				Query("audience", matchers.S("atmos-pro.com"))
		}).
		WillRespondWith(200, func(b *consumer.V2ResponseBuilder) {
			b.Header("Content-Type", matchers.S("application/json")).
				JSONBody(body{
					"value": matchers.Like("eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.oidc"),
				})
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			settings := schema.GithubOIDCSettings{
				// Must be https:// — the TLS mock server satisfies buildOIDCRequestURL's scheme check.
				RequestURL:   fmt.Sprintf("https://%s:%d/token", config.Host, config.Port),
				RequestToken: "test-request-token",
			}
			// Pass the TLS-aware client directly via variadic arg, avoiding global state mutation.
			token, err := getGitHubOIDCToken(settings, tlsHTTPClient(config.TLSConfig))
			if err != nil {
				return err
			}
			assert.NotEmpty(t, token)
			return nil
		})
	require.NoError(t, err)
}

// TestPact_UploadExecMetadata verifies the consumer contract for
// POST /api/v1/atmos/exec with a populated `data` field (terraform plan
// shape), per specs/002-pro-exec-metadata/contracts/interactions.md.
// Single- and multi-component invocations are never structurally different
// (spec.md FR-006a, 2026-08-21 clarification): a single-component invocation's
// `Data` is the same {"version": 1, "components": [TerraformExecData]}
// wrapper as a multi-component run, just with a one-element list — never a
// bare TerraformExecData object at the top level.
func TestPact_UploadExecMetadata(t *testing.T) {
	mockProvider := newHTTPMockProvider(t)

	err := mockProvider.
		AddInteraction().
		Given("workspace exists and accepts execution metadata").
		UponReceiving("a request to upload command-execution metadata with inline data").
		WithRequest("POST", "/api/v1/atmos/exec", func(b *consumer.V2RequestBuilder) {
			b.Header("Authorization", matchers.Like("Bearer test-token")).
				Header("Content-Type", matchers.S("application/json")).
				JSONBody(body{
					"execution_id":     matchers.Like("b3b1e2b0-1234-4a1b-8c1d-1234567890ab"),
					"atmos_pro_run_id": matchers.Like("run-12345"),
					"atmos_version":    matchers.Like("1.2.3"),
					"atmos_os":         matchers.Like("linux"),
					"atmos_arch":       matchers.Like("amd64"),
					"command":          matchers.Like("terraform plan"),
					"args":             []interface{}{},
					"flags":            []interface{}{},
					"exit_code":        matchers.Like(0),
					"git_sha":          matchers.Like("abc123def456"),
					"repo_url":         matchers.Like("https://github.com/org/repo"),
					"repo_name":        matchers.Like("repo"),
					"repo_owner":       matchers.Like("org"),
					"repo_host":        matchers.Like("github.com"),
					"metrics": body{
						"wall_time_ms":       matchers.Like(1234),
						"user_cpu_time_ms":   matchers.Like(800),
						"system_cpu_time_ms": matchers.Like(150),
						"max_rss_bytes":      matchers.Like(52428800),
						"minor_page_faults":  matchers.Like(120),
						"major_page_faults":  matchers.Like(1),
						"in_block_ops":       matchers.Like(4),
						"out_block_ops":      matchers.Like(2),
						"vol_ctx_switches":   matchers.Like(30),
						"invol_ctx_switches": matchers.Like(5),
					},
					"data": body{
						"version": 1,
						"components": matchers.EachLike(body{
							"resource_counts": body{
								"create":  matchers.Like(2),
								"change":  matchers.Like(1),
								"replace": matchers.Like(0),
								"destroy": matchers.Like(0),
							},
							"outputs": body{
								"bucket_arn": body{
									"value":     matchers.Like("arn:aws:s3:::prod-bucket"),
									"sensitive": matchers.Like(false),
								},
								"secret_key": body{
									"value":     iolib.MaskReplacement,
									"sensitive": matchers.Like(true),
								},
							},
							"warnings": matchers.EachLike("deprecated argument used", 1),
							"changes": matchers.EachLike(body{
								"action":  matchers.Like("created"),
								"address": matchers.Like("aws_s3_bucket.example"),
							}, 1),
							"has_changes": matchers.Like(true),
							"has_errors":  matchers.Like(false),
							"errors":      []interface{}{},
							"exit_code":   matchers.Like(2),
							"component":   matchers.Like("vpc"),
							"stack":       matchers.Like("plat-use2-dev"),
							"logs":        matchers.Like(base64.StdEncoding.EncodeToString([]byte("Plan: 2 to add, 1 to change, 0 to destroy."))),
						}, 1),
					},
				})
		}).
		WillRespondWith(200, func(b *consumer.V2ResponseBuilder) {
			b.JSONBody(body{
				"success": matchers.Like(true),
			})
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			client := newPactClient(config)
			data, err := json.Marshal(map[string]any{
				"version": 1,
				"components": []map[string]any{
					{
						"resource_counts": map[string]any{
							"create":  2,
							"change":  1,
							"replace": 0,
							"destroy": 0,
						},
						"outputs": map[string]any{
							"bucket_arn": map[string]any{"value": "arn:aws:s3:::prod-bucket", "sensitive": false},
							"secret_key": map[string]any{"value": iolib.MaskReplacement, "sensitive": true},
						},
						"warnings": []string{"deprecated argument used"},
						"changes": []map[string]any{
							{"action": "created", "address": "aws_s3_bucket.example"},
						},
						"has_changes": true,
						"has_errors":  false,
						"errors":      []string{},
						// exit_code (research.md Decision 27) is the terraform subprocess's
						// own exit code — distinct from the envelope's own ExitCode (0)
						// above: a `plan -detailed-exitcode`-style 2 signals "succeeded,
						// changes present", the authoritative signal independent of
						// has_changes/resource_counts.
						"exit_code": 2,
						"component": "vpc",
						"stack":     "plat-use2-dev",
						// logs is base64-encoded plaintext (already masked before
						// encoding, per FR-010a) — never raw/inline text, since a
						// downstream Gitleaks pass over the whole marshaled Data blob
						// cannot pattern-match secrets inside base64-encoded bytes.
						"logs": base64.StdEncoding.EncodeToString([]byte("Plan: 2 to add, 1 to change, 0 to destroy.")),
					},
				},
			})
			if err != nil {
				return err
			}
			return client.UploadExecMetadata(&dtos.ExecUploadRequest{
				ExecutionID:   "b3b1e2b0-1234-4a1b-8c1d-1234567890ab",
				AtmosProRunID: "run-12345",
				AtmosVersion:  "1.2.3",
				AtmosOS:       "linux",
				AtmosArch:     "amd64",
				Command:       "atmos terraform plan",
				Args:          []string{},
				Flags:         []string{},
				ExitCode:      0,
				GitSHA:        "abc123def456",
				RepoURL:       "https://github.com/org/repo",
				RepoName:      "repo",
				RepoOwner:     "org",
				RepoHost:      "github.com",
				Metrics: dtos.ResourceUsageMetrics{
					WallTimeMS:       1234,
					UserCPUTimeMS:    800,
					SystemCPUTimeMS:  150,
					MaxRSSBytes:      52428800,
					MinorPageFaults:  120,
					MajorPageFaults:  1,
					InBlockOps:       4,
					OutBlockOps:      2,
					VolCtxSwitches:   30,
					InvolCtxSwitches: 5,
				},
				Data: data,
			})
		})
	require.NoError(t, err)
}

// TestPact_UploadExecMetadata_Apply verifies the consumer contract for
// POST /api/v1/atmos/exec with a populated `data` field for `terraform apply`.
// `TerraformExecData`'s wire shape is shared, unconditionally, across
// `plan`/`apply`/`deploy` (research.md Decisions 37/38, spec.md FR-006a,
// specs/002-pro-exec-metadata/contracts/interactions.md interaction 9) — this
// is not a distinct contract from TestPact_UploadExecMetadata, just an
// explicit `command: "terraform apply"` example exercising a
// successful-apply-shaped fixture (subprocess exit_code 0 — apply has no
// `-detailed-exitcode` convention the way `plan` does, so 0 covers both
// "succeeded" and "succeeded with changes applied").
func TestPact_UploadExecMetadata_Apply(t *testing.T) {
	mockProvider := newHTTPMockProvider(t)

	err := mockProvider.
		AddInteraction().
		Given("workspace exists and accepts execution metadata").
		UponReceiving("a request to upload apply command-execution metadata with inline data").
		WithRequest("POST", "/api/v1/atmos/exec", func(b *consumer.V2RequestBuilder) {
			b.Header("Authorization", matchers.Like("Bearer test-token")).
				Header("Content-Type", matchers.S("application/json")).
				JSONBody(body{
					"execution_id":     matchers.Like("22f2a2f2-789a-4b2c-9d2e-2345678901bc"),
					"atmos_pro_run_id": matchers.Like("run-12345"),
					"atmos_version":    matchers.Like("1.2.3"),
					"atmos_os":         matchers.Like("linux"),
					"atmos_arch":       matchers.Like("amd64"),
					"command":          matchers.Like("terraform apply"),
					"args":             []interface{}{},
					"flags":            []interface{}{},
					"exit_code":        matchers.Like(0),
					"git_sha":          matchers.Like("abc123def456"),
					"repo_url":         matchers.Like("https://github.com/org/repo"),
					"repo_name":        matchers.Like("repo"),
					"repo_owner":       matchers.Like("org"),
					"repo_host":        matchers.Like("github.com"),
					"metrics": body{
						"wall_time_ms":       matchers.Like(5678),
						"user_cpu_time_ms":   matchers.Like(3200),
						"system_cpu_time_ms": matchers.Like(400),
					},
					"data": body{
						"version": 1,
						"components": matchers.EachLike(body{
							"resource_counts": body{
								"create":  matchers.Like(2),
								"change":  matchers.Like(1),
								"replace": matchers.Like(0),
								"destroy": matchers.Like(0),
							},
							"outputs": body{
								"bucket_arn": body{
									"value":     matchers.Like("arn:aws:s3:::prod-bucket"),
									"sensitive": matchers.Like(false),
								},
							},
							"warnings": []interface{}{},
							"changes": matchers.EachLike(body{
								"action":  matchers.Like("created"),
								"address": matchers.Like("aws_s3_bucket.example"),
							}, 1),
							"has_changes": matchers.Like(true),
							"has_errors":  matchers.Like(false),
							"errors":      []interface{}{},
							"exit_code":   matchers.Like(0),
							"component":   matchers.Like("vpc"),
							"stack":       matchers.Like("plat-use2-dev"),
							"logs":        matchers.Like(base64.StdEncoding.EncodeToString([]byte("Apply complete! Resources: 2 added, 1 changed, 0 destroyed."))),
						}, 1),
					},
				})
		}).
		WillRespondWith(200, func(b *consumer.V2ResponseBuilder) {
			b.JSONBody(body{
				"success": matchers.Like(true),
			})
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			client := newPactClient(config)
			data, err := json.Marshal(map[string]any{
				"version": 1,
				"components": []map[string]any{
					{
						"resource_counts": map[string]any{
							"create":  2,
							"change":  1,
							"replace": 0,
							"destroy": 0,
						},
						"outputs": map[string]any{
							"bucket_arn": map[string]any{"value": "arn:aws:s3:::prod-bucket", "sensitive": false},
						},
						"warnings": []string{},
						"changes": []map[string]any{
							{"action": "created", "address": "aws_s3_bucket.example"},
						},
						"has_changes": true,
						"has_errors":  false,
						"errors":      []string{},
						"exit_code":   0,
						"component":   "vpc",
						"stack":       "plat-use2-dev",
						"logs":        base64.StdEncoding.EncodeToString([]byte("Apply complete! Resources: 2 added, 1 changed, 0 destroyed.")),
					},
				},
			})
			if err != nil {
				return err
			}
			return client.UploadExecMetadata(&dtos.ExecUploadRequest{
				ExecutionID:   "22f2a2f2-789a-4b2c-9d2e-2345678901bc",
				AtmosProRunID: "run-12345",
				AtmosVersion:  "1.2.3",
				AtmosOS:       "linux",
				AtmosArch:     "amd64",
				Command:       "terraform apply",
				Args:          []string{},
				Flags:         []string{},
				ExitCode:      0,
				GitSHA:        "abc123def456",
				RepoURL:       "https://github.com/org/repo",
				RepoName:      "repo",
				RepoOwner:     "org",
				RepoHost:      "github.com",
				Metrics: dtos.ResourceUsageMetrics{
					WallTimeMS:      5678,
					UserCPUTimeMS:   3200,
					SystemCPUTimeMS: 400,
				},
				Data: data,
			})
		})
	require.NoError(t, err)
}

// TestPact_UploadExecMetadata_ApplyFailure verifies the consumer contract for
// POST /api/v1/atmos/exec with a `terraform apply` failure-shaped `data`
// payload — `has_errors: true` with populated `errors`, mirroring
// `TestBuildTerraformExecData_ApplyFailure` (cmd/terraform/utils_exec_metadata_test.go)
// at the contract level so a provider-side error-handling regression on the
// apply failure path is caught here too.
func TestPact_UploadExecMetadata_ApplyFailure(t *testing.T) {
	mockProvider := newHTTPMockProvider(t)

	err := mockProvider.
		AddInteraction().
		Given("workspace exists and accepts execution metadata").
		UponReceiving("a request to upload failed apply command-execution metadata").
		WithRequest("POST", "/api/v1/atmos/exec", func(b *consumer.V2RequestBuilder) {
			b.Header("Authorization", matchers.Like("Bearer test-token")).
				Header("Content-Type", matchers.S("application/json")).
				JSONBody(body{
					"execution_id":     matchers.Like("33f3b3f3-89ab-4c3d-ae3f-3456789012cd"),
					"atmos_pro_run_id": matchers.Like("run-12345"),
					"atmos_version":    matchers.Like("1.2.3"),
					"atmos_os":         matchers.Like("linux"),
					"atmos_arch":       matchers.Like("amd64"),
					"command":          matchers.Like("terraform apply"),
					"args":             []interface{}{},
					"flags":            []interface{}{},
					"exit_code":        matchers.Like(1),
					"git_sha":          matchers.Like("abc123def456"),
					"repo_url":         matchers.Like("https://github.com/org/repo"),
					"repo_name":        matchers.Like("repo"),
					"repo_owner":       matchers.Like("org"),
					"repo_host":        matchers.Like("github.com"),
					"metrics": body{
						"wall_time_ms":       matchers.Like(890),
						"user_cpu_time_ms":   matchers.Like(500),
						"system_cpu_time_ms": matchers.Like(60),
					},
					"data": body{
						"version": 1,
						"components": matchers.EachLike(body{
							"resource_counts": body{
								"create":  matchers.Like(0),
								"change":  matchers.Like(0),
								"replace": matchers.Like(0),
								"destroy": matchers.Like(0),
							},
							"outputs":     body{},
							"warnings":    []interface{}{},
							"changes":     []interface{}{},
							"has_changes": matchers.Like(false),
							"has_errors":  matchers.Like(true),
							"errors":      matchers.EachLike(matchers.Like("Error: creating S3 Bucket: AccessDenied"), 1),
							"exit_code":   matchers.Like(1),
							"component":   matchers.Like("vpc"),
							"stack":       matchers.Like("plat-use2-dev"),
							"logs":        matchers.Like(base64.StdEncoding.EncodeToString([]byte("Error: creating S3 Bucket: AccessDenied"))),
						}, 1),
					},
				})
		}).
		WillRespondWith(200, func(b *consumer.V2ResponseBuilder) {
			b.JSONBody(body{
				"success": matchers.Like(true),
			})
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			client := newPactClient(config)
			data, err := json.Marshal(map[string]any{
				"version": 1,
				"components": []map[string]any{
					{
						"resource_counts": map[string]any{
							"create":  0,
							"change":  0,
							"replace": 0,
							"destroy": 0,
						},
						"outputs":     map[string]any{},
						"warnings":    []string{},
						"changes":     []map[string]any{},
						"has_changes": false,
						"has_errors":  true,
						"errors":      []string{"Error: creating S3 Bucket: AccessDenied"},
						"exit_code":   1,
						"component":   "vpc",
						"stack":       "plat-use2-dev",
						"logs":        base64.StdEncoding.EncodeToString([]byte("Error: creating S3 Bucket: AccessDenied")),
					},
				},
			})
			if err != nil {
				return err
			}
			return client.UploadExecMetadata(&dtos.ExecUploadRequest{
				ExecutionID:   "33f3b3f3-89ab-4c3d-ae3f-3456789012cd",
				AtmosProRunID: "run-12345",
				AtmosVersion:  "1.2.3",
				AtmosOS:       "linux",
				AtmosArch:     "amd64",
				Command:       "terraform apply",
				Args:          []string{},
				Flags:         []string{},
				ExitCode:      1,
				GitSHA:        "abc123def456",
				RepoURL:       "https://github.com/org/repo",
				RepoName:      "repo",
				RepoOwner:     "org",
				RepoHost:      "github.com",
				Metrics: dtos.ResourceUsageMetrics{
					WallTimeMS:      890,
					UserCPUTimeMS:   500,
					SystemCPUTimeMS: 60,
				},
				Data: data,
			})
		})
	require.NoError(t, err)
}

// TestPact_UploadExecMetadata_NoData verifies the consumer contract for
// POST /api/v1/atmos/exec when the invoking command has no structured-data
// extension (e.g. a non-terraform command) — `data` is absent entirely,
// per spec Acceptance Scenario US3.4.
func TestPact_UploadExecMetadata_NoData(t *testing.T) {
	mockProvider := newHTTPMockProvider(t)

	err := mockProvider.
		AddInteraction().
		Given("workspace exists and accepts execution metadata").
		UponReceiving("a request to upload command-execution metadata with no structured data").
		WithRequest("POST", "/api/v1/atmos/exec", func(b *consumer.V2RequestBuilder) {
			b.Header("Authorization", matchers.Like("Bearer test-token")).
				Header("Content-Type", matchers.S("application/json")).
				JSONBody(body{
					"execution_id":     matchers.Like("c4c2f3c1-2345-4b2c-9d2e-234567890abc"),
					"atmos_pro_run_id": matchers.Like("run-12345"),
					"atmos_version":    matchers.Like("1.2.3"),
					"atmos_os":         matchers.Like("linux"),
					"atmos_arch":       matchers.Like("amd64"),
					"command":          matchers.Like("list components"),
					"args":             []interface{}{},
					"flags":            []interface{}{},
					"exit_code":        matchers.Like(0),
					"git_sha":          matchers.Like("abc123def456"),
					"repo_url":         matchers.Like("https://github.com/org/repo"),
					"repo_name":        matchers.Like("repo"),
					"repo_owner":       matchers.Like("org"),
					"repo_host":        matchers.Like("github.com"),
					"metrics": body{
						"wall_time_ms":       matchers.Like(45),
						"user_cpu_time_ms":   matchers.Like(20),
						"system_cpu_time_ms": matchers.Like(5),
					},
				})
		}).
		WillRespondWith(200, func(b *consumer.V2ResponseBuilder) {
			b.JSONBody(body{
				"success": matchers.Like(true),
			})
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			client := newPactClient(config)
			return client.UploadExecMetadata(&dtos.ExecUploadRequest{
				ExecutionID:   "c4c2f3c1-2345-4b2c-9d2e-234567890abc",
				AtmosProRunID: "run-12345",
				AtmosVersion:  "1.2.3",
				AtmosOS:       "linux",
				AtmosArch:     "amd64",
				Command:       "atmos list components",
				Args:          []string{},
				Flags:         []string{},
				ExitCode:      0,
				GitSHA:        "abc123def456",
				RepoURL:       "https://github.com/org/repo",
				RepoName:      "repo",
				RepoOwner:     "org",
				RepoHost:      "github.com",
				Metrics: dtos.ResourceUsageMetrics{
					WallTimeMS:      45,
					UserCPUTimeMS:   20,
					SystemCPUTimeMS: 5,
				},
			})
		})
	require.NoError(t, err)
}

// TestPact_UploadExecMetadata_BlobURL verifies the consumer contract for the
// out-of-band delivery path (FR-011, research.md Decision 16): when the
// marshaled record is at/over the payload size threshold, the client first
// calls POST /api/v1/atmos/exec/data (keyed by execution_id) to upload Data,
// then sends POST /api/v1/atmos/exec with Data replaced by the returned URL
// (a JSON string, not an inline object) — replacing the retired multi-chunk
// model (no batch_id/batch_index/batch_total anywhere). MaxPayloadBytes is
// set to 1 to deterministically force this path regardless of envelope size.
func TestPact_UploadExecMetadata_BlobURL(t *testing.T) {
	mockProvider := newHTTPMockProvider(t)

	const executionID = "d5d3a4d2-3456-4c3d-ae3f-34567890abcd"
	const blobURL = "https://blob.vercel-storage.com/atmos-exec/d5d3a4d2/data.json"

	mockProvider.
		AddInteraction().
		Given("workspace exists and accepts execution metadata").
		UponReceiving("a request to upload out-of-band command-execution structured data").
		WithRequest("POST", "/api/v1/atmos/exec/data", func(b *consumer.V2RequestBuilder) {
			b.Header("Authorization", matchers.Like("Bearer test-token")).
				Header("Content-Type", matchers.S("application/json")).
				JSONBody(body{
					"execution_id": matchers.Like(executionID),
					// Same {"version": 1, "components": [TerraformExecData, ...]}
					// wrapper as the inline case (interaction 9) — the blob-URL path
					// uploads the identical structured Data verbatim out-of-band, it
					// never reshapes it (research.md Decisions 37/38).
					"data": body{
						"version": 1,
						"components": matchers.EachLike(body{
							"resource_counts": body{
								"create":  matchers.Like(2),
								"change":  matchers.Like(1),
								"replace": matchers.Like(0),
								"destroy": matchers.Like(0),
							},
							"outputs":  body{},
							"warnings": []interface{}{},
							"changes": matchers.EachLike(body{
								"action":  matchers.Like("created"),
								"address": matchers.Like("aws_s3_bucket.example"),
							}, 1),
							"has_changes": matchers.Like(true),
							"has_errors":  matchers.Like(false),
							"errors":      []interface{}{},
							"exit_code":   matchers.Like(2),
							"component":   matchers.Like("vpc"),
							"stack":       matchers.Like("plat-use2-dev"),
							"logs":        matchers.Like(base64.StdEncoding.EncodeToString([]byte("Plan: 2 to add, 1 to change, 0 to destroy."))),
						}, 1),
					},
				})
		}).
		WillRespondWith(200, func(b *consumer.V2ResponseBuilder) {
			b.JSONBody(body{
				"success": matchers.Like(true),
				"url":     matchers.Like(blobURL),
			})
		})

	err := mockProvider.
		AddInteraction().
		Given("workspace exists and accepts execution metadata").
		UponReceiving("a request to upload command-execution metadata with out-of-band data").
		WithRequest("POST", "/api/v1/atmos/exec", func(b *consumer.V2RequestBuilder) {
			b.Header("Authorization", matchers.Like("Bearer test-token")).
				Header("Content-Type", matchers.S("application/json")).
				JSONBody(body{
					"execution_id":     matchers.Like(executionID),
					"atmos_pro_run_id": matchers.Like(""),
					"atmos_version":    matchers.Like(""),
					"atmos_os":         matchers.Like(""),
					"atmos_arch":       matchers.Like(""),
					"command":          matchers.Like("terraform plan"),
					"args":             []interface{}{},
					"flags":            []interface{}{},
					"exit_code":        matchers.Like(0),
					"git_sha":          matchers.Like(""),
					"repo_url":         matchers.Like(""),
					"repo_name":        matchers.Like(""),
					"repo_owner":       matchers.Like(""),
					"repo_host":        matchers.Like(""),
					"metrics": body{
						"wall_time_ms":       matchers.Like(0),
						"user_cpu_time_ms":   matchers.Like(0),
						"system_cpu_time_ms": matchers.Like(0),
					},
					"data": matchers.Like(blobURL),
				})
		}).
		WillRespondWith(200, func(b *consumer.V2ResponseBuilder) {
			b.JSONBody(body{"success": matchers.Like(true)})
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			client := newPactClient(config)
			client.MaxPayloadBytes = 1 // Forces the out-of-band path regardless of envelope size.

			data, err := json.Marshal(map[string]any{
				"version": 1,
				"components": []map[string]any{
					{
						"resource_counts": map[string]any{
							"create":  2,
							"change":  1,
							"replace": 0,
							"destroy": 0,
						},
						"outputs":  map[string]any{},
						"warnings": []string{},
						"changes": []map[string]any{
							{"action": "created", "address": "aws_s3_bucket.example"},
						},
						"has_changes": true,
						"has_errors":  false,
						"errors":      []string{},
						"exit_code":   2,
						"component":   "vpc",
						"stack":       "plat-use2-dev",
						"logs":        base64.StdEncoding.EncodeToString([]byte("Plan: 2 to add, 1 to change, 0 to destroy.")),
					},
				},
			})
			if err != nil {
				return err
			}

			return client.UploadExecMetadata(&dtos.ExecUploadRequest{
				ExecutionID: executionID,
				Command:     "atmos terraform plan",
				Args:        []string{},
				Flags:       []string{},
				ExitCode:    0,
				Data:        data,
			})
		})
	require.NoError(t, err)
}

// TestPact_UploadExecMetadata_MultiComponent verifies the consumer contract
// for POST /api/v1/atmos/exec carrying a multi-component `terraform plan
// --affected`/`--all` run's structured Data shape (FR-006a, spec.md Session
// 2026-08-21 restructure): `{"version": 1, "components": [TerraformExecData,
// ...]}` — the same unified shape TestPact_UploadExecMetadata already covers
// for a single component, exercised here with a components list of length >1
// to prove the list itself, not just the wrapper. Per-component entries omit
// their own "version" field — it's redundant with the outer wrapper's.
func TestPact_UploadExecMetadata_MultiComponent(t *testing.T) {
	mockProvider := newHTTPMockProvider(t)

	err := mockProvider.
		AddInteraction().
		Given("workspace exists and accepts execution metadata").
		UponReceiving("a request to upload multi-component command-execution metadata").
		WithRequest("POST", "/api/v1/atmos/exec", func(b *consumer.V2RequestBuilder) {
			b.Header("Authorization", matchers.Like("Bearer test-token")).
				Header("Content-Type", matchers.S("application/json")).
				JSONBody(body{
					"execution_id":     matchers.Like("11e1f1e1-6789-4a1b-8c1d-1234567890ab"),
					"atmos_pro_run_id": matchers.Like("run-12345"),
					"atmos_version":    matchers.Like("1.2.3"),
					"atmos_os":         matchers.Like("linux"),
					"atmos_arch":       matchers.Like("amd64"),
					"command":          matchers.Like("terraform plan"),
					"args":             []interface{}{},
					"flags":            matchers.EachLike("--affected", 1),
					"exit_code":        matchers.Like(0),
					"git_sha":          matchers.Like("abc123def456"),
					"repo_url":         matchers.Like("https://github.com/org/repo"),
					"repo_name":        matchers.Like("repo"),
					"repo_owner":       matchers.Like("org"),
					"repo_host":        matchers.Like("github.com"),
					"metrics": body{
						"wall_time_ms":       matchers.Like(2345),
						"user_cpu_time_ms":   matchers.Like(1200),
						"system_cpu_time_ms": matchers.Like(200),
					},
					"data": body{
						"version": 1,
						"components": matchers.EachLike(body{
							"resource_counts": body{
								"create":  matchers.Like(2),
								"change":  matchers.Like(0),
								"replace": matchers.Like(0),
								"destroy": matchers.Like(0),
							},
							"outputs":  body{},
							"warnings": []interface{}{},
							"changes": matchers.EachLike(body{
								"action":  matchers.Like("created"),
								"address": matchers.Like("aws_vpc.this"),
							}, 1),
							"has_changes": matchers.Like(true),
							"has_errors":  matchers.Like(false),
							"errors":      []interface{}{},
							"exit_code":   matchers.Like(2),
							"component":   matchers.Like("vpc"),
							"stack":       matchers.Like("plat-use2-dev"),
							"logs":        matchers.Like(base64.StdEncoding.EncodeToString([]byte("Plan: 2 to add, 0 to change, 0 to destroy."))),
						}, 1),
					},
				})
		}).
		WillRespondWith(200, func(b *consumer.V2ResponseBuilder) {
			b.JSONBody(body{
				"success": matchers.Like(true),
			})
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			client := newPactClient(config)
			data, err := json.Marshal(map[string]any{
				"version": 1,
				"components": []map[string]any{
					{
						"resource_counts": map[string]any{
							"create":  2,
							"change":  0,
							"replace": 0,
							"destroy": 0,
						},
						"outputs":  map[string]any{},
						"warnings": []string{},
						"changes": []map[string]any{
							{"action": "created", "address": "aws_vpc.this"},
						},
						"has_changes": true,
						"has_errors":  false,
						"errors":      []string{},
						"exit_code":   2,
						"component":   "vpc",
						"stack":       "plat-use2-dev",
						"logs":        base64.StdEncoding.EncodeToString([]byte("Plan: 2 to add, 0 to change, 0 to destroy.")),
					},
				},
			})
			if err != nil {
				return err
			}
			return client.UploadExecMetadata(&dtos.ExecUploadRequest{
				ExecutionID:   "11e1f1e1-6789-4a1b-8c1d-1234567890ab",
				AtmosProRunID: "run-12345",
				AtmosVersion:  "1.2.3",
				AtmosOS:       "linux",
				AtmosArch:     "amd64",
				Command:       "terraform plan",
				Args:          []string{},
				Flags:         []string{"--affected"},
				ExitCode:      0,
				GitSHA:        "abc123def456",
				RepoURL:       "https://github.com/org/repo",
				RepoName:      "repo",
				RepoOwner:     "org",
				RepoHost:      "github.com",
				Metrics: dtos.ResourceUsageMetrics{
					WallTimeMS:      2345,
					UserCPUTimeMS:   1200,
					SystemCPUTimeMS: 200,
				},
				Data: data,
			})
		})
	require.NoError(t, err)
}

// TestPact_UploadExecMetadata_DescribeAffected verifies the consumer
// contract for POST /api/v1/atmos/exec carrying `describe affected`'s
// structured Data shape (`{version, stacks}` — FR-006b, research.md Decision
// 22, contracts/interactions.md interaction 12), inline mode.
func TestPact_UploadExecMetadata_DescribeAffected(t *testing.T) {
	mockProvider := newHTTPMockProvider(t)

	err := mockProvider.
		AddInteraction().
		Given("workspace exists and accepts execution metadata").
		UponReceiving("a request to upload describe-affected execution metadata with inline data").
		WithRequest("POST", "/api/v1/atmos/exec", func(b *consumer.V2RequestBuilder) {
			b.Header("Authorization", matchers.Like("Bearer test-token")).
				Header("Content-Type", matchers.S("application/json")).
				JSONBody(body{
					"execution_id":     matchers.Like("e6e4b5e3-4567-4d4e-bf4a-4567890abcde"),
					"atmos_pro_run_id": matchers.Like("run-12345"),
					"atmos_version":    matchers.Like("1.2.3"),
					"atmos_os":         matchers.Like("linux"),
					"atmos_arch":       matchers.Like("amd64"),
					"command":          matchers.Like("describe affected"),
					"args":             []interface{}{},
					"flags":            []interface{}{},
					"exit_code":        matchers.Like(0),
					"git_sha":          matchers.Like("abc123def456"),
					"repo_url":         matchers.Like("https://github.com/org/repo"),
					"repo_name":        matchers.Like("repo"),
					"repo_owner":       matchers.Like("org"),
					"repo_host":        matchers.Like("github.com"),
					"metrics": body{
						"wall_time_ms":       matchers.Like(900),
						"user_cpu_time_ms":   matchers.Like(400),
						"system_cpu_time_ms": matchers.Like(80),
					},
					"data": body{
						"version": 1,
						"stacks": matchers.EachLike(body{
							"component":              matchers.Like("vpc"),
							"stack":                  matchers.Like("plat-use2-dev"),
							"component_type":         matchers.Like("terraform"),
							"component_path":         matchers.Like(""),
							"stack_slug":             matchers.Like(""),
							"affected":               matchers.Like("component"),
							"affected_all":           nil,
							"dependents":             []interface{}{},
							"included_in_dependents": matchers.Like(false),
							"settings":               nil,
						}, 1),
					},
				})
		}).
		WillRespondWith(200, func(b *consumer.V2ResponseBuilder) {
			b.JSONBody(body{
				"success": matchers.Like(true),
			})
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			client := newPactClient(config)
			data, err := json.Marshal(map[string]any{
				"version": 1,
				"stacks": []schema.Affected{
					{Component: "vpc", ComponentType: "terraform", Stack: "plat-use2-dev", Affected: "component", Dependents: []schema.Dependent{}},
				},
			})
			if err != nil {
				return err
			}
			return client.UploadExecMetadata(&dtos.ExecUploadRequest{
				ExecutionID:   "e6e4b5e3-4567-4d4e-bf4a-4567890abcde",
				AtmosProRunID: "run-12345",
				AtmosVersion:  "1.2.3",
				AtmosOS:       "linux",
				AtmosArch:     "amd64",
				Command:       "describe affected",
				Args:          []string{},
				Flags:         []string{},
				ExitCode:      0,
				GitSHA:        "abc123def456",
				RepoURL:       "https://github.com/org/repo",
				RepoName:      "repo",
				RepoOwner:     "org",
				RepoHost:      "github.com",
				Metrics: dtos.ResourceUsageMetrics{
					WallTimeMS:      900,
					UserCPUTimeMS:   400,
					SystemCPUTimeMS: 80,
				},
				Data: data,
			})
		})
	require.NoError(t, err)
}

// TestPact_UploadExecMetadata_DescribeAffected_BlobURL verifies the
// consumer contract for the out-of-band delivery path carrying
// `describe affected`'s structured Data shape (contracts/interactions.md
// interactions 13/14), constructed directly rather than routed through the
// real size-threshold decision code (research.md Decision 25).
func TestPact_UploadExecMetadata_DescribeAffected_BlobURL(t *testing.T) {
	mockProvider := newHTTPMockProvider(t)

	const executionID = "f7f5c6f4-5678-4e5f-cf5b-567890abcdef"
	const blobURL = "https://blob.vercel-storage.com/atmos-exec/f7f5c6f4/data.json"

	mockProvider.
		AddInteraction().
		Given("workspace exists and accepts execution metadata").
		UponReceiving("a request to upload describe-affected out-of-band command-execution structured data").
		WithRequest("POST", "/api/v1/atmos/exec/data", func(b *consumer.V2RequestBuilder) {
			b.Header("Authorization", matchers.Like("Bearer test-token")).
				Header("Content-Type", matchers.S("application/json")).
				JSONBody(body{
					"execution_id": matchers.Like(executionID),
					"data": body{
						"version": 1,
						"stacks": matchers.EachLike(body{
							"component":              matchers.Like("vpc"),
							"stack":                  matchers.Like("plat-use2-dev"),
							"component_type":         matchers.Like("terraform"),
							"component_path":         matchers.Like(""),
							"stack_slug":             matchers.Like(""),
							"affected":               matchers.Like("component"),
							"affected_all":           nil,
							"dependents":             []interface{}{},
							"included_in_dependents": matchers.Like(false),
							"settings":               nil,
						}, 1),
					},
				})
		}).
		WillRespondWith(200, func(b *consumer.V2ResponseBuilder) {
			b.JSONBody(body{
				"success": matchers.Like(true),
				"url":     matchers.Like(blobURL),
			})
		})

	err := mockProvider.
		AddInteraction().
		Given("workspace exists and accepts execution metadata").
		UponReceiving("a request to upload describe-affected execution metadata with out-of-band data").
		WithRequest("POST", "/api/v1/atmos/exec", func(b *consumer.V2RequestBuilder) {
			b.Header("Authorization", matchers.Like("Bearer test-token")).
				Header("Content-Type", matchers.S("application/json")).
				JSONBody(body{
					"execution_id":     matchers.Like(executionID),
					"atmos_pro_run_id": matchers.Like(""),
					"atmos_version":    matchers.Like(""),
					"atmos_os":         matchers.Like(""),
					"atmos_arch":       matchers.Like(""),
					"command":          matchers.Like("describe affected"),
					"args":             []interface{}{},
					"flags":            []interface{}{},
					"exit_code":        matchers.Like(0),
					"git_sha":          matchers.Like(""),
					"repo_url":         matchers.Like(""),
					"repo_name":        matchers.Like(""),
					"repo_owner":       matchers.Like(""),
					"repo_host":        matchers.Like(""),
					"metrics": body{
						"wall_time_ms":       matchers.Like(0),
						"user_cpu_time_ms":   matchers.Like(0),
						"system_cpu_time_ms": matchers.Like(0),
					},
					"data": matchers.Like(blobURL),
				})
		}).
		WillRespondWith(200, func(b *consumer.V2ResponseBuilder) {
			b.JSONBody(body{"success": matchers.Like(true)})
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			client := newPactClient(config)
			client.MaxPayloadBytes = 1 // Forces the out-of-band path regardless of envelope size.

			data, err := json.Marshal(map[string]any{
				"version": 1,
				"stacks": []schema.Affected{
					{Component: "vpc", ComponentType: "terraform", Stack: "plat-use2-dev", Affected: "component", Dependents: []schema.Dependent{}},
				},
			})
			if err != nil {
				return err
			}

			return client.UploadExecMetadata(&dtos.ExecUploadRequest{
				ExecutionID: executionID,
				Command:     "describe affected",
				Args:        []string{},
				Flags:       []string{},
				ExitCode:    0,
				Data:        data,
			})
		})
	require.NoError(t, err)
}

// TestPact_UploadExecMetadata_ListInstances verifies the consumer contract
// for POST /api/v1/atmos/exec carrying `list instances`' structured Data
// shape (`{version, instances}` — FR-006c, research.md Decision 23,
// contracts/interactions.md interaction 15), inline mode. This shape is only
// ever present when `--upload` was passed for the invocation.
func TestPact_UploadExecMetadata_ListInstances(t *testing.T) {
	mockProvider := newHTTPMockProvider(t)

	err := mockProvider.
		AddInteraction().
		Given("workspace exists and accepts execution metadata").
		UponReceiving("a request to upload list-instances execution metadata with inline data").
		WithRequest("POST", "/api/v1/atmos/exec", func(b *consumer.V2RequestBuilder) {
			b.Header("Authorization", matchers.Like("Bearer test-token")).
				Header("Content-Type", matchers.S("application/json")).
				JSONBody(body{
					"execution_id":     matchers.Like("a8a6d7a5-6789-4f6a-df6c-67890abcdef0"),
					"atmos_pro_run_id": matchers.Like("run-12345"),
					"atmos_version":    matchers.Like("1.2.3"),
					"atmos_os":         matchers.Like("linux"),
					"atmos_arch":       matchers.Like("amd64"),
					"command":          matchers.Like("list instances"),
					"args":             []interface{}{},
					"flags":            matchers.EachLike("--upload", 1),
					"exit_code":        matchers.Like(0),
					"git_sha":          matchers.Like("abc123def456"),
					"repo_url":         matchers.Like("https://github.com/org/repo"),
					"repo_name":        matchers.Like("repo"),
					"repo_owner":       matchers.Like("org"),
					"repo_host":        matchers.Like("github.com"),
					"metrics": body{
						"wall_time_ms":       matchers.Like(600),
						"user_cpu_time_ms":   matchers.Like(250),
						"system_cpu_time_ms": matchers.Like(50),
					},
					"data": body{
						"version": 1,
						"instances": matchers.EachLike(body{
							"component":      matchers.Like("vpc"),
							"stack":          matchers.Like("plat-use2-dev"),
							"component_type": matchers.Like("terraform"),
						}, 1),
					},
				})
		}).
		WillRespondWith(200, func(b *consumer.V2ResponseBuilder) {
			b.JSONBody(body{
				"success": matchers.Like(true),
			})
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			client := newPactClient(config)
			data, err := json.Marshal(map[string]any{
				"version": 1,
				"instances": []dtos.UploadInstance{
					{Component: "vpc", Stack: "plat-use2-dev", ComponentType: "terraform"},
				},
			})
			if err != nil {
				return err
			}
			return client.UploadExecMetadata(&dtos.ExecUploadRequest{
				ExecutionID:   "a8a6d7a5-6789-4f6a-df6c-67890abcdef0",
				AtmosProRunID: "run-12345",
				AtmosVersion:  "1.2.3",
				AtmosOS:       "linux",
				AtmosArch:     "amd64",
				Command:       "list instances",
				Args:          []string{},
				Flags:         []string{"--upload"},
				ExitCode:      0,
				GitSHA:        "abc123def456",
				RepoURL:       "https://github.com/org/repo",
				RepoName:      "repo",
				RepoOwner:     "org",
				RepoHost:      "github.com",
				Metrics: dtos.ResourceUsageMetrics{
					WallTimeMS:      600,
					UserCPUTimeMS:   250,
					SystemCPUTimeMS: 50,
				},
				Data: data,
			})
		})
	require.NoError(t, err)
}

// TestPact_UploadExecMetadata_ListInstances_BlobURL verifies the consumer
// contract for the out-of-band delivery path carrying `list instances`'
// structured Data shape (contracts/interactions.md interaction 15's
// blob-URL sub-case, paired with its own UploadExecData interaction),
// constructed directly rather than routed through the real size-threshold
// decision code (research.md Decision 25).
func TestPact_UploadExecMetadata_ListInstances_BlobURL(t *testing.T) {
	mockProvider := newHTTPMockProvider(t)

	const executionID = "b9b7e8b6-789a-4a7b-ea7d-7890abcdef01"
	const blobURL = "https://blob.vercel-storage.com/atmos-exec/b9b7e8b6/data.json"

	mockProvider.
		AddInteraction().
		Given("workspace exists and accepts execution metadata").
		UponReceiving("a request to upload list-instances out-of-band command-execution structured data").
		WithRequest("POST", "/api/v1/atmos/exec/data", func(b *consumer.V2RequestBuilder) {
			b.Header("Authorization", matchers.Like("Bearer test-token")).
				Header("Content-Type", matchers.S("application/json")).
				JSONBody(body{
					"execution_id": matchers.Like(executionID),
					"data": body{
						"version": 1,
						"instances": matchers.EachLike(body{
							"component":      matchers.Like("vpc"),
							"stack":          matchers.Like("plat-use2-dev"),
							"component_type": matchers.Like("terraform"),
						}, 1),
					},
				})
		}).
		WillRespondWith(200, func(b *consumer.V2ResponseBuilder) {
			b.JSONBody(body{
				"success": matchers.Like(true),
				"url":     matchers.Like(blobURL),
			})
		})

	err := mockProvider.
		AddInteraction().
		Given("workspace exists and accepts execution metadata").
		UponReceiving("a request to upload list-instances execution metadata with out-of-band data").
		WithRequest("POST", "/api/v1/atmos/exec", func(b *consumer.V2RequestBuilder) {
			b.Header("Authorization", matchers.Like("Bearer test-token")).
				Header("Content-Type", matchers.S("application/json")).
				JSONBody(body{
					"execution_id":     matchers.Like(executionID),
					"atmos_pro_run_id": matchers.Like(""),
					"atmos_version":    matchers.Like(""),
					"atmos_os":         matchers.Like(""),
					"atmos_arch":       matchers.Like(""),
					"command":          matchers.Like("list instances"),
					"args":             []interface{}{},
					"flags":            matchers.EachLike("--upload", 1),
					"exit_code":        matchers.Like(0),
					"git_sha":          matchers.Like(""),
					"repo_url":         matchers.Like(""),
					"repo_name":        matchers.Like(""),
					"repo_owner":       matchers.Like(""),
					"repo_host":        matchers.Like(""),
					"metrics": body{
						"wall_time_ms":       matchers.Like(0),
						"user_cpu_time_ms":   matchers.Like(0),
						"system_cpu_time_ms": matchers.Like(0),
					},
					"data": matchers.Like(blobURL),
				})
		}).
		WillRespondWith(200, func(b *consumer.V2ResponseBuilder) {
			b.JSONBody(body{"success": matchers.Like(true)})
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			client := newPactClient(config)
			client.MaxPayloadBytes = 1 // Forces the out-of-band path regardless of envelope size.

			data, err := json.Marshal(map[string]any{
				"version": 1,
				"instances": []dtos.UploadInstance{
					{Component: "vpc", Stack: "plat-use2-dev", ComponentType: "terraform"},
				},
			})
			if err != nil {
				return err
			}

			return client.UploadExecMetadata(&dtos.ExecUploadRequest{
				ExecutionID: executionID,
				Command:     "list instances",
				Args:        []string{},
				Flags:       []string{"--upload"},
				ExitCode:    0,
				Data:        data,
			})
		})
	require.NoError(t, err)
}
