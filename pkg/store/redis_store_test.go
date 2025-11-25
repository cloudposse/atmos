package store

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockRedisClient is a mock implementation of the RedisClient interface.
type MockRedisClient struct {
	mock.Mock
}

func (m *MockRedisClient) Get(ctx context.Context, key string) *redis.StringCmd {
	args := m.Called(ctx, key)
	cmd := redis.NewStringResult(args.String(0), args.Error(1))
	return cmd
}

func (m *MockRedisClient) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd {
	args := m.Called(ctx, key, value, expiration)
	cmd := redis.NewStatusResult(args.String(0), args.Error(1))
	return cmd
}

func ptr(s string) *string {
	return &s
}

func TestRedisStore_Get_Success(t *testing.T) {
	tests := []struct {
		name          string
		expectedValue interface{}
	}{
		{
			name: "map value",
			expectedValue: map[string]interface{}{
				"field": "value",
				"nested": map[string]interface{}{
					"inner": 42,
				},
			},
		},
		{
			name:          "slice value",
			expectedValue: []interface{}{"a", "b", "c"},
		},
		{
			name: "complex value",
			expectedValue: map[string]interface{}{
				"strings": []interface{}{"a", "b"},
				"numbers": []interface{}{1, 2, 3},
				"nested": map[string]interface{}{
					"array": []interface{}{
						map[string]interface{}{"x": 1},
						map[string]interface{}{"y": 2},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			mockClient := new(MockRedisClient)
			store, err := NewRedisStore(RedisStoreOptions{
				Prefix:         ptr("testprefix"),
				StackDelimiter: ptr("/"),
				URL:            ptr("redis://localhost:6379"),
			})
			assert.NoError(t, err)

			redisStore, ok := store.(*RedisStore)
			assert.True(t, ok)
			redisStore.redisClient = mockClient

			stack := "mystack"
			component := "mycomponent"
			key := "mykey"
			fullKey := "testprefix/mystack/mycomponent/mykey"

			jsonData, _ := json.Marshal(tt.expectedValue)

			// Set up the expected calls and return values
			mockClient.On("Get", context.Background(), fullKey).Return(string(jsonData), nil)

			// Act
			result, err := redisStore.Get(stack, component, key)

			// Assert
			assert.NoError(t, err)
			// Use JSONEq to compare the JSON representation instead of direct equality
			expectedJSON, _ := json.Marshal(tt.expectedValue)
			actualJSON, _ := json.Marshal(result)
			assert.JSONEq(t, string(expectedJSON), string(actualJSON))
			mockClient.AssertExpectations(t)
		})
	}
}

func TestRedisStore_Get_KeyNotFound(t *testing.T) {
	// Arrange
	mockClient := new(MockRedisClient)
	store, err := NewRedisStore(RedisStoreOptions{
		Prefix:         ptr("testprefix"),
		StackDelimiter: ptr("/"),
		URL:            ptr("redis://localhost:6379"),
	})
	assert.NoError(t, err)

	redisStore, ok := store.(*RedisStore)
	assert.True(t, ok)
	redisStore.redisClient = mockClient

	stack := "mystack"
	component := "mycomponent"
	key := "mykey"
	fullKey := "testprefix/mystack/mycomponent/mykey"

	// Set up the expected calls and return values
	mockClient.On("Get", context.Background(), fullKey).Return("", redis.Nil)

	// Act
	result, err := redisStore.Get(stack, component, key)

	// Assert
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to get key")
	mockClient.AssertExpectations(t)
}

func TestRedisStore_Set_Success(t *testing.T) {
	// Arrange
	mockClient := new(MockRedisClient)
	store, err := NewRedisStore(RedisStoreOptions{
		Prefix:         ptr("testprefix"),
		StackDelimiter: ptr("/"),
		URL:            ptr("redis://localhost:6379"),
	})
	assert.NoError(t, err)

	redisStore, ok := store.(*RedisStore)
	assert.True(t, ok)
	redisStore.redisClient = mockClient

	stack := "mystack"
	component := "mycomponent"
	key := "mykey"
	fullKey := "testprefix/mystack/mycomponent/mykey"

	value := map[string]interface{}{
		"field": "value",
	}

	jsonData, _ := json.Marshal(value)

	// Set up the expected calls and return values
	mockClient.On("Set", context.Background(), fullKey, jsonData, time.Duration(0)).Return("OK", nil)

	// Act
	err = redisStore.Set(stack, component, key, value)

	// Assert
	assert.NoError(t, err)
	mockClient.AssertExpectations(t)
}

func TestRedisStore_Set_MarshalError(t *testing.T) {
	// Arrange
	mockClient := new(MockRedisClient)
	store, err := NewRedisStore(RedisStoreOptions{
		Prefix:         ptr("testprefix"),
		StackDelimiter: ptr("/"),
		URL:            ptr("redis://localhost:6379"),
	})
	assert.NoError(t, err)

	redisStore, ok := store.(*RedisStore)
	assert.True(t, ok)
	redisStore.redisClient = mockClient

	stack := "mystack"
	component := "mycomponent"
	key := "mykey"

	// Create a value that cannot be marshaled to JSON (e.g., a channel)
	value := make(chan int)

	// Act
	err = redisStore.Set(stack, component, key, value)

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to marshal value")
}

func TestRedisStore_Get_UnmarshalError(t *testing.T) {
	// Arrange
	mockClient := new(MockRedisClient)
	store, err := NewRedisStore(RedisStoreOptions{
		Prefix:         ptr("testprefix"),
		StackDelimiter: ptr("/"),
		URL:            ptr("redis://localhost:6379"),
	})
	assert.NoError(t, err)

	redisStore, ok := store.(*RedisStore)
	assert.True(t, ok)
	redisStore.redisClient = mockClient

	stack := "mystack"
	component := "mycomponent"
	key := "mykey"
	fullKey := "testprefix/mystack/mycomponent/mykey"

	invalidJSON := "invalid_json"

	// Set up the expected calls and return values
	mockClient.On("Get", context.Background(), fullKey).Return(invalidJSON, nil)

	// Act
	result, err := redisStore.Get(stack, component, key)

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, invalidJSON, result)
	mockClient.AssertExpectations(t)
}

func TestRedisStore_Get_NonJsonValues(t *testing.T) {
	tests := []struct {
		name          string
		rawValue      string
		expectedValue interface{}
	}{
		{
			name:          "plain text",
			rawValue:      "plain text value",
			expectedValue: "plain text value",
		},
		{
			name:          "malformed json",
			rawValue:      `{"key1":"value1", "key2":}`,
			expectedValue: `{"key1":"value1", "key2":}`,
		},
		{
			name:          "integer value",
			rawValue:      `42`,
			expectedValue: float64(42), // JSON unmarshals numbers as float64
		},
		{
			name:          "float value",
			rawValue:      `3.14159`,
			expectedValue: 3.14159,
		},
		{
			name:          "numeric string",
			rawValue:      `"42"`, // JSON string containing a number
			expectedValue: "42",   // Should be parsed as a string, not a number
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			mockClient := new(MockRedisClient)
			store, err := NewRedisStore(RedisStoreOptions{
				Prefix:         ptr("testprefix"),
				StackDelimiter: ptr("/"),
				URL:            ptr("redis://localhost:6379"),
			})
			assert.NoError(t, err)

			redisStore, ok := store.(*RedisStore)
			assert.True(t, ok)
			redisStore.redisClient = mockClient

			stack := "mystack"
			component := "mycomponent"
			key := "mykey"
			fullKey := "testprefix/mystack/mycomponent/mykey"

			// Set up the expected calls and return values
			mockClient.On("Get", context.Background(), fullKey).Return(tt.rawValue, nil)

			// Act
			result, err := redisStore.Get(stack, component, key)

			// Assert
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedValue, result)
			mockClient.AssertExpectations(t)
		})
	}
}

func TestRedisStore_Get_GetKeyError(t *testing.T) {
	// Arrange
	mockClient := new(MockRedisClient)
	store, err := NewRedisStore(RedisStoreOptions{
		Prefix:         nil, // Prefix is nil
		StackDelimiter: ptr("/"),
		URL:            ptr("redis://localhost:6379"),
	})
	assert.NoError(t, err)

	redisStore, ok := store.(*RedisStore)
	assert.True(t, ok)
	redisStore.redisClient = mockClient
	// repoName is not set, so prefixParts = ["", ""] and prefix = "/"
	// Hence, the full key becomes "/mystack/mycomponent/mykey"

	stack := "mystack"
	component := "mycomponent"
	key := "mykey"

	// Expected full key based on getKey implementation
	fullKey := "/mystack/mycomponent/mykey"

	// Set up the expected call to redisClient.Get with fullKey and return redis.Nil to simulate key not found
	mockClient.On("Get", context.Background(), fullKey).Return("", redis.Nil)

	// Act
	result, err := redisStore.Get(stack, component, key)

	// Assert
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to get key")
	mockClient.AssertExpectations(t)
}

func TestRedisStore_GetKey(t *testing.T) {
	tests := []struct {
		name          string
		key           string
		mockReturn    string
		mockError     error
		expectedValue interface{}
		expectError   bool
		errorContains string
	}{
		{
			name:          "successful string retrieval",
			key:           "config/app-settings",
			mockReturn:    "production",
			mockError:     nil,
			expectedValue: "production",
			expectError:   false,
		},
		{
			name:          "successful JSON object retrieval",
			key:           "config/database",
			mockReturn:    `{"host":"localhost","port":5432}`,
			mockError:     nil,
			expectedValue: map[string]interface{}{"host": "localhost", "port": float64(5432)},
			expectError:   false,
		},
		{
			name:          "successful JSON array retrieval",
			key:           "config/servers",
			mockReturn:    `["server1","server2","server3"]`,
			mockError:     nil,
			expectedValue: []interface{}{"server1", "server2", "server3"},
			expectError:   false,
		},
		{
			name:          "key not found",
			key:           "nonexistent",
			mockReturn:    "",
			mockError:     redis.Nil,
			expectedValue: nil,
			expectError:   true,
			errorContains: "resource not found",
		},
		{
			name:          "empty key returns empty string",
			key:           "empty-key",
			mockReturn:    "",
			mockError:     nil,
			expectedValue: "",
			expectError:   false,
		},
		{
			name:          "malformed JSON returns as string",
			key:           "config/invalid",
			mockReturn:    `{"invalid": json`,
			mockError:     nil,
			expectedValue: `{"invalid": json`,
			expectError:   false,
		},
		{
			name:          "redis connection error",
			key:           "config/test",
			mockReturn:    "",
			mockError:     fmt.Errorf("connection refused"),
			expectedValue: nil,
			expectError:   true,
			errorContains: "connection refused",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			mockClient := new(MockRedisClient)
			store, err := NewRedisStore(RedisStoreOptions{
				Prefix:         ptr("myapp"),
				StackDelimiter: ptr("/"),
				URL:            ptr("redis://localhost:6379"),
			})
			assert.NoError(t, err)

			redisStore, ok := store.(*RedisStore)
			assert.True(t, ok)
			redisStore.redisClient = mockClient

			// Set up mock expectations
			// GetKey uses the key exactly as provided without adding prefix
			mockClient.On("Get", context.Background(), tt.key).Return(tt.mockReturn, tt.mockError)

			// Act
			result, err := redisStore.GetKey(tt.key)

			// Assert
			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
				assert.Equal(t, tt.expectedValue, result)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedValue, result)
			}
			mockClient.AssertExpectations(t)
		})
	}
}
