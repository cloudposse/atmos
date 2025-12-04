package terraform_backend

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/storage"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/gcp"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// maxGCSRetryCount defines the max attempts to read a state file from a GCS bucket.
const maxGCSRetryCount = 2

// GetGCSBackendCredentials returns the credentials configuration from the GCS backend config.
// This is a thin wrapper around the unified GCP authentication utility.
// https://developer.hashicorp.com/terraform/language/settings/backends/gcs#credentials
func GetGCSBackendCredentials(backend *map[string]any) string {
	defer perf.Track(nil, "terraform_backend.GetGCSBackendCredentials")()

	return gcp.GetCredentialsFromBackend(*backend)
}

// GetGCSBackendImpersonateServiceAccount returns the impersonation service account from the GCS backend config.
// https://developer.hashicorp.com/terraform/language/settings/backends/gcs#impersonate_service_account
func GetGCSBackendImpersonateServiceAccount(backend *map[string]any) string {
	defer perf.Track(nil, "terraform_backend.GetGCSBackendImpersonateServiceAccount")()

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

// Concrete implementations for production use.
type gcsClientImpl struct {
	client *storage.Client
}

func (c *gcsClientImpl) Bucket(name string) GCSBucketHandle {
	defer perf.Track(nil, "terraform_backend.gcsClientImpl.Bucket")()

	return &gcsBucketHandleImpl{bucket: c.client.Bucket(name)}
}

type gcsBucketHandleImpl struct {
	bucket *storage.BucketHandle
}

func (b *gcsBucketHandleImpl) Object(name string) GCSObjectHandle {
	defer perf.Track(nil, "terraform_backend.gcsBucketHandleImpl.Object")()

	return &gcsObjectHandleImpl{object: b.bucket.Object(name)}
}

type gcsObjectHandleImpl struct {
	object *storage.ObjectHandle
}

func (o *gcsObjectHandleImpl) NewReader(ctx context.Context) (io.ReadCloser, error) {
	defer perf.Track(nil, "terraform_backend.gcsObjectHandleImpl.NewReader")()

	return o.object.NewReader(ctx)
}

// gcsClientCache caches the GCS clients based on a deterministic cache key.
// It's a map[string]GCSClient.
var gcsClientCache sync.Map

func getCachedGCSClient(backend *map[string]any) (GCSClient, error) {
	credentials := GetGCSBackendCredentials(backend)
	impersonateServiceAccount := GetGCSBackendImpersonateServiceAccount(backend)

	// Build a deterministic cache key (hash credentials to avoid exposing sensitive data in cache key).
	// Create a proper hash to avoid collisions.
	h := sha256.Sum256([]byte(credentials))
	cacheKey := fmt.Sprintf("credentials_hash=%s;impersonate=%s",
		hex.EncodeToString(h[:8]), // Use first 8 bytes for brevity.
		impersonateServiceAccount)

	// Check the cache.
	if cached, ok := gcsClientCache.Load(cacheKey); ok {
		return cached.(GCSClient), nil
	}

	// Build the GCS client if not cached.
	// 30 sec timeout to configure a GCS client.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	gcsClient, err := createGCSClient(ctx, backend)
	if err != nil {
		return nil, err
	}

	gcsClientCache.Store(cacheKey, gcsClient)
	return gcsClient, nil
}

// ReadTerraformBackendGCS reads the Terraform state file from the configured GCS backend.
// If the state file does not exist in the bucket, the function returns `nil`.
func ReadTerraformBackendGCS(
	_ *schema.AtmosConfiguration,
	componentSections *map[string]any,
	_ *schema.AuthContext,
) ([]byte, error) {
	defer perf.Track(nil, "terraform_backend.ReadTerraformBackendGCS")()

	backend := GetComponentBackend(componentSections)

	// Extract the GCS-specific configuration section (same pattern as stack processor).
	gcsBackend := map[string]any{}
	if gcsSection, ok := backend[cfg.BackendTypeGCS].(map[string]any); ok {
		gcsBackend = gcsSection
	} else {
		// If no nested gcs section, assume the backend config is already flattened (e.g., from stack processor).
		gcsBackend = backend
	}

	gcsClient, err := getCachedGCSClient(&gcsBackend)
	if err != nil {
		return nil, err
	}

	return ReadTerraformBackendGCSInternal(gcsClient, componentSections, &gcsBackend)
}

// createGCSClient creates a GCS client with proper authentication based on backend configuration.
// This uses the unified GCP authentication utility for consistency across all Google Cloud services.
func createGCSClient(ctx context.Context, backend *map[string]any) (GCSClient, error) {
	credentials := GetGCSBackendCredentials(backend)
	impersonateServiceAccount := GetGCSBackendImpersonateServiceAccount(backend)

	// Use unified GCP authentication.
	opts := gcp.GetClientOptions(gcp.AuthOptions{
		Credentials: credentials,
	})

	if credentials != "" {
		if strings.HasPrefix(strings.TrimSpace(credentials), "{") {
			log.Debug("Using GCS credentials from JSON content")
		} else {
			log.Debug("Using GCS credentials from file", "path", credentials)
		}
	} else {
		log.Debug("Using default GCS credentials (ADC)")
	}

	// TODO: Handle service account impersonation properly.
	// This requires using impersonate.CredentialsTokenSource from google.golang.org/api/impersonate.
	if impersonateServiceAccount != "" {
		log.Debug("Service account impersonation requested but not yet implemented", "account", impersonateServiceAccount)
		// For now, we'll log a warning that this feature needs proper implementation.
		// In a production environment, this should use impersonate.CredentialsTokenSource.
	}

	// Create the storage client.
	client, err := storage.NewClient(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf(errWrapFormat, errUtils.ErrCreateGCSClient, err)
	}

	return &gcsClientImpl{client: client}, nil
}

// ReadTerraformBackendGCSInternal accepts a GCS client and reads the Terraform state file from the configured GCS backend.
func ReadTerraformBackendGCSInternal(
	gcsClient GCSClient,
	componentSections *map[string]any,
	backend *map[string]any,
) ([]byte, error) {
	defer perf.Track(nil, "terraform_backend.ReadTerraformBackendGCSInternal")()

	// Build the path to the tfstate file in the GCS bucket.
	// According to Terraform docs: "Named states for workspaces are stored in an object called `<prefix>/<workspace>.tfstate`".
	prefix := GetBackendAttribute(backend, "prefix")
	workspace := GetTerraformWorkspace(componentSections)
	if workspace == "" {
		workspace = "default"
	}

	var tfStateFilePath string
	if prefix == "" {
		// If no prefix is set, store at root level: <workspace>.tfstate.
		tfStateFilePath = workspace + ".tfstate"
	} else {
		// If prefix is set: <prefix>/<workspace>.tfstate.
		tfStateFilePath = path.Join(prefix, workspace+".tfstate")
	}

	bucket := GetBackendAttribute(backend, "bucket")
	if bucket == "" {
		return nil, errUtils.ErrGCSBucketRequired
	}

	var lastErr error
	for attempt := 0; attempt <= maxGCSRetryCount; attempt++ {
		// 30 sec timeout to read the state file from the GCS bucket.
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)

		// Get the object from GCS.
		objectHandle := gcsClient.Bucket(bucket).Object(tfStateFilePath)
		reader, err := objectHandle.NewReader(ctx)
		if err != nil {
			cancel() // Cancel immediately after use
			// Check if the error is because the object doesn't exist.
			// If the state file does not exist (the component in the stack has not been provisioned yet), return a `nil` result and no error.
			if errors.Is(err, storage.ErrObjectNotExist) || status.Code(err) == codes.NotFound {
				log.Debug("Terraform state file doesn't exist in the GCS bucket; returning 'null'", "file", tfStateFilePath, "bucket", bucket)
				return nil, nil
			}

			lastErr = err
			if attempt < maxGCSRetryCount {
				log.Debug("Failed to read Terraform state file from GCS bucket", "attempt", attempt+1, "file", tfStateFilePath, "bucket", bucket, "error", err)
				time.Sleep(time.Second * 2) // backoff
				continue
			}
			// Retries exhausted - log warning with error details to help diagnose the issue.
			logGCSRetryExhausted(err, tfStateFilePath, bucket, maxGCSRetryCount)
			return nil, fmt.Errorf(errWrapFormat, errUtils.ErrGetObjectFromGCS, lastErr)
		}

		content, err := io.ReadAll(reader)
		_ = reader.Close() // Explicit close instead of defer
		cancel()           // Cancel immediately after use
		if err != nil {
			return nil, fmt.Errorf(errWrapFormat, errUtils.ErrReadGCSObjectBody, err)
		}
		return content, nil
	}

	return nil, fmt.Errorf(errWrapFormat, errUtils.ErrGetObjectFromGCS, lastErr)
}

// logGCSRetryExhausted logs a warning when all retries are exhausted for GCS operations.
// This helps users report issues by providing the gRPC status code and details.
func logGCSRetryExhausted(err error, tfStateFilePath, bucket string, maxRetries int) {
	defer perf.Track(nil, "terraform_backend.logGCSRetryExhausted")()

	// Extract gRPC status code if available.
	errorCode := "unknown"
	if grpcStatus, ok := status.FromError(err); ok {
		errorCode = grpcStatus.Code().String()
	}

	// Check for context timeout.
	if errors.Is(err, context.DeadlineExceeded) {
		errorCode = "DEADLINE_EXCEEDED"
	}

	log.Warn(
		"Failed to read Terraform state after all retries exhausted",
		"file", tfStateFilePath,
		"bucket", bucket,
		"attempts", maxRetries+1,
		"error_code", errorCode,
		"error", err,
	)
}
