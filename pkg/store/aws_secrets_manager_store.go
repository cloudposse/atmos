package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	smtypes "github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
)

// SecretsManagerStore is an implementation of the Store interface for AWS Secrets Manager.
// Unlike SSM Parameter Store, Secrets Manager is encrypted at rest by default and is suited
// to structured/JSON secrets, rotation, and larger values.
type SecretsManagerStore struct {
	client         SecretsManagerClient
	prefix         string
	stackDelimiter *string
	region         string
	endpoint       string

	// Identity-based authentication fields.
	identityName string
	authResolver AuthContextResolver
	initOnce     sync.Once
	initErr      error
}

// SecretsManagerStoreOptions configures an AWS Secrets Manager store.
type SecretsManagerStoreOptions struct {
	Prefix         *string `mapstructure:"prefix"`
	Region         string  `mapstructure:"region"`
	StackDelimiter *string `mapstructure:"stack_delimiter"`
	Endpoint       *string `mapstructure:"endpoint"`
	EndpointURL    *string `mapstructure:"endpoint_url"`
}

// SecretsManagerClient is the subset of the AWS Secrets Manager API used by the store.
type SecretsManagerClient interface {
	CreateSecret(ctx context.Context, params *secretsmanager.CreateSecretInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.CreateSecretOutput, error)
	PutSecretValue(ctx context.Context, params *secretsmanager.PutSecretValueInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.PutSecretValueOutput, error)
	GetSecretValue(ctx context.Context, params *secretsmanager.GetSecretValueInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error)
	DeleteSecret(ctx context.Context, params *secretsmanager.DeleteSecretInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.DeleteSecretOutput, error)
}

// Ensure SecretsManagerStore implements the expected interfaces.
var (
	_ Store              = (*SecretsManagerStore)(nil)
	_ IdentityAwareStore = (*SecretsManagerStore)(nil)
	_ DeletableStore     = (*SecretsManagerStore)(nil)
	_ StatusStore        = (*SecretsManagerStore)(nil)
)

// NewSecretsManagerStore initializes a new SecretsManagerStore.
// Client initialization is deferred until first use so callers can inject an auth resolver
// after config load and before the first backend operation.
func NewSecretsManagerStore(options SecretsManagerStoreOptions, identityName string) (Store, error) {
	if options.Region == "" {
		return nil, ErrRegionRequired
	}

	s := &SecretsManagerStore{
		region:       options.Region,
		endpoint:     firstNonEmptyStringPtr(options.Endpoint, options.EndpointURL),
		identityName: identityName,
	}

	if options.Prefix != nil {
		s.prefix = *options.Prefix
	}

	if options.StackDelimiter != nil {
		s.stackDelimiter = options.StackDelimiter
	} else {
		s.stackDelimiter = aws.String("-")
	}

	return s, nil
}

// SetAuthContext implements IdentityAwareStore.
func (s *SecretsManagerStore) SetAuthContext(resolver AuthContextResolver, identityName string) {
	s.authResolver = resolver
	if identityName != "" && s.identityName != identityName {
		s.identityName = identityName
		s.client = nil
		s.initOnce = sync.Once{}
		s.initErr = nil
	}
}

func (s *SecretsManagerStore) initDefaultClient() error {
	ctx := context.TODO()
	cfgOpts := []func(*config.LoadOptions) error{config.WithRegion(s.region)}
	if s.endpoint != "" {
		cfgOpts = append(cfgOpts, config.WithBaseEndpoint(s.endpoint))
	}
	awsConfig, err := config.LoadDefaultConfig(ctx, cfgOpts...)
	if err != nil {
		return fmt.Errorf(errWrapFormat, ErrLoadAWSConfig, err)
	}
	s.client = secretsmanager.NewFromConfig(awsConfig)
	return nil
}

func (s *SecretsManagerStore) initIdentityClient() error {
	if s.authResolver == nil {
		return fmt.Errorf("%w: store requires identity %q but no auth resolver was injected", ErrIdentityNotConfigured, s.identityName)
	}

	ctx := context.TODO()
	authContext, err := s.authResolver.ResolveAWSAuthContext(ctx, s.identityName)
	if err != nil {
		return fmt.Errorf("%w: failed to resolve AWS auth context for identity %q: %w", ErrAuthContextNotAvailable, s.identityName, err)
	}

	var cfgOpts []func(*config.LoadOptions) error
	if authContext.CredentialsFile != "" {
		cfgOpts = append(cfgOpts, config.WithSharedCredentialsFiles([]string{authContext.CredentialsFile}))
	}
	if authContext.ConfigFile != "" {
		cfgOpts = append(cfgOpts, config.WithSharedConfigFiles([]string{authContext.ConfigFile}))
	}
	if authContext.Profile != "" {
		cfgOpts = append(cfgOpts, config.WithSharedConfigProfile(authContext.Profile))
	}
	region := s.region
	if region == "" && authContext.Region != "" {
		region = authContext.Region
	}
	if region != "" {
		cfgOpts = append(cfgOpts, config.WithRegion(region))
	}
	endpoint := s.endpoint
	if endpoint == "" {
		endpoint = authContext.EndpointURL
	}
	if endpoint != "" {
		cfgOpts = append(cfgOpts, config.WithBaseEndpoint(endpoint))
	}

	awsConfig, err := config.LoadDefaultConfig(ctx, cfgOpts...)
	if err != nil {
		return fmt.Errorf(errWrapFormat, ErrLoadAWSConfig, err)
	}
	s.client = secretsmanager.NewFromConfig(awsConfig)
	return nil
}

func (s *SecretsManagerStore) ensureClient() error {
	s.initOnce.Do(func() {
		if s.client != nil {
			return
		}
		if s.identityName == "" {
			s.initErr = s.initDefaultClient()
		} else {
			s.initErr = s.initIdentityClient()
		}
	})
	return s.initErr
}

// getKey builds the Secrets Manager secret id. Secrets Manager names allow "/" so we reuse the
// store namespacing with "/" as the final delimiter.
func (s *SecretsManagerStore) getKey(stack string, component string, key string) (string, error) {
	if s.stackDelimiter == nil {
		return "", ErrStackDelimiterNotSet
	}
	return getKey(s.prefix, *s.stackDelimiter, stack, component, key, "/")
}

// Set stores a value, creating the secret if it does not yet exist. An empty stack and/or
// component is permitted: scoped secret coordinates (stack/global scope) omit those path segments.
func (s *SecretsManagerStore) Set(stack string, component string, key string, value any) error {
	if key == "" {
		return ErrEmptyKey
	}
	if value == nil {
		return fmt.Errorf("%w for key %s in stack %s component %s", ErrNilValue, key, stack, component)
	}

	if err := s.ensureClient(); err != nil {
		return err
	}

	ctx := context.TODO()

	jsonValue, err := marshalSecretsManagerValue(value)
	if err != nil {
		return fmt.Errorf(errWrapFormat, ErrSerializeJSON, err)
	}
	strValue := string(jsonValue)

	secretID, err := s.getKey(stack, component, key)
	if err != nil {
		return fmt.Errorf(errWrapFormat, ErrGetKey, err)
	}

	return s.putOrCreate(ctx, secretID, strValue)
}

func marshalSecretsManagerValue(value any) ([]byte, error) {
	if str, ok := value.(string); ok {
		trimmed := strings.TrimSpace(str)
		if len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[') && json.Valid([]byte(trimmed)) {
			return []byte(trimmed), nil
		}
	}
	return json.Marshal(value)
}

// putOrCreate updates an existing secret value, creating the secret if it does not yet exist.
func (s *SecretsManagerStore) putOrCreate(ctx context.Context, secretID, strValue string) error {
	_, err := s.client.PutSecretValue(ctx, &secretsmanager.PutSecretValueInput{
		SecretId:     aws.String(secretID),
		SecretString: aws.String(strValue),
	})
	if err == nil {
		return nil
	}

	var notFound *smtypes.ResourceNotFoundException
	if !errors.As(err, &notFound) {
		return fmt.Errorf(errWrapFormatWithID, ErrSetSecret, secretID, err)
	}

	if _, err = s.client.CreateSecret(ctx, &secretsmanager.CreateSecretInput{
		Name:         aws.String(secretID),
		SecretString: aws.String(strValue),
	}); err != nil {
		return fmt.Errorf(errWrapFormatWithID, ErrSetSecret, secretID, err)
	}

	return nil
}

// Get retrieves a value for an Atmos component in a stack. An empty stack and/or component is
// permitted: scoped secret coordinates (stack/global scope) omit those path segments.
func (s *SecretsManagerStore) Get(stack string, component string, key string) (any, error) {
	if key == "" {
		return nil, ErrEmptyKey
	}

	if err := s.ensureClient(); err != nil {
		return nil, err
	}

	secretID, err := s.getKey(stack, component, key)
	if err != nil {
		return nil, fmt.Errorf(errWrapFormat, ErrGetKey, err)
	}

	return s.getByID(secretID)
}

// GetKey retrieves a value by its raw secret id (optionally prefixed).
func (s *SecretsManagerStore) GetKey(key string) (any, error) {
	if key == "" {
		return nil, ErrEmptyKey
	}
	if err := s.ensureClient(); err != nil {
		return nil, err
	}

	secretID := key
	if s.prefix != "" {
		secretID = s.prefix + "/" + key
	}
	return s.getByID(secretID)
}

func (s *SecretsManagerStore) getByID(secretID string) (any, error) {
	ctx := context.TODO()
	output, err := s.client.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(secretID),
	})
	if err != nil {
		// Use %w for the underlying error so callers (e.g. Has) can detect ResourceNotFound.
		return nil, fmt.Errorf("%w '%s': %w", ErrGetSecret, secretID, err)
	}
	if output.SecretString == nil {
		return nil, fmt.Errorf("%w '%s': empty secret string", ErrGetSecret, secretID)
	}

	var result any
	//nolint:nilerr // Non-JSON secrets are returned as the raw string.
	if err := json.Unmarshal([]byte(*output.SecretString), &result); err != nil {
		return *output.SecretString, nil
	}
	return result, nil
}

// Delete removes a secret (with no recovery window so the name can be reused immediately).
// An empty stack and/or component is permitted: scoped secret coordinates (stack/global scope)
// omit those path segments.
func (s *SecretsManagerStore) Delete(stack string, component string, key string) error {
	if key == "" {
		return ErrEmptyKey
	}

	if err := s.ensureClient(); err != nil {
		return err
	}

	secretID, err := s.getKey(stack, component, key)
	if err != nil {
		return fmt.Errorf(errWrapFormat, ErrGetKey, err)
	}

	ctx := context.TODO()
	_, err = s.client.DeleteSecret(ctx, &secretsmanager.DeleteSecretInput{
		SecretId:                   aws.String(secretID),
		ForceDeleteWithoutRecovery: aws.Bool(true),
	})
	if err != nil {
		return fmt.Errorf(errWrapFormatWithID, ErrDeleteSecret, secretID, err)
	}
	return nil
}

// Has reports whether a secret exists, treating ResourceNotFound as non-existent.
func (s *SecretsManagerStore) Has(stack string, component string, key string) (bool, error) {
	_, err := s.Get(stack, component, key)
	if err != nil {
		var notFound *smtypes.ResourceNotFoundException
		if errors.As(err, &notFound) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
