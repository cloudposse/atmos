// Package gcp_wif implements the gcp/workload-identity-federation provider.
//
// This provider enables keyless authentication to GCP from external identity
// providers (GitHub Actions, GitLab CI, Azure DevOps, etc.) using Workload
// Identity Federation.
//
// The authentication flow:
//
//  1. Obtain OIDC token from external provider (via environment or file)
//  2. Exchange OIDC token with Google STS for a federated access token
//  3. Optionally impersonate a service account for the final access token
//
// Configuration example in atmos.yaml:
//
//	auth:
//	  providers:
//	    github-wif:
//	      kind: gcp/workload-identity-federation
//	      spec:
//	        project_id: my-project
//	        project_number: "123456789012"
//	        workload_identity_pool_id: github-pool
//	        workload_identity_provider_id: github-provider
//	        service_account_email: deploy@my-project.iam.gserviceaccount.com
//	        token_source:
//	          type: environment  # or "file"
//	          environment_variable: ACTIONS_ID_TOKEN_REQUEST_TOKEN
//	          # file_path: /path/to/oidc/token  # if type is "file"
//
// For GitHub Actions, set up OIDC token permissions:
//
//	permissions:
//	  id-token: write
//	  contents: read
//
// # Credential caching
//
// This provider is ambient: the OIDC token is read from the environment, a file, or a
// URL on every authentication and exchanged for a short-lived access token. It therefore
// implements types.AmbientProvider, which tells the auth manager never to persist
// credentials for chains rooted here to the keyring — a persisted token would be
// replayed after the ambient OIDC token has already been rotated.
package gcp_wif
