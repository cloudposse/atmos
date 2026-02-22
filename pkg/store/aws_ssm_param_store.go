package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

// SSMStore is an implementation of the Store interface for AWS SSM Parameter Store.
type SSMStore struct {
	client         SSMClient
	prefix         string
	stackDelimiter *string
	awsConfig      *aws.Config
	readRoleArn    *string
	writeRoleArn   *string
	newSTSClient   func(cfg aws.Config) STSClient
	newSSMClient   func(cfg aws.Config) SSMClient

	// Identity-based authentication fields.
	identityName string
	authResolver AuthContextResolver
	region       string
	initOnce     sync.Once
	initErr      error
}

type SSMStoreOptions struct {
	Prefix         *string `mapstructure:"prefix"`
	Region         string  `mapstructure:"region"`
	StackDelimiter *string `mapstructure:"stack_delimiter"`
	ReadRoleArn    *string `mapstructure:"read_role_arn"`
	WriteRoleArn   *string `mapstructure:"write_role_arn"`
}

// Ensure SSMStore implements the store.Store and IdentityAwareStore interfaces.
var (
	_ Store              = (*SSMStore)(nil)
	_ IdentityAwareStore = (*SSMStore)(nil)
)

// SSMClient interface allows us to mock the AWS SSM client.
type SSMClient interface {
	PutParameter(ctx context.Context, params *ssm.PutParameterInput, optFns ...func(*ssm.Options)) (*ssm.PutParameterOutput, error)
	GetParameter(ctx context.Context, params *ssm.GetParameterInput, optFns ...func(*ssm.Options)) (*ssm.GetParameterOutput, error)
}

// STSClient interface allows us to mock the AWS STS client.
type STSClient interface {
	AssumeRole(ctx context.Context, params *sts.AssumeRoleInput, optFns ...func(*sts.Options)) (*sts.AssumeRoleOutput, error)
}

// NewSSMStore initializes a new SSMStore.
// If identityName is non-empty, client initialization is deferred until first use (lazy init).
func NewSSMStore(options SSMStoreOptions, identityName string) (Store, error) {
	if options.Region == "" {
		return nil, ErrRegionRequired
	}

	store := &SSMStore{
		region:       options.Region,
		identityName: identityName,
		newSTSClient: func(cfg aws.Config) STSClient {
			return sts.NewFromConfig(cfg)
		},
		newSSMClient: func(cfg aws.Config) SSMClient {
			return ssm.NewFromConfig(cfg)
		},
	}

	if options.Prefix != nil {
		store.prefix = *options.Prefix
	}

	if options.StackDelimiter != nil {
		store.stackDelimiter = options.StackDelimiter
	} else {
		store.stackDelimiter = aws.String("-")
	}

	if options.ReadRoleArn != nil {
		store.readRoleArn = options.ReadRoleArn
	}
	if options.WriteRoleArn != nil {
		store.writeRoleArn = options.WriteRoleArn
	}

	// If no identity is configured, initialize the client eagerly (backward compatible behavior).
	if identityName == "" {
		if err := store.initDefaultClient(); err != nil {
			return nil, err
		}
	}

	return store, nil
}

// SetAuthContext implements IdentityAwareStore.
// If identityName is non-empty, it overrides the store's identity. Otherwise, the existing identity is preserved.
func (s *SSMStore) SetAuthContext(resolver AuthContextResolver, identityName string) {
	s.authResolver = resolver
	if identityName != "" {
		s.identityName = identityName
	}
}

// initDefaultClient initializes the AWS client using the default credential chain.
func (s *SSMStore) initDefaultClient() error {
	ctx := context.TODO()
	awsConfig, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return fmt.Errorf(errWrapFormat, ErrLoadAWSConfig, err)
	}

	awsConfig.Region = s.region
	s.awsConfig = &awsConfig
	s.client = ssm.NewFromConfig(awsConfig)

	return nil
}

// initIdentityClient initializes the AWS client using identity-based credentials.
func (s *SSMStore) initIdentityClient() error {
	if s.authResolver == nil {
		return fmt.Errorf("%w: store requires identity %q but no auth resolver was injected", ErrIdentityNotConfigured, s.identityName)
	}

	ctx := context.TODO()
	authContext, err := s.authResolver.ResolveAWSAuthContext(ctx, s.identityName)
	if err != nil {
		return fmt.Errorf("%w: failed to resolve AWS auth context for identity %q: %w", ErrAuthContextNotAvailable, s.identityName, err)
	}

	// Build AWS config options from the auth context credentials.
	cfgOpts := s.buildAuthConfigOpts(authContext)

	awsConfig, err := config.LoadDefaultConfig(ctx, cfgOpts...)
	if err != nil {
		return fmt.Errorf(errWrapFormat, ErrLoadAWSConfig, err)
	}

	s.awsConfig = &awsConfig
	s.client = ssm.NewFromConfig(awsConfig)

	return nil
}

// buildAuthConfigOpts constructs AWS SDK config options from the auth context.
func (s *SSMStore) buildAuthConfigOpts(authContext *AWSAuthConfig) []func(*config.LoadOptions) error {
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

	// Use region from auth context if the store doesn't specify one.
	region := s.region
	if region == "" && authContext.Region != "" {
		region = authContext.Region
	}
	if region != "" {
		cfgOpts = append(cfgOpts, config.WithRegion(region))
	}

	return cfgOpts
}

// ensureClient lazily initializes the AWS client if it hasn't been initialized yet.
// For identity-based stores, this resolves credentials via the auth resolver.
func (s *SSMStore) ensureClient() error {
	// If client is already initialized (eager init path), nothing to do.
	if s.client != nil {
		return nil
	}

	s.initOnce.Do(func() {
		if s.identityName == "" {
			s.initErr = s.initDefaultClient()
		} else {
			s.initErr = s.initIdentityClient()
		}
	})

	return s.initErr
}

func (s *SSMStore) getKey(stack string, component string, key string) (string, error) {
	if s.stackDelimiter == nil {
		return "", ErrStackDelimiterNotSet
	}

	return getKey(s.prefix, *s.stackDelimiter, stack, component, key, "/")
}

// assumeRole assumes the specified IAM role and returns a new AWS config.
func (s *SSMStore) assumeRole(ctx context.Context, roleArn *string) (*aws.Config, error) {
	if roleArn == nil {
		return s.awsConfig, nil
	}

	var stsClient STSClient
	if s.newSTSClient != nil {
		stsClient = s.newSTSClient(*s.awsConfig)
	} else {
		stsClient = sts.NewFromConfig(*s.awsConfig)
	}

	result, err := stsClient.AssumeRole(ctx, &sts.AssumeRoleInput{
		RoleArn:         roleArn,
		RoleSessionName: aws.String("atmos-ssm-session"),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to assume role %s: %w", *roleArn, err)
	}

	cfg := s.awsConfig.Copy()
	cfg.Credentials = credentials.NewStaticCredentialsProvider(
		*result.Credentials.AccessKeyId,
		*result.Credentials.SecretAccessKey,
		*result.Credentials.SessionToken,
	)
	return &cfg, nil
}

// Set stores a key-value pair in AWS SSM Parameter Store.
func (s *SSMStore) Set(stack string, component string, key string, value any) error {
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

	if err := s.ensureClient(); err != nil {
		return err
	}

	ctx := context.TODO()

	// Convert value to JSON string
	jsonValue, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf(errWrapFormat, ErrSerializeJSON, err)
	}
	strValue := string(jsonValue)

	// Construct the full parameter name using getKey
	paramName, err := s.getKey(stack, component, key)
	if err != nil {
		return fmt.Errorf(errWrapFormat, ErrGetKey, err)
	}

	// Assume write role if specified
	cfg, err := s.assumeRole(ctx, s.writeRoleArn)
	if err != nil {
		return fmt.Errorf("failed to assume write role: %w", err)
	}

	// Use the same client if no role was assumed
	client := s.client
	if s.writeRoleArn != nil {
		// Create SSM client with assumed role if applicable
		if s.newSSMClient != nil {
			client = s.newSSMClient(*cfg)
		} else {
			client = ssm.NewFromConfig(*cfg)
		}
	}

	// Put the parameter in SSM Parameter Store
	_, err = client.PutParameter(ctx, &ssm.PutParameterInput{
		Name:      aws.String(paramName),
		Value:     aws.String(strValue),
		Type:      types.ParameterTypeString,
		Overwrite: aws.Bool(true), // Allow overwriting existing keys
	})
	if err != nil {
		return fmt.Errorf(errWrapFormatWithID, ErrSetParameter, paramName, err)
	}

	return nil
}

// Get retrieves a value by key for an Atmos component in a stack from AWS SSM Parameter Store.
func (s *SSMStore) Get(stack string, component string, key string) (any, error) {
	if stack == "" {
		return nil, ErrEmptyStack
	}
	if component == "" {
		return nil, ErrEmptyComponent
	}
	if key == "" {
		return nil, ErrEmptyKey
	}

	if err := s.ensureClient(); err != nil {
		return nil, err
	}

	ctx := context.TODO()

	// Construct the full parameter name using getKey
	paramName, err := s.getKey(stack, component, key)
	if err != nil {
		return nil, fmt.Errorf(errWrapFormat, ErrGetKey, err)
	}

	// Assume the read role if specified
	cfg, err := s.assumeRole(ctx, s.readRoleArn)
	if err != nil {
		return nil, fmt.Errorf("failed to assume read role: %w", err)
	}

	// Use the same client if no role was assumed
	client := s.client
	if s.readRoleArn != nil {
		// Create SSM client with the assumed role if applicable
		if s.newSSMClient != nil {
			client = s.newSSMClient(*cfg)
		} else {
			client = ssm.NewFromConfig(*cfg)
		}
	}

	// Get the parameter from SSM Parameter Store
	output, err := client.GetParameter(ctx, &ssm.GetParameterInput{
		Name: aws.String(paramName),
	})
	if err != nil {
		return nil, fmt.Errorf(errWrapFormatWithID, ErrGetParameter, paramName, err)
	}

	// Try to unmarshal the value as JSON
	var result any
	//nolint:nilerr // Intentionally ignoring JSON unmarshal error to handle legacy or 3rd-party parameters that might not be JSON-encoded
	if err := json.Unmarshal([]byte(*output.Parameter.Value), &result); err != nil {
		// If it's not valid JSON, return the raw string value
		return *output.Parameter.Value, nil
	}

	return result, nil
}

// GetKey retrieves a value by key from AWS SSM Parameter Store.
func (s *SSMStore) GetKey(key string) (any, error) {
	if key == "" {
		return nil, ErrEmptyKey
	}

	if err := s.ensureClient(); err != nil {
		return nil, err
	}

	ctx := context.TODO()

	// Use the key directly as the parameter name
	paramName := key

	// If the prefix is set, prepend it to the key
	if s.prefix != "" {
		paramName = s.prefix + "/" + key
	}

	// Ensure the parameter name starts with "/" for SSM
	if !strings.HasPrefix(paramName, "/") {
		paramName = "/" + paramName
	}

	// Assume the read role if specified
	cfg, err := s.assumeRole(ctx, s.readRoleArn)
	if err != nil {
		return nil, fmt.Errorf("failed to assume read role: %w", err)
	}

	// Use the same client if no role was assumed
	client := s.client
	if s.readRoleArn != nil {
		// Create an SSM client with the assumed role if applicable
		if s.newSSMClient != nil {
			client = s.newSSMClient(*cfg)
		} else {
			client = ssm.NewFromConfig(*cfg)
		}
	}

	// Get the parameter from SSM Parameter Store
	output, err := client.GetParameter(ctx, &ssm.GetParameterInput{
		Name: aws.String(paramName),
	})
	if err != nil {
		return nil, fmt.Errorf(errWrapFormatWithID, ErrGetParameter, paramName, err)
	}

	// Try to unmarshal the value as JSON
	var result any
	//nolint:nilerr // Intentionally ignoring JSON unmarshal error to handle legacy or 3rd-party parameters that might not be JSON-encoded
	if err := json.Unmarshal([]byte(*output.Parameter.Value), &result); err != nil {
		// If it's not valid JSON, return the raw string value
		return *output.Parameter.Value, nil
	}

	return result, nil
}
