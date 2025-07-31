package terraform_backend

import (
	"context"
	"fmt"
	"io"
	"path"
	"time"

	"cloud.google.com/go/storage"
	"google.golang.org/api/option"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// GetGCSBackendServiceAccount returns the service account configuration from the GCS backend config.
// This handles the various ways service accounts can be configured for GCS backend.
// https://developer.hashicorp.com/terraform/language/settings/backends/gcs#credentials
func GetGCSBackendServiceAccount(backend *map[string]any) string {
	// Check for credentials file path
	if credentialsPath, ok := (*backend)["credentials"].(string); ok && credentialsPath != "" {
		return credentialsPath
	}
	return ""
}

// GetGCSBackendImpersonateServiceAccount returns the impersonation service account from the GCS backend config.
// https://developer.hashicorp.com/terraform/language/settings/backends/gcs#impersonate_service_account
func GetGCSBackendImpersonateServiceAccount(backend *map[string]any) string {
	if serviceAccount, ok := (*backend)["impersonate_service_account"].(string); ok {
		return serviceAccount
	}
	return ""
}

// GCSClient defines an interface for interacting with Google Cloud Storage.
type GCSClient interface {
	Bucket(name string) GCSBucketHandle
}

// GCSBucketHandle defines an interface for interacting with a GCS bucket.
type GCSBucketHandle interface {
	Object(name string) GCSObjectHandle
}

// GCSObjectHandle defines an interface for interacting with a GCS object.
type GCSObjectHandle interface {
	NewReader(ctx context.Context) (io.ReadCloser, error)
}

// Concrete implementations for production use
type gcsClientImpl struct {
	client *storage.Client
}

func (c *gcsClientImpl) Bucket(name string) GCSBucketHandle {
	return &gcsBucketHandleImpl{bucket: c.client.Bucket(name)}
}

type gcsBucketHandleImpl struct {
	bucket *storage.BucketHandle
}

func (b *gcsBucketHandleImpl) Object(name string) GCSObjectHandle {
	return &gcsObjectHandleImpl{object: b.bucket.Object(name)}
}

type gcsObjectHandleImpl struct {
	object *storage.ObjectHandle
}

func (o *gcsObjectHandleImpl) NewReader(ctx context.Context) (io.ReadCloser, error) {
	return o.object.NewReader(ctx)
}

// ReadTerraformBackendGCS reads the Terraform state file from the configured GCS backend.
// If the state file does not exist in the bucket, the function returns `nil`.
func ReadTerraformBackendGCS(
	atmosConfig *schema.AtmosConfiguration,
	componentSections *map[string]any,
) ([]byte, error) {
	backend := GetComponentBackend(componentSections)
	
	// Extract the GCS-specific configuration section (same pattern as stack processor)
	gcsBackend := map[string]any{}
	if gcsSection, ok := backend["gcs"].(map[string]any); ok {
		gcsBackend = gcsSection
	} else {
		// If no nested gcs section, assume the backend config is already flattened (e.g., from stack processor)
		gcsBackend = backend
	}

	// 30 second timeout to read the state file from the GCS bucket (longer than S3 due to potential auth setup)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create GCS client with authentication
	gcsClient, err := createGCSClient(ctx, &gcsBackend)
	if err != nil {
		return nil, err
	}

	return ReadTerraformBackendGCSInternal(gcsClient, componentSections, &gcsBackend)
}

// createGCSClient creates a GCS client with proper authentication based on backend configuration.
func createGCSClient(ctx context.Context, backend *map[string]any) (GCSClient, error) {
	var opts []option.ClientOption

	// Handle credentials file if specified
	if credentialsPath := GetGCSBackendServiceAccount(backend); credentialsPath != "" {
		opts = append(opts, option.WithCredentialsFile(credentialsPath))
	}

	// Handle service account impersonation if specified
	if impersonateServiceAccount := GetGCSBackendImpersonateServiceAccount(backend); impersonateServiceAccount != "" {
		// For impersonation, we need to set up the credentials properly
		// This would typically involve creating credentials that impersonate the target service account
		opts = append(opts, option.WithQuotaProject(impersonateServiceAccount))
	}

	// Create the storage client
	client, err := storage.NewClient(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errUtils.ErrCreateGCSClient, err)
	}

	return &gcsClientImpl{client: client}, nil
}

// ReadTerraformBackendGCSInternal accepts a GCS client and reads the Terraform state file from the configured GCS backend.
func ReadTerraformBackendGCSInternal(
	gcsClient GCSClient,
	componentSections *map[string]any,
	backend *map[string]any,
) ([]byte, error) {
	// Build the path to the tfstate file in the GCS bucket
	// According to Terraform docs: "Named states for workspaces are stored in an object called `<prefix>/<workspace>.tfstate`"
	prefix := GetBackendAttribute(backend, "prefix")
	workspace := GetTerraformWorkspace(componentSections)
	
	var tfStateFilePath string
	if prefix == "" {
		// If no prefix is set, store at root level: <workspace>.tfstate
		tfStateFilePath = workspace + ".tfstate"
	} else {
		// If prefix is set: <prefix>/<workspace>.tfstate
		tfStateFilePath = path.Join(prefix, workspace+".tfstate")
	}

	bucket := GetBackendAttribute(backend, "bucket")
	if bucket == "" {
		return nil, fmt.Errorf("%w: bucket name is required for GCS backend", errUtils.ErrInvalidBackendConfig)
	}

	// 10 second timeout to read the state file from the GCS bucket
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get the object from GCS
	objectHandle := gcsClient.Bucket(bucket).Object(tfStateFilePath)
	reader, err := objectHandle.NewReader(ctx)
	if err != nil {
		// Check if the error is because the object doesn't exist
		// If the state file does not exist (the component in the stack has not been provisioned yet), return a `nil` result and no error
		if status.Code(err) == codes.NotFound {
			return nil, nil
		}
		// If any other error, return it
		return nil, fmt.Errorf("%w: %v", errUtils.ErrGetObjectFromGCS, err)
	}
	defer reader.Close()

	content, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errUtils.ErrReadGCSObjectBody, err)
	}

	return content, nil
}