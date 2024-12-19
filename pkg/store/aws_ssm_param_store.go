package store

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
)

// SSMStore is an implementation of the Store interface for AWS SSM Parameter Store.
type SSMStore struct {
	client SSMClient
}

type SSMStoreOptions struct {
	Region string `mapstructure:"region"`
}

// Ensure SSMStore implements the store.Store interface.
var _ Store = (*SSMStore)(nil)

// SSMClient interface allows us to mock the AWS SSM client
type SSMClient interface {
	PutParameter(ctx context.Context, params *ssm.PutParameterInput, optFns ...func(*ssm.Options)) (*ssm.PutParameterOutput, error)
	GetParameter(ctx context.Context, params *ssm.GetParameterInput, optFns ...func(*ssm.Options)) (*ssm.GetParameterOutput, error)
}

// NewInMemoryStore initializes a new MemoryStore.
func NewSSMStore(options SSMStoreOptions) (Store, error) {
	ctx := context.TODO()

	// Load AWS configuration (can be customized using options)
	awsConfig, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS configuration: %w", err)
	}

	if options.Region != "" {
		awsConfig.Region = options.Region
	} else {
		return nil, fmt.Errorf("region is required in ssm store configuration")
	}

	// Create the SSM client
	client := ssm.NewFromConfig(awsConfig)
	return &SSMStore{client: client}, nil
}

// Set stores a key-value pair in AWS SSM Parameter Store.
func (s *SSMStore) Set(key string, value interface{}) error {
	ctx := context.TODO()

	// Convert value to string
	strValue, ok := value.(string)
	if !ok {
		return fmt.Errorf("value must be a string")
	}

	// Put the parameter in SSM Parameter Store
	_, err := s.client.PutParameter(ctx, &ssm.PutParameterInput{
		Name:      aws.String(key),
		Value:     aws.String(strValue),
		Type:      types.ParameterTypeString,
		Overwrite: aws.Bool(true), // Allow overwriting existing keys
	})

	if err != nil {
		return fmt.Errorf("failed to set parameter '%s': %w", key, err)
	}

	return nil
}

// Get retrieves a value by key from AWS SSM Parameter Store.
func (s *SSMStore) Get(key string) (interface{}, error) {
	ctx := context.TODO()

	// Get the parameter from SSM Parameter Store
	result, err := s.client.GetParameter(ctx, &ssm.GetParameterInput{
		Name:           aws.String(key),
		WithDecryption: aws.Bool(true), // Decrypt secure parameters if necessary
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get parameter '%s': %w", key, err)
	}

	return aws.ToString(result.Parameter.Value), nil
}
