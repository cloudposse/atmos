package aws

import (
	"context"
	"fmt"

	"github.com/cloudposse/atmos/internal/auth"
	"github.com/cloudposse/atmos/pkg/schema"
)

// userIdentity implements AWS user identity (passthrough)
type userIdentity struct {
	name   string
	config *schema.Identity
}

// NewUserIdentity creates a new AWS user identity
func NewUserIdentity(name string, config *schema.Identity) (auth.Identity, error) {
	if config.Kind != "aws/user" {
		return nil, fmt.Errorf("invalid identity kind for user: %s", config.Kind)
	}

	return &userIdentity{
		name:   name,
		config: config,
	}, nil
}

// Kind returns the identity kind
func (i *userIdentity) Kind() string {
	return "aws/user"
}

// Authenticate performs authentication (passthrough for user credentials)
func (i *userIdentity) Authenticate(ctx context.Context, baseCreds *schema.Credentials) (*schema.Credentials, error) {
	if baseCreds == nil || baseCreds.AWS == nil {
		return nil, fmt.Errorf("base AWS credentials are required")
	}

	// For user identities, we typically just pass through the credentials
	// but we can apply any environment variable overrides
	return baseCreds, nil
}

// Validate validates the identity configuration
func (i *userIdentity) Validate() error {
	// User identities don't require additional validation beyond the provider
	return nil
}

// Environment returns environment variables for this identity
func (i *userIdentity) Environment() (map[string]string, error) {
	env := make(map[string]string)
	
	// Add environment variables from identity config
	for _, envVar := range i.config.Environment {
		env[envVar.Key] = envVar.Value
	}

	return env, nil
}

// Merge merges this identity configuration with component-level overrides
func (i *userIdentity) Merge(component *schema.Identity) auth.Identity {
	merged := &userIdentity{
		name: i.name,
		config: &schema.Identity{
			Kind:        i.config.Kind,
			Default:     component.Default, // Component can override default
			Via:         i.config.Via,
			Spec:        make(map[string]interface{}),
			Alias:       i.config.Alias,
			Environment: i.config.Environment,
		},
	}

	// Merge spec
	for k, v := range i.config.Spec {
		merged.config.Spec[k] = v
	}
	for k, v := range component.Spec {
		merged.config.Spec[k] = v // Component overrides
	}

	// Merge environment variables
	merged.config.Environment = append(merged.config.Environment, component.Environment...)

	// Override alias if provided
	if component.Alias != "" {
		merged.config.Alias = component.Alias
	}

	return merged
}
