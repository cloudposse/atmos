package provisioner

import "errors"

// Error types for provisioner operations.
var (
	ErrBucketRequired      = errors.New("backend.bucket is required")
	ErrRegionRequired      = errors.New("backend.region is required")
	ErrBackendNotFound     = errors.New("backend configuration not found")
	ErrBackendTypeRequired = errors.New("backend_type not specified")
	ErrNoProvisionerFound  = errors.New("no provisioner registered for backend type")
	ErrProvisionerFailed   = errors.New("provisioner failed")
	ErrLoadAWSConfig       = errors.New("failed to load AWS config")
	ErrCheckBucketExist    = errors.New("failed to check bucket existence")
	ErrCreateBucket        = errors.New("failed to create bucket")
	ErrApplyBucketDefaults = errors.New("failed to apply bucket defaults")
	ErrEnableVersioning    = errors.New("failed to enable versioning")
	ErrEnableEncryption    = errors.New("failed to enable encryption")
	ErrBlockPublicAccess   = errors.New("failed to block public access")
	ErrApplyTags           = errors.New("failed to apply tags")
)
