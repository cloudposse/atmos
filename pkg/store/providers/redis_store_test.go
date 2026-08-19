package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	storepkg "github.com/cloudposse/atmos/pkg/store"
)

// stringCmd builds a *redis.StringCmd the way the real go-redis client would populate one,
// for use as a gomock .Return() value on RedisClient.Get expectations.
func stringCmd(val string, err error) *redis.StringCmd {
	return redis.NewStringResult(val, err)
}

// statusCmd builds a *redis.StatusCmd for use as a gomock .Return() value on RedisClient.Set
// expectations.
func statusCmd(val string, err error) *redis.StatusCmd {
	return redis.NewStatusResult(val, err)
}

// scanCmd builds a *redis.ScanCmd with either a populated key page/cursor or an error, for use as
// a gomock .Return() value on RedisClient.Scan expectations.
func scanCmd(ctx context.Context, keys []string, cursor uint64, err error) *redis.ScanCmd {
	cmd := redis.NewScanCmd(ctx, nil)
	if err != nil {
		cmd.SetErr(err)
		return cmd
	}
	cmd.SetVal(keys, cursor)
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
			ctrl := gomock.NewController(t)
			mockClient := NewMockRedisClient(ctrl)
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
			mockClient.EXPECT().Get(context.Background(), fullKey).Return(stringCmd(string(jsonData), nil))

			// Act
			result, err := redisStore.Get(stack, component, key)

			// Assert
			assert.NoError(t, err)
			// Use JSONEq to compare the JSON representation instead of direct equality
			expectedJSON, _ := json.Marshal(tt.expectedValue)
			actualJSON, _ := json.Marshal(result)
			assert.JSONEq(t, string(expectedJSON), string(actualJSON))
		})
	}
}

func TestRedisStore_Get_KeyNotFound(t *testing.T) {
	// Arrange
	ctrl := gomock.NewController(t)
	mockClient := NewMockRedisClient(ctrl)
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
	mockClient.EXPECT().Get(context.Background(), fullKey).Return(stringCmd("", redis.Nil))

	// Act
	result, err := redisStore.Get(stack, component, key)

	// Assert
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to get key")
}

func TestRedisStore_Set_Success(t *testing.T) {
	// Arrange
	ctrl := gomock.NewController(t)
	mockClient := NewMockRedisClient(ctrl)
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
	mockClient.EXPECT().Set(context.Background(), fullKey, jsonData, time.Duration(0)).Return(statusCmd("OK", nil))

	// Act
	err = redisStore.Set(stack, component, key, value)

	// Assert
	assert.NoError(t, err)
}

func TestRedisStore_Set_MarshalError(t *testing.T) {
	// Arrange
	ctrl := gomock.NewController(t)
	mockClient := NewMockRedisClient(ctrl)
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
	ctrl := gomock.NewController(t)
	mockClient := NewMockRedisClient(ctrl)
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
	mockClient.EXPECT().Get(context.Background(), fullKey).Return(stringCmd(invalidJSON, nil))

	// Act
	result, err := redisStore.Get(stack, component, key)

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, invalidJSON, result)
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
			ctrl := gomock.NewController(t)
			mockClient := NewMockRedisClient(ctrl)
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
			mockClient.EXPECT().Get(context.Background(), fullKey).Return(stringCmd(tt.rawValue, nil))

			// Act
			result, err := redisStore.Get(stack, component, key)

			// Assert
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedValue, result)
		})
	}
}

func TestRedisStore_Get_GetKeyError(t *testing.T) {
	// Arrange
	ctrl := gomock.NewController(t)
	mockClient := NewMockRedisClient(ctrl)
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
	mockClient.EXPECT().Get(context.Background(), fullKey).Return(stringCmd("", redis.Nil))

	// Act
	result, err := redisStore.Get(stack, component, key)

	// Assert
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to get key")
}

func TestRedisStore_getRedisOptions(t *testing.T) {
	t.Run("invalid url returns parse error", func(t *testing.T) {
		_, err := getRedisOptions(&RedisStoreOptions{URL: ptr("not-a-valid-redis-url")})
		assert.ErrorIs(t, err, storepkg.ErrParseRedisURL)
	})

	t.Run("url from ATMOS_REDIS_URL env", func(t *testing.T) {
		t.Setenv("ATMOS_REDIS_URL", "redis://localhost:6379")
		opts, err := getRedisOptions(&RedisStoreOptions{})
		assert.NoError(t, err)
		assert.Equal(t, "localhost:6379", opts.Addr)
	})

	t.Run("invalid ATMOS_REDIS_URL returns parse error", func(t *testing.T) {
		t.Setenv("ATMOS_REDIS_URL", "not-a-valid-redis-url")
		_, err := getRedisOptions(&RedisStoreOptions{})
		assert.ErrorIs(t, err, storepkg.ErrParseRedisURL)
	})

	t.Run("missing url returns error", func(t *testing.T) {
		// Ensure the env fallback is empty so the missing-URL branch is taken.
		t.Setenv("ATMOS_REDIS_URL", "")
		_, err := getRedisOptions(&RedisStoreOptions{})
		assert.ErrorIs(t, err, storepkg.ErrMissingRedisURL)
	})
}

func TestRedisStore_RedisClient(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := NewMockRedisClient(ctrl)
	s := &RedisStore{redisClient: mockClient}

	assert.Equal(t, mockClient, s.RedisClient())
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
			ctrl := gomock.NewController(t)
			mockClient := NewMockRedisClient(ctrl)
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
			// GetKey prepends the prefix with a colon when prefix is set
			expectedKey := "myapp:" + tt.key
			mockClient.EXPECT().Get(context.Background(), expectedKey).Return(stringCmd(tt.mockReturn, tt.mockError))

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
		})
	}
}

func TestNewRedisStore_InvalidURL(t *testing.T) {
	_, err := NewRedisStore(RedisStoreOptions{URL: ptr("not-a-valid-redis-url")})
	assert.ErrorIs(t, err, storepkg.ErrParseRedisURL)
}

func TestRedisStore_getKey_NilDelimiter(t *testing.T) {
	s := &RedisStore{prefix: "p"} // stackDelimiter is nil.
	_, err := s.getKey("dev", "app", "k")
	assert.ErrorIs(t, err, storepkg.ErrStackDelimiterNotSet)
}

func TestRedisStore_Get_Validation(t *testing.T) {
	s := &RedisStore{stackDelimiter: ptr("/")}

	tests := []struct {
		name      string
		stack     string
		component string
		key       string
		want      error
	}{
		{"empty stack", "", "app", "k", storepkg.ErrEmptyStack},
		{"empty component", "dev", "", "k", storepkg.ErrEmptyComponent},
		{"empty key", "dev", "app", "", storepkg.ErrEmptyKey},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := s.Get(tt.stack, tt.component, tt.key)
			assert.ErrorIs(t, err, tt.want)
		})
	}

	t.Run("getKey error on nil delimiter", func(t *testing.T) {
		nilDelim := &RedisStore{} // nil delimiter.
		_, err := nilDelim.Get("dev", "app", "k")
		assert.ErrorIs(t, err, storepkg.ErrGetKey)
	})
}

func TestRedisStore_Set_Validation(t *testing.T) {
	s := &RedisStore{stackDelimiter: ptr("/")}

	tests := []struct {
		name      string
		stack     string
		component string
		key       string
		value     interface{}
		want      error
	}{
		{"empty stack", "", "app", "k", "v", storepkg.ErrEmptyStack},
		{"empty component", "dev", "", "k", "v", storepkg.ErrEmptyComponent},
		{"empty key", "dev", "app", "", "v", storepkg.ErrEmptyKey},
		{"nil value", "dev", "app", "k", nil, storepkg.ErrNilValue},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.ErrorIs(t, s.Set(tt.stack, tt.component, tt.key, tt.value), tt.want)
		})
	}

	t.Run("getKey error on nil delimiter", func(t *testing.T) {
		nilDelim := &RedisStore{} // nil delimiter.
		assert.ErrorIs(t, nilDelim.Set("dev", "app", "k", "v"), storepkg.ErrGetKey)
	})
}

func TestRedisStore_Set_ClientError(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := NewMockRedisClient(ctrl)
	s := &RedisStore{prefix: "p", redisClient: mockClient, stackDelimiter: ptr("/")}

	mockClient.EXPECT().Set(context.Background(), "p/dev/app/k", gomock.Any(), time.Duration(0)).
		Return(statusCmd("", fmt.Errorf("set boom")))

	err := s.Set("dev", "app", "k", "v")
	assert.Error(t, err)
}

func TestBuildRedisStore_ParseError(t *testing.T) {
	_, err := buildRedisStore("n", storepkg.StoreConfig{
		Options: map[string]interface{}{"prefix": []string{"x"}},
	})
	assert.ErrorIs(t, err, storepkg.ErrParseRedisOptions)
}

// TestRedisStore_Keys proves Keys pages through SCAN (never KEYS) until the cursor returns to 0,
// matches on a segment-bounded pattern (prefix + "/*", not prefix + "*"), and strips that prefix
// from each returned key. It also proves a same-level sibling that merely shares a character
// prefix (e.g. "vpc2" when scoped to "vpc") is excluded.
func TestRedisStore_Keys(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := NewMockRedisClient(ctrl)
	s := &RedisStore{redisClient: mockClient, prefix: "p", stackDelimiter: ptr("-")}

	ctx := context.Background()
	mockClient.EXPECT().Scan(ctx, uint64(0), "p/prod/vpc/*", int64(0)).
		Return(scanCmd(ctx, []string{"p/prod/vpc/image_tag"}, uint64(7), nil))
	mockClient.EXPECT().Scan(ctx, uint64(7), "p/prod/vpc/*", int64(0)).
		Return(scanCmd(ctx, []string{"p/prod/vpc/region", "p/prod/vpc2/key"}, uint64(0), nil))

	keys, err := s.Keys("prod", "vpc")
	assert.NoError(t, err)
	assert.ElementsMatch(t, []string{"image_tag", "region"}, keys)
}

// TestRedisStore_Keys_EscapesGlobMetacharacters proves a literal glob metacharacter in the
// stack/component name is escaped in the SCAN pattern rather than interpreted as a wildcard.
func TestRedisStore_Keys_EscapesGlobMetacharacters(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := NewMockRedisClient(ctrl)
	s := &RedisStore{redisClient: mockClient, prefix: "p", stackDelimiter: ptr("-")}

	ctx := context.Background()
	mockClient.EXPECT().Scan(ctx, uint64(0), `p/prod/vpc\[1\]/*`, int64(0)).
		Return(scanCmd(ctx, []string{"p/prod/vpc[1]/image_tag"}, uint64(0), nil))

	keys, err := s.Keys("prod", "vpc[1]")
	assert.NoError(t, err)
	assert.ElementsMatch(t, []string{"image_tag"}, keys)
}

func TestRedisStore_Keys_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := NewMockRedisClient(ctrl)
	s := &RedisStore{redisClient: mockClient, prefix: "p", stackDelimiter: ptr("-")}

	ctx := context.Background()
	mockClient.EXPECT().Scan(ctx, uint64(0), "p/prod/vpc/*", int64(0)).
		Return(scanCmd(ctx, nil, uint64(0), fmt.Errorf("scan boom")))

	_, err := s.Keys("prod", "vpc")
	assert.Error(t, err)
	assert.ErrorIs(t, err, storepkg.ErrScanRedisKeys)
}
