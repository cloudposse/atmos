package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	secretmanagerpb "cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	"github.com/googleapis/gax-go/v2"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/cloudposse/atmos/internal/gcp"
	log "github.com/cloudposse/atmos/pkg/logger"
)

const (
	gsmOperationTimeout = 30 * time.Second
	gsmKeySeparator     = "_"
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
	replication    *secretmanagerpb.Replication
}

// GSMStoreOptions defines the configuration options for Google Secret Manager store.
type GSMStoreOptions struct {
	Prefix         *string   `mapstructure:"prefix"`
	ProjectID      string    `mapstructure:"project_id"`
	StackDelimiter *string   `mapstructure:"stack_delimiter"`
	Credentials    *string   `mapstructure:"credentials"` // Optional JSON credentials
	Locations      *[]string `mapstructure:"locations"`   // Optional replication locations
}

// Verify that GSMStore implements the Store interface.
var _ Store = (*GSMStore)(nil)

// NewGSMStore initializes a new Google Secret Manager Store.
func NewGSMStore(options GSMStoreOptions) (Store, error) {
	if options.ProjectID == "" {
		return nil, ErrProjectIDRequired
	}

	ctx := context.Background()

	// Use unified GCP authentication
	clientOpts := gcp.GetClientOptions(gcp.AuthOptions{
		Credentials: gcp.GetCredentialsFromStore(options.Credentials),
	})

	client, err := secretmanager.NewClient(ctx, clientOpts...)
	if err != nil {
		// Close the client to prevent resource leaks
		if client != nil {
			if closeErr := client.Close(); closeErr != nil {
				log.Trace("Failed to close Google Secret Manager client after creation error", "error", closeErr)
			}
		}
		return nil, fmt.Errorf(errWrapFormat, ErrCreateClient, err)
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

	store.replication = createReplicationFromLocations(options.Locations)

	return store, nil
}

func createReplicationFromLocations(locations *[]string) *secretmanagerpb.Replication {
	if locations == nil || len(*locations) == 0 {
		return &secretmanagerpb.Replication{
			Replication: &secretmanagerpb.Replication_Automatic_{
				Automatic: &secretmanagerpb.Replication_Automatic{},
			},
		}
	} else {
		replicas := make([]*secretmanagerpb.Replication_UserManaged_Replica, len(*locations))
		for i, location := range *locations {
			replicas[i] = &secretmanagerpb.Replication_UserManaged_Replica{
				Location: location,
			}
		}

		return &secretmanagerpb.Replication{
			Replication: &secretmanagerpb.Replication_UserManaged_{
				UserManaged: &secretmanagerpb.Replication_UserManaged{
					Replicas: replicas,
				},
			},
		}
	}
}

// getKey generates a key for the Google Secret Manager.
func (s *GSMStore) getKey(stack string, component string, key string) (string, error) {
	if s.stackDelimiter == nil {
		return "", ErrStackDelimiterNotSet
	}

	baseKey, err := getKey(s.prefix, *s.stackDelimiter, stack, component, key, gsmKeySeparator)
	if err != nil {
		return "", fmt.Errorf(errWrapFormat, ErrGetKey, err)
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
			Replication: s.replication,
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
				return nil, fmt.Errorf(errWrapFormatWithID, ErrResourceNotFound, fmt.Sprintf("projects/%s/secrets/%s", s.projectID, secretID), err)
			case codes.PermissionDenied:
				return nil, fmt.Errorf(errWrapFormatWithID, ErrPermissionDenied, fmt.Sprintf("project %s", s.projectID), err)
			}
		}
		return nil, fmt.Errorf(errWrapFormat, ErrCreateSecret, err)
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
				return fmt.Errorf(errWrapFormatWithID, ErrResourceNotFound, secret.GetName(), err)
			case codes.PermissionDenied:
				return fmt.Errorf(errWrapFormatWithID, ErrPermissionDenied, secret.GetName(), err)
			}
		}
		return fmt.Errorf(errWrapFormat, ErrAddSecretVersion, err)
	}
	return nil
}

// Set stores a key-value pair in Google Secret Manager.
func (s *GSMStore) Set(stack string, component string, key string, value any) error {
	if stack == "" {
		return ErrEmptyStack
	}
	if component == "" {
		return ErrEmptyComponent
	}
	if key == "" {
		return ErrEmptyKey
	}
	if value == nil {
		return fmt.Errorf("%w for key %s in stack %s component %s", ErrNilValue, key, stack, component)
	}

	ctx, cancel := context.WithTimeout(context.Background(), gsmOperationTimeout)
	defer cancel()

	// Convert value to JSON string
	jsonValue, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf(errWrapFormat, ErrSerializeJSON, err)
	}
	strValue := string(jsonValue)

	// Get the secret ID using getKey
	secretID, err := s.getKey(stack, component, key)
	if err != nil {
		return fmt.Errorf(errWrapFormat, ErrGetKey, err)
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
	if stack == "" {
		return nil, ErrEmptyStack
	}
	if component == "" {
		return nil, ErrEmptyComponent
	}
	if key == "" {
		return nil, ErrEmptyKey
	}

	ctx, cancel := context.WithTimeout(context.Background(), gsmOperationTimeout)
	defer cancel()

	// Get the secret ID using getKey
	secretID, err := s.getKey(stack, component, key)
	if err != nil {
		return nil, fmt.Errorf(errWrapFormat, ErrGetKey, err)
	}

	// Build the resource name for the latest version
	name := fmt.Sprintf("projects/%s/secrets/%s/versions/latest", s.projectID, secretID)

	// Access the secret version
	result, err := s.client.AccessSecretVersion(ctx, &secretmanagerpb.AccessSecretVersionRequest{
		Name: name,
	})
	if err != nil {
		st, ok := status.FromError(err)
		if ok {
			switch st.Code() {
			case codes.NotFound:
				return nil, fmt.Errorf(errWrapFormatWithID, ErrResourceNotFound, secretID, err)
			case codes.PermissionDenied:
				return nil, fmt.Errorf(errWrapFormatWithID, ErrPermissionDenied, fmt.Sprintf("secret %s", secretID), err)
			}
		}
		return nil, fmt.Errorf(errWrapFormat, ErrAccessSecret, err)
	}

	var unmarshalled interface{}
	// Intentionally ignoring JSON unmarshal error to handle legacy or 3rd-party secrets that might not be JSON-encoded
	if err := json.Unmarshal(result.Payload.Data, &unmarshalled); err != nil {
		// If it's not valid JSON, return the raw string value
		return string(result.Payload.Data), nil
	}
	return unmarshalled, nil
}

func (s *GSMStore) GetKey(key string) (interface{}, error) {
	if key == "" {
		return nil, ErrEmptyKey
	}

	// Use the key directly as the secret name
	secretName := key

	// If prefix is set, prepend it to the key
	if s.prefix != "" {
		secretName = s.prefix + "_" + key
	}

	// Construct the full secret name
	fullSecretName := fmt.Sprintf("projects/%s/secrets/%s/versions/latest", s.projectID, secretName)

	ctx, cancel := context.WithTimeout(context.Background(), gsmOperationTimeout)
	defer cancel()

	// Access the secret version
	resp, err := s.client.AccessSecretVersion(ctx, &secretmanagerpb.AccessSecretVersionRequest{
		Name: fullSecretName,
	})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, fmt.Errorf(errWrapFormatWithID, ErrResourceNotFound, secretName, err)
		}
		return nil, fmt.Errorf(errWrapFormat, ErrAccessSecret, err)
	}

	if resp.Payload == nil || resp.Payload.Data == nil {
		return "", nil
	}

	// Try to unmarshal as JSON first, fallback to string if it fails.
	var result interface{}
	if jsonErr := json.Unmarshal(resp.Payload.Data, &result); jsonErr != nil {
		// If JSON unmarshaling fails, return as string.
		return string(resp.Payload.Data), nil
	}
	return result, nil
}
