package store

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
)

// SSMStore is an implementation of the Store interface for AWS SSM Parameter Store.
type SSMStore struct {
	client         SSMClient
	prefix         string
	stackDelimiter *string
}

type SSMStoreOptions struct {
	Prefix         *string `mapstructure:"prefix"`
	Region         string  `mapstructure:"region"`
	StackDelimiter *string `mapstructure:"stack_delimiter"`
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
	store := &SSMStore{client: client}

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

	return store, nil
}

func (s *SSMStore) getKey(stack string, component string, key string) string {
	stackParts := strings.Split(stack, *s.stackDelimiter)
	componentParts := strings.Split(component, "/")

	parts := append([]string{s.prefix}, stackParts...)
	parts = append(parts, componentParts...)
	parts = append(parts, key)
	return strings.Join(parts, "/")
}

// Set stores a key-value pair in AWS SSM Parameter Store.
func (s *SSMStore) Set(stack string, component string, key string, value interface{}) error {
	if stack == "" {
		return fmt.Errorf("stack cannot be empty")
	}
	if component == "" {
		return fmt.Errorf("component cannot be empty")
	}
	if key == "" {
		return fmt.Errorf("key cannot be empty")
	}

	ctx := context.TODO()

	// Convert value to string
	strValue, ok := value.(string)
	if !ok {
		return fmt.Errorf("value must be a string")
	}

	// Construct the full parameter name using getKey
	paramName := s.getKey(stack, component, key)

	// Put the parameter in SSM Parameter Store
	_, err := s.client.PutParameter(ctx, &ssm.PutParameterInput{
		Name:      aws.String(paramName),
		Value:     aws.String(strValue),
		Type:      types.ParameterTypeString,
		Overwrite: aws.Bool(true), // Allow overwriting existing keys
	})

	if err != nil {
		return fmt.Errorf("failed to set parameter '%s': %w", paramName, err)
	}

	return nil
}

// Get retrieves a value by key from AWS SSM Parameter Store.
func (s *SSMStore) Get(stack string, component string, key string) (interface{}, error) {
	if stack == "" {
		return nil, fmt.Errorf("stack cannot be empty")
	}
	if component == "" {
		return nil, fmt.Errorf("component cannot be empty")
	}
	if key == "" {
		return nil, fmt.Errorf("key cannot be empty")
	}

	ctx := context.TODO()

	// Construct the full parameter name using getKey
	paramName := s.getKey(stack, component, key)

	// Get the parameter from SSM Parameter Store
	result, err := s.client.GetParameter(ctx, &ssm.GetParameterInput{
		Name:           aws.String(paramName),
		WithDecryption: aws.Bool(true), // Decrypt secure parameters if necessary
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get parameter '%s': %w", paramName, err)
	}

	return aws.ToString(result.Parameter.Value), nil
}
