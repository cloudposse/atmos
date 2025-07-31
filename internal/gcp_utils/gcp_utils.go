package gcp_utils

import (
	"context"
	"fmt"

	"cloud.google.com/go/storage"
	"google.golang.org/api/option"

	errUtils "github.com/cloudposse/atmos/errors"
)

// LoadGCPStorageClient loads a GCP Storage client.
/*
	It looks for credentials in the following order:

	Environment variables:
	  GOOGLE_APPLICATION_CREDENTIALS (path to service account JSON file)
	  GOOGLE_CLOUD_PROJECT (for project ID)

	Service Account JSON file:
	  Path specified by GOOGLE_APPLICATION_CREDENTIALS
	  Or provided programmatically via option.WithCredentialsFile()

	Google Cloud SDK credentials:
	  From gcloud auth application-default login
	  Typically at ~/.config/gcloud/application_default_credentials.json

	Google Compute Engine metadata server:
	  If running on GCE, GKE, Cloud Run, etc.
	  Uses the service account attached to the compute instance

	Workload Identity (for GKE):
	  When running in GKE with Workload Identity enabled
	  Kubernetes service account mapped to Google service account

	Custom credential sources:
	  Provided programmatically using option.WithCredentials(...)
*/
func LoadGCPStorageClient(ctx context.Context, credentialsPath string, impersonateServiceAccount string) (*storage.Client, error) {
	var opts []option.ClientOption

	// Conditionally set credentials file path
	if credentialsPath != "" {
		opts = append(opts, option.WithCredentialsFile(credentialsPath))
	}

	// Handle service account impersonation if specified
	if impersonateServiceAccount != "" {
		// Note: For full impersonation support, we would need to implement
		// credentials that use the impersonation service account.
		// This is a placeholder for future enhancement.
		opts = append(opts, option.WithQuotaProject(impersonateServiceAccount))
	}

	// Create the storage client
	client, err := storage.NewClient(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errUtils.ErrCreateGCSClient, err)
	}

	return client, nil
}