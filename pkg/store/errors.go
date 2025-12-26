package store

import "errors"

// Error format constants.
const (
	errFormat           = "%w: %v"
	errWrapFormat       = "%w: %s"
	errWrapFormatWithID = "%w '%s': %s"
)

// Common errors shared across store implementations.
var (
	// Common validation errors.
	ErrEmptyStack           = errors.New("stack cannot be empty")
	ErrEmptyComponent       = errors.New("component cannot be empty")
	ErrEmptyKey             = errors.New("key cannot be empty")
	ErrStackDelimiterNotSet = errors.New("stack delimiter is not set")
	ErrGetKey               = errors.New("failed to get key")

	// AWS SSM specific errors.
	ErrRegionRequired = errors.New("region is required in ssm store configuration")
	ErrLoadAWSConfig  = errors.New("failed to load AWS config")
	ErrSetParameter   = errors.New("failed to set parameter")
	ErrGetParameter   = errors.New("failed to get parameter")

	// Azure Key Vault specific errors.
	ErrVaultURLRequired = errors.New("vault_url is required in azure key vault store configuration")
	ErrCreateClient     = errors.New("failed to create client")
	ErrAccessSecret     = errors.New("failed to access secret")
	ErrResourceNotFound = errors.New("resource not found")
	ErrPermissionDenied = errors.New("permission denied")

	// Redis specific errors.
	ErrParseRedisURL   = errors.New("failed to parse redis url")
	ErrMissingRedisURL = errors.New("either url must be set in options or ATMOS_REDIS_URL environment variable must be set")
	ErrGetRedisKey     = errors.New("failed to get key from redis")

	// Artifactory specific errors.
	ErrMissingArtifactoryToken = errors.New("either access_token must be set in options or one of JFROG_ACCESS_TOKEN or ARTIFACTORY_ACCESS_TOKEN environment variables must be set")
	ErrCreateTempDir           = errors.New("failed to create temp dir")
	ErrCreateTempFile          = errors.New("failed to create temp file")
	ErrDownloadFile            = errors.New("failed to download file")
	ErrNoFilesDownloaded       = errors.New("no files downloaded")
	ErrReadFile                = errors.New("failed to read file")
	ErrUnmarshalFile           = errors.New("failed to unmarshal file")
	ErrWriteTempFile           = errors.New("failed to write to temp file")
	ErrUploadFile              = errors.New("failed to upload file")

	// Google Secret Manager specific errors.
	ErrProjectIDRequired = errors.New("project_id is required in Google Secret Manager store configuration")
	ErrValueMustBeString = errors.New("value must be a string")
	ErrCreateSecret      = errors.New("failed to create secret")
	ErrAddSecretVersion  = errors.New("failed to add secret version")

	// Registry specific errors.
	ErrParseArtifactoryOptions = errors.New("failed to parse Artifactory store options")
	ErrParseSSMOptions         = errors.New("failed to parse SSM store options")
	ErrParseRedisOptions       = errors.New("failed to parse Redis store options")
	ErrStoreTypeNotFound       = errors.New("store type not found")

	// Shared errors.
	ErrSerializeJSON = errors.New("failed to serialize value to JSON")
	ErrMarshalValue  = errors.New("failed to marshal value")
	ErrNilValue      = errors.New("cannot store nil value")
)
