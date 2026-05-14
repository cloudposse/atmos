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
package gcp_wif
