package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	secretmanagerpb "cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	"github.com/googleapis/gax-go/v2"
	"google.golang.org/api/option"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	gsmOperationTimeout = 30 * time.Second
	gsmKeySeparator     = "_"
)

// Define package-level error variables.
var (
	ErrProjectIDRequired      = fmt.Errorf("project_id is required in Google Secret Manager store configuration")
	ErrStackDelimiterNotSet   = fmt.Errorf("stack delimiter is not set")
	ErrStackCannotBeEmpty     = fmt.Errorf("stack cannot be empty")
	ErrComponentCannotBeEmpty = fmt.Errorf("component cannot be empty")
	ErrKeyCannotBeEmpty       = fmt.Errorf("key cannot be empty")
	ErrValueMustBeString      = fmt.Errorf("value must be a string")
)

// GSMClient is the interface that wraps the Google Secret Manager client methods we use.
type GSMClient interface {
	CreateSecret(ctx context.Context, req *secretmanagerpb.CreateSecretRequest, opts ...gax.CallOption) (*secretmanagerpb.Secret, error)
	AddSecretVersion(ctx context.Context, req *secretmanagerpb.AddSecretVersionRequest, opts ...gax.CallOption) (*secretmanagerpb.SecretVersion, error)
	AccessSecretVersion(ctx context.Context, req *secretmanagerpb.AccessSecretVersionRequest, opts ...gax.CallOption) (*secretmanagerpb.AccessSecretVersionResponse, error)
	Close() error
}

// GSMStore is an implementation of the Store interface for Google Secret Manager.
type GSMStore struct {
	client         GSMClient
	projectID      string
	prefix         string
	stackDelimiter *string
}

// GSMStoreOptions defines the configuration options for Google Secret Manager store.
type GSMStoreOptions struct {
	Prefix         *string `mapstructure:"prefix"`
	ProjectID      string  `mapstructure:"project_id"`
	StackDelimiter *string `mapstructure:"stack_delimiter"`
	Credentials    *string `mapstructure:"credentials"` // Optional JSON credentials
}

// Verify that GSMStore implements the Store interface.
var _ Store = (*GSMStore)(nil)

// NewGSMStore initializes a new Google Secret Manager Store.
func NewGSMStore(options GSMStoreOptions) (Store, error) {
	if options.ProjectID == "" {
		return nil, ErrProjectIDRequired
	}

	ctx := context.Background()

	var clientOpts []option.ClientOption
	if options.Credentials != nil && *options.Credentials != "" {
		clientOpts = append(clientOpts, option.WithCredentialsJSON([]byte(*options.Credentials)))
	}

	client, err := secretmanager.NewClient(ctx, clientOpts...)
	if err != nil {
		// Close the client to prevent resource leaks
		if client != nil {
			client.Close()
		}
		return nil, fmt.Errorf("failed to create Secret Manager client: %w", err)
	}

	store := &GSMStore{
		client:    client,
		projectID: options.ProjectID,
	}

	if options.Prefix != nil {
		store.prefix = *options.Prefix
	}

	if options.StackDelimiter != nil {
		store.stackDelimiter = options.StackDelimiter
	} else {
		defaultDelimiter := "-"
		store.stackDelimiter = &defaultDelimiter
	}

	return store, nil
}

// getKey generates a key for the Google Secret Manager.
func (s *GSMStore) getKey(stack string, component string, key string) (string, error) {
	if s.stackDelimiter == nil {
		return "", ErrStackDelimiterNotSet
	}

	baseKey, err := getKey(s.prefix, *s.stackDelimiter, stack, component, key, gsmKeySeparator)
	if err != nil {
		return "", fmt.Errorf("error getting key: %w", err)
	}

	// Replace any remaining slashes with underscores as Secret Manager doesn't allow slashes
	baseKey = strings.ReplaceAll(baseKey, "/", gsmKeySeparator)
	// Remove any double underscores that might have been created
	baseKey = strings.ReplaceAll(baseKey, gsmKeySeparator+gsmKeySeparator, gsmKeySeparator)
	// Trim any leading or trailing underscores
	baseKey = strings.Trim(baseKey, gsmKeySeparator)

	return baseKey, nil
}

func (s *GSMStore) createSecret(ctx context.Context, secretID string) (*secretmanagerpb.Secret, error) {
	parent := fmt.Sprintf("projects/%s", s.projectID)
	createSecretReq := &secretmanagerpb.CreateSecretRequest{
		Parent:   parent,
		SecretId: secretID,
		Secret: &secretmanagerpb.Secret{
			Replication: &secretmanagerpb.Replication{
				Replication: &secretmanagerpb.Replication_Automatic_{
					Automatic: &secretmanagerpb.Replication_Automatic{},
				},
			},
		},
	}

	secret, err := s.client.CreateSecret(ctx, createSecretReq)
	if err != nil {
		if st, ok := status.FromError(err); ok {
			switch st.Code() {
			case codes.AlreadyExists:
				return &secretmanagerpb.Secret{
					Name: fmt.Sprintf("projects/%s/secrets/%s", s.projectID, secretID),
				}, nil
			case codes.NotFound:
				return nil, fmt.Errorf("projects/%s/secrets/%s not found: %w", s.projectID, secretID, err)
			case codes.PermissionDenied:
				return nil, fmt.Errorf("permission denied for project %s - please check if the project exists and you have the required permissions: %w", s.projectID, err)
			}
		}
		return nil, fmt.Errorf("failed to create secret: %w", err)
	}
	return secret, nil
}

func (s *GSMStore) addSecretVersion(ctx context.Context, secret *secretmanagerpb.Secret, value string) error {
	addVersionReq := &secretmanagerpb.AddSecretVersionRequest{
		Parent: secret.GetName(),
		Payload: &secretmanagerpb.SecretPayload{
			Data: []byte(value),
		},
	}

	_, err := s.client.AddSecretVersion(ctx, addVersionReq)
	if err != nil {
		if st, ok := status.FromError(err); ok {
			switch st.Code() {
			case codes.NotFound:
				return fmt.Errorf("resource not found %s: %w", secret.GetName(), err)
			case codes.PermissionDenied:
				return fmt.Errorf("permission denied for %s - please check if you have the required permissions: %w", secret.GetName(), err)
			}
		}
		return fmt.Errorf("failed to add secret version: %w", err)
	}
	return nil
}

// Set stores a key-value pair in Google Secret Manager.
func (s *GSMStore) Set(stack string, component string, key string, value any) error {
	ctx, cancel := context.WithTimeout(context.Background(), gsmOperationTimeout)
	defer cancel()

	if stack == "" {
		return ErrStackCannotBeEmpty
	}
	if component == "" {
		return ErrComponentCannotBeEmpty
	}
	if key == "" {
		return ErrKeyCannotBeEmpty
	}

	strValue, ok := value.(string)
	if !ok {
		return ErrValueMustBeString
	}

	secretID, err := s.getKey(stack, component, key)
	if err != nil {
		return fmt.Errorf("failed to get key: %w", err)
	}

	secret, err := s.createSecret(ctx, secretID)
	if err != nil {
		return err
	}

	if err := s.addSecretVersion(ctx, secret, strValue); err != nil {
		return err
	}

	return nil
}

// Get retrieves a value by key from Google Secret Manager.
func (s *GSMStore) Get(stack string, component string, key string) (any, error) {
	ctx, cancel := context.WithTimeout(context.Background(), gsmOperationTimeout)
	defer cancel()

	if stack == "" {
		return nil, ErrStackCannotBeEmpty
	}
	if component == "" {
		return nil, ErrComponentCannotBeEmpty
	}
	if key == "" {
		return nil, ErrKeyCannotBeEmpty
	}

	// Get the secret ID using getKey
	secretID, err := s.getKey(stack, component, key)
	if err != nil {
		return nil, fmt.Errorf("failed to get key: %w", err)
	}

	// Build the resource name for the latest version
	name := fmt.Sprintf("projects/%s/secrets/%s/versions/latest", s.projectID, secretID)

	// Access the secret version
	result, err := s.client.AccessSecretVersion(ctx, &secretmanagerpb.AccessSecretVersionRequest{
		Name: name,
	})
	if err != nil {
		if st, ok := status.FromError(err); ok {
			switch st.Code() {
			case codes.NotFound:
				return nil, fmt.Errorf("resource not found %s: %w", secretID, err)
			case codes.PermissionDenied:
				return nil, fmt.Errorf("permission denied for secret %s - please check if you have the required permissions: %w", secretID, err)
			}
		}
		return nil, fmt.Errorf("failed to access secret version: %w", err)
	}

	return string(result.Payload.Data), nil
}
