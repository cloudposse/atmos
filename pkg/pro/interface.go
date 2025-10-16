package pro

import (
	"github.com/cloudposse/atmos/pkg/pro/dtos"
	"github.com/cloudposse/atmos/pkg/schema"
)

// APIClient defines operations for interacting with Atmos Pro API.
// This interface allows mocking of API operations in tests.
//
//go:generate mockgen -source=$GOFILE -destination=mock_$GOFILE -package=$GOPACKAGE
type APIClient interface {
	// UploadInstances uploads component instances to Atmos Pro.
	UploadInstances(req *dtos.InstancesUploadRequest) error
}

// ClientFactory creates an APIClient from configuration.
type ClientFactory interface {
	// NewClient creates a new API client from the given configuration.
	NewClient(atmosConfig *schema.AtmosConfiguration) (APIClient, error)
}

// DefaultClientFactory implements ClientFactory using real API client creation.
type DefaultClientFactory struct{}

// NewClient creates a new API client from environment variables.
func (d *DefaultClientFactory) NewClient(atmosConfig *schema.AtmosConfiguration) (APIClient, error) {
	return NewAtmosProAPIClientFromEnv(atmosConfig)
}
