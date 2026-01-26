package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

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
}

type SSMStoreOptions struct {
	Prefix         *string `mapstructure:"prefix"`
	Region         string  `mapstructure:"region"`
	StackDelimiter *string `mapstructure:"stack_delimiter"`
	ReadRoleArn    *string `mapstructure:"read_role_arn"`
	WriteRoleArn   *string `mapstructure:"write_role_arn"`
}

// Ensure SSMStore implements the store.Store interface.
var _ Store = (*SSMStore)(nil)

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
func NewSSMStore(options SSMStoreOptions) (Store, error) {
	ctx := context.TODO()

	// Load AWS configuration (can be customized using options)
	awsConfig, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf(errWrapFormat, ErrLoadAWSConfig, err)
	}

	if options.Region == "" {
		return nil, ErrRegionRequired
	}

	awsConfig.Region = options.Region

	// Create the SSM client
	client := ssm.NewFromConfig(awsConfig)
	store := &SSMStore{
		client: client,
		newSTSClient: func(cfg aws.Config) STSClient {
			return sts.NewFromConfig(cfg)
		},
		newSSMClient: func(cfg aws.Config) SSMClient {
			return ssm.NewFromConfig(cfg)
		},
	}

	if options.Prefix != nil {
		store.prefix = *options.Prefix
	} else {
		store.prefix = ""
	}

	if options.StackDelimiter != nil {
		store.stackDelimiter = options.StackDelimiter
	} else {
		store.stackDelimiter = aws.String("-")
	}

	store.awsConfig = &awsConfig
	if options.ReadRoleArn != nil {
		store.readRoleArn = options.ReadRoleArn
	}
	if options.WriteRoleArn != nil {
		store.writeRoleArn = options.WriteRoleArn
	}

	return store, nil
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

	ctx := context.TODO()

	// Use the key directly as the parameter name without any prefix transformation.
	// This allows !store.get to access arbitrary keys as-is, per documentation.
	paramName := key

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
