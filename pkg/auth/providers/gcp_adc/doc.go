// Package gcp_adc implements the gcp/adc authentication provider.
//
// This provider obtains credentials from Google Cloud Application Default
// Credentials (ADC), which searches for credentials in this order:
//
//  1. GOOGLE_APPLICATION_CREDENTIALS environment variable
//  2. User credentials from gcloud auth application-default login
//  3. Service account attached to GCP resource (GCE, Cloud Run, etc.)
//
// Configuration example in atmos.yaml:
//
//	auth:
//	  providers:
//	    my-gcp-adc:
//	      kind: gcp/adc
//	      spec:
//	        project_id: my-project        # optional, defaults to ADC project
//	        region: us-central1           # optional
//	        scopes:                        # optional, defaults to cloud-platform
//	          - https://www.googleapis.com/auth/cloud-platform
package gcp_adc
