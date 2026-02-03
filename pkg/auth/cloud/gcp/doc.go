// Package gcp provides GCP-specific credential file management and environment
// setup for the Atmos authentication system.
//
// This package implements the XDG-compliant file isolation pattern, storing
// GCP credentials in ~/.config/atmos/gcp/<provider-name>/ to avoid conflicts
// with the user's existing gcloud configuration.
//
// Directory Structure:
//
//	~/.config/atmos/gcp/<provider-name>/
//	├── adc/
//	│   └── <identity-name>/
//	│       └── application_default_credentials.json
//	└── config/
//	    └── <identity-name>/
//	        ├── active_config
//	        ├── configurations/
//	        │   └── config_atmos
//	        └── properties
//
// Environment Variables:
//
// The package manages the following environment variables:
//   - GOOGLE_APPLICATION_CREDENTIALS: Path to ADC JSON file
//   - GOOGLE_CLOUD_PROJECT: Default project ID
//   - CLOUDSDK_CONFIG: Path to gcloud config directory
//   - CLOUDSDK_CORE_PROJECT: Project for gcloud commands
//   - GOOGLE_CLOUD_REGION: Default region
//
// Usage:
//
//	// Complete setup for an identity
//	err := gcp.Setup(ctx, atmosConfig, "my-provider", "my-identity", creds)
//
//	// Or step by step:
//	err := gcp.PrepareEnvironment(ctx, atmosConfig)
//	paths, err := gcp.SetupFiles(ctx, atmosConfig, "my-provider", "my-identity", creds)
//	err = gcp.SetAuthContext(authContext, "my-provider", "my-identity", creds)
//	err = gcp.SetEnvironmentVariables(ctx, atmosConfig, "my-provider", "my-identity")
package gcp
