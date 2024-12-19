package store

import (
	"reflect"
	"testing"
)

func TestNewMemoryStore(t *testing.T) {
	store, err := NewInMemoryStore(nil)
	if err != nil {
		t.Errorf("NewMemoryStore() error = %v, want nil", err)
	}
	if store == nil {
		t.Error("NewMemoryStore() returned nil store")
	}
}

func TestMemoryStoreSet(t *testing.T) {
	store := &InMemoryStore{
		data: make(map[string]interface{}),
	}

	tests := []struct {
		name  string
		key   string
		value interface{}
	}{
		{
			name:  "string value",
			key:   "test-key",
			value: "test-value",
		},
		{
			name:  "integer value",
			key:   "number",
			value: 42,
		},
		{
			name:  "complex value",
			key:   "object",
			value: map[string]string{"nested": "value"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := store.Set(tt.key, tt.value)
			if err != nil {
				t.Errorf("Set() error = %v, want nil", err)
			}

			// Verify the value was stored correctly
			got, exists := store.data[tt.key]
			if !exists {
				t.Errorf("Set() key %v not found in store", tt.key)
			}

			// Use reflect.DeepEqual for comparing complex types
			if !reflect.DeepEqual(got, tt.value) {
				t.Errorf("Set() = %v, want %v", got, tt.value)
			}
		})
	}
}

func TestMemoryStoreGet(t *testing.T) {
	store := &InMemoryStore{
		data: make(map[string]interface{}),
	}

	// Setup test data
	testKey := "test-key"
	testValue := "test-value"
	store.data[testKey] = testValue

	tests := []struct {
		name      string
		key       string
		want      interface{}
		wantError bool
	}{
		{
			name:      "existing key",
			key:       testKey,
			want:      testValue,
			wantError: false,
		},
		{
			name:      "non-existing key",
			key:       "non-existing",
			want:      nil,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := store.Get(tt.key)
			if (err != nil) != tt.wantError {
				t.Errorf("Get() error = %v, wantError %v", err, tt.wantError)
				return
			}
			if got != tt.want {
				t.Errorf("Get() = %v, want %v", got, tt.want)
			}
		})
	}
}
