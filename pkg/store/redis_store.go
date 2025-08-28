package store

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisStore struct {
	prefix         string
	redisClient    RedisClient
	stackDelimiter *string
}

type RedisStoreOptions struct {
	Prefix         *string `mapstructure:"prefix"`
	StackDelimiter *string `mapstructure:"stack_delimiter"`
	URL            *string `mapstructure:"url"`
}

// RedisClient interface allows us to mock the Redis Client in test with only the methods we are using in the RedisStore.
type RedisClient interface {
	Get(ctx context.Context, key string) *redis.StringCmd
	Set(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd
}

// Ensure RedisStore implements the store.Store interface.
var _ Store = (*RedisStore)(nil)

func getRedisOptions(options *RedisStoreOptions) (*redis.Options, error) {
	if options.URL != nil {
		opts, err := redis.ParseURL(*options.URL)
		if err != nil {
			return &redis.Options{}, fmt.Errorf(errFormat, ErrParseRedisURL, err)
		}

		return opts, nil
	}

	if os.Getenv("ATMOS_REDIS_URL") != "" {
		return redis.ParseURL(os.Getenv("ATMOS_REDIS_URL"))
	}

	return &redis.Options{}, ErrMissingRedisURL
}

func NewRedisStore(options RedisStoreOptions) (Store, error) {
	prefix := ""
	if options.Prefix != nil {
		prefix = *options.Prefix
	}

	stackDelimiter := "/"
	if options.StackDelimiter != nil {
		stackDelimiter = *options.StackDelimiter
	}

	opts, err := getRedisOptions(&options)
	if err != nil {
		return nil, fmt.Errorf(errFormat, ErrParseRedisURL, err)
	}

	redisClient := redis.NewClient(opts)

	return &RedisStore{
		prefix:         prefix,
		redisClient:    redisClient,
		stackDelimiter: &stackDelimiter,
	}, nil
}

func (s *RedisStore) getKey(stack string, component string, key string) (string, error) {
	if s.stackDelimiter == nil {
		return "", ErrStackDelimiterNotSet
	}

	prefixParts := []string{s.prefix}
	prefix := strings.Join(prefixParts, "/")

	return getKey(prefix, *s.stackDelimiter, stack, component, key, "/")
}

func (s *RedisStore) Get(stack string, component string, key string) (interface{}, error) {
	if stack == "" {
		return nil, ErrEmptyStack
	}

	if component == "" {
		return nil, ErrEmptyComponent
	}

	if key == "" {
		return nil, ErrEmptyKey
	}

	paramName, err := s.getKey(stack, component, key)
	if err != nil {
		return nil, fmt.Errorf(errFormat, ErrGetKey, err)
	}

	ctx := context.Background()
	jsonData, err := s.redisClient.Get(ctx, paramName).Result()
	if err != nil {
		return nil, fmt.Errorf(errFormat, ErrGetRedisKey, err)
	}

	// First try to unmarshal as JSON
	var result interface{}
	if err := json.Unmarshal([]byte(jsonData), &result); err == nil {
		return result, nil
	}

	// If JSON unmarshalling fails, return the raw string value
	return jsonData, nil
}

func (s *RedisStore) Set(stack string, component string, key string, value interface{}) error {
	if stack == "" {
		return ErrEmptyStack
	}

	if component == "" {
		return ErrEmptyComponent
	}

	if key == "" {
		return ErrEmptyKey
	}

	// Construct the full parameter name using getKey
	paramName, err := s.getKey(stack, component, key)
	if err != nil {
		return fmt.Errorf(errFormat, ErrGetKey, err)
	}

	jsonData, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf(errFormat, ErrMarshalValue, err)
	}

	ctx := context.Background()
	err = s.redisClient.Set(ctx, paramName, jsonData, 0).Err()

	return err
}

func (s *RedisStore) GetKey(key string) (interface{}, error) {
	if key == "" {
		return nil, ErrEmptyKey
	}

	// Use the key directly as the Redis key
	redisKey := key

	// If prefix is set, prepend it to the key
	if s.prefix != "" {
		redisKey = s.prefix + ":" + key
	}

	// Get the value from Redis
	resp := s.redisClient.Get(context.Background(), redisKey)
	if resp.Err() != nil {
		if resp.Err().Error() == "redis: nil" {
			return nil, fmt.Errorf(errWrapFormatWithID, ErrResourceNotFound, redisKey, resp.Err())
		}
		return nil, fmt.Errorf(errFormat, ErrGetParameter, resp.Err())
	}

	value := resp.Val()
	if value == "" {
		return "", nil
	}

	// Try to unmarshal as JSON first, fallback to string if it fails.
	var result interface{}
	if unmarshalErr := json.Unmarshal([]byte(value), &result); unmarshalErr != nil {
		// If JSON unmarshaling fails, return as string.
		// Intentionally ignoring JSON unmarshal error to fall back to string
		//nolint:nilerr
		return value, nil
	}
	return result, nil
}

// RedisClient returns the underlying Redis client for testing purposes.
func (s *RedisStore) RedisClient() RedisClient {
	return s.redisClient
}
