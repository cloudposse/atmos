//go:build pact

package pro

import (
	"fmt"
	"testing"

	"github.com/pact-foundation/pact-go/v2/consumer"
	"github.com/pact-foundation/pact-go/v2/matchers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
					"command":   matchers.Like("terraform plan"),
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
				Command:   "terraform plan",
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
