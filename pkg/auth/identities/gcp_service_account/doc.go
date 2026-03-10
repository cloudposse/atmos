// Package gcp_service_account implements the gcp/service-account identity.
//
// This identity impersonates a GCP service account using credentials from
// an upstream provider. It uses the IAM Credentials API to generate access
// tokens for the target service account.
//
// Configuration example in atmos.yaml:
//
//	auth:
//	  identities:
//	    prod-deployer:
//	      kind: gcp/service-account
//	      provider: my-gcp-adc          # or any GCP provider
//	      principal:
//	        service_account_email: deployer@prod-project.iam.gserviceaccount.com
//	        # Optional: chain through intermediate service accounts
//	        delegates:
//	          - intermediate@proj.iam.gserviceaccount.com
//	        # Optional: override default scopes
//	        scopes:
//	          - https://www.googleapis.com/auth/cloud-platform
//	        # Optional: token lifetime (default 1h, max 12h with constraints)
//	        lifetime: 3600s
//
// Required IAM permissions on the provider's identity:
//   - roles/iam.serviceAccountTokenCreator on the target service account
//   - Or: iam.serviceAccounts.getAccessToken permission
//
// For delegation chains, each delegate must grant Token Creator to the next.
package gcp_service_account
