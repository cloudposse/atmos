// Package gcp_project implements the gcp/project identity.
//
// This identity sets GCP project context without performing authentication.
// It configures environment variables for project and region, allowing tools
// like gcloud, terraform, and the GCP SDKs to target a specific project.
//
// Use this identity when:
//   - You have ambient credentials (ADC, metadata server, etc.)
//   - You need to target a specific project without switching credentials
//   - You want consistent project/region settings across tools
//
// Configuration example in atmos.yaml:
//
//	auth:
//	  identities:
//	    prod-context:
//	      kind: gcp/project
//	      principal:
//	        project_id: my-prod-project
//	        region: us-central1
//	        zone: us-central1-a  # optional
//
// This identity does NOT:
//   - Authenticate or obtain credentials
//   - Call any GCP APIs
//   - Require a provider
//
// Environment variables set:
//   - GOOGLE_CLOUD_PROJECT / GCLOUD_PROJECT / CLOUDSDK_CORE_PROJECT
//   - GOOGLE_CLOUD_REGION / CLOUDSDK_COMPUTE_REGION (if region specified)
//   - GOOGLE_CLOUD_ZONE / CLOUDSDK_COMPUTE_ZONE (if zone specified)
package gcp_project
