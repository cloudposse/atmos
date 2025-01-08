package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInMemoryStore_Set(t *testing.T) {
	tests := []struct {
		name      string
		stack     string
		component string
		key       string
		value     interface{}
		wantErr   bool
	}{
		{
			name:      "successful set",
			stack:     "dev-us-west-2",
			component: "app/service",
			key:       "config-key",
			value:     "test-value",
			wantErr:   false,
		},
		{
			name:      "complex value",
			stack:     "dev-us-west-2",
			component: "app/service",
			key:       "config-map",
			value:     map[string]string{"key": "value"},
			wantErr:   false,
		},
		{
			name:      "empty stack",
			stack:     "",
			component: "app/service",
			key:       "config-key",
			value:     "test-value",
			wantErr:   true,
		},
		{
			name:      "empty component",
			stack:     "dev-us-west-2",
			component: "",
			key:       "config-key",
			value:     "test-value",
			wantErr:   true,
		},
		{
			name:      "empty key",
			stack:     "dev-us-west-2",
			component: "app/service",
			key:       "",
			value:     "test-value",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, err := NewInMemoryStore()
			assert.NoError(t, err)

			err = store.Set(tt.stack, tt.component, tt.key, tt.value)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				// Verify the value was stored correctly
				got, err := store.Get(tt.stack, tt.component, tt.key)
				assert.NoError(t, err)
				assert.Equal(t, tt.value, got)
			}
		})
	}
}

func TestInMemoryStore_Get(t *testing.T) {
	store, err := NewInMemoryStore()
	assert.NoError(t, err)

	// Setup test data
	testStack := "dev-us-west-2"
	testComponent := "app/service"
	testKey := "config-key"
	testValue := "test-value"

	err = store.Set(testStack, testComponent, testKey, testValue)
	assert.NoError(t, err)

	tests := []struct {
		name      string
		stack     string
		component string
		key       string
		want      interface{}
		wantErr   bool
	}{
		{
			name:      "existing key",
			stack:     testStack,
			component: testComponent,
			key:       testKey,
			want:      testValue,
			wantErr:   false,
		},
		{
			name:      "non-existing key",
			stack:     testStack,
			component: testComponent,
			key:       "non-existing",
			want:      nil,
			wantErr:   true,
		},
		{
			name:      "empty stack",
			stack:     "",
			component: testComponent,
			key:       testKey,
			want:      nil,
			wantErr:   true,
		},
		{
			name:      "empty component",
			stack:     testStack,
			component: "",
			key:       testKey,
			want:      nil,
			wantErr:   true,
		},
		{
			name:      "empty key",
			stack:     testStack,
			component: testComponent,
			key:       "",
			want:      nil,
			wantErr:   true,
		},
		{
			name:      "wrong stack",
			stack:     "wrong-stack",
			component: testComponent,
			key:       testKey,
			want:      nil,
			wantErr:   true,
		},
		{
			name:      "wrong component",
			stack:     testStack,
			component: "wrong/component",
			key:       testKey,
			want:      nil,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := store.Get(tt.stack, tt.component, tt.key)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, got)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}
