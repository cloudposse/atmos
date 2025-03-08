package store

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
)

// Error format constants.
const (
	errWrapFormat       = "%w: %w"
	errWrapFormatWithID = "%w '%s': %w"
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

// SSMClient interface allows us to mock the AWS SSM client.
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
		return nil, fmt.Errorf(errWrapFormat, ErrLoadAWSConfig, err)
	}

	if options.Region == "" {
		return nil, ErrRegionRequired
	}

	awsConfig.Region = options.Region

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

func (s *SSMStore) getKey(stack string, component string, key string) (string, error) {
	if s.stackDelimiter == nil {
		return "", ErrStackDelimiterNotSet
	}

	return getKey(s.prefix, *s.stackDelimiter, stack, component, key, "/")
}

// Set stores a key-value pair in AWS SSM Parameter Store.
func (s *SSMStore) Set(stack string, component string, key string, value interface{}) error {
	if stack == "" {
		return ErrEmptyStack
	}
	if component == "" {
		return ErrEmptyComponent
	}
	if key == "" {
		return ErrEmptyKey
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

	// Put the parameter in SSM Parameter Store
	_, err = s.client.PutParameter(ctx, &ssm.PutParameterInput{
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

// Get retrieves a value by key from AWS SSM Parameter Store.
func (s *SSMStore) Get(stack string, component string, key string) (interface{}, error) {
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

	// Get the parameter from SSM Parameter Store
	result, err := s.client.GetParameter(ctx, &ssm.GetParameterInput{
		Name:           aws.String(paramName),
		WithDecryption: aws.Bool(true), // Decrypt secure parameters if necessary
	})
	if err != nil {
		return nil, fmt.Errorf(errWrapFormatWithID, ErrGetParameter, paramName, err)
	}

	// First try to unmarshal as JSON
	var value interface{}
	if err := json.Unmarshal([]byte(*result.Parameter.Value), &value); err == nil {
		return value, nil
	}

	// If JSON unmarshalling fails, return the raw string value
	return *result.Parameter.Value, nil
}
