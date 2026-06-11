package gcp

import (
	"testing"
	"time"

	"google.golang.org/api/option"
)

func TestGetClientOptions(t *testing.T) {
	tests := []struct {
		name     string
		opts     AuthOptions
		expected int // number of client options returned
	}{
		{
			name: "no credentials - use ADC",
			opts: AuthOptions{
				Credentials: "",
			},
			expected: 0, // ADC uses no explicit options
		},
		{
			name: "access token",
			opts: AuthOptions{
				AccessToken: "ya29.access-token",
				TokenExpiry: time.Now().Add(time.Hour),
			},
			expected: 1, // WithTokenSource
		},
		{
			name: "JSON credentials",
			opts: AuthOptions{
				Credentials: `{"type": "service_account", "project_id": "test"}`,
			},
			expected: 1, // WithCredentialsJSON
		},
		{
			name: "file path credentials",
			opts: AuthOptions{
				Credentials: "/path/to/service-account.json",
			},
			expected: 1, // WithCredentialsFile
		},
		{
			name: "JSON with whitespace",
			opts: AuthOptions{
				Credentials: `  {"type": "service_account"}  `,
			},
			expected: 1, // WithCredentialsJSON
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientOpts := GetClientOptions(tt.opts)
			if len(clientOpts) != tt.expected {
				t.Errorf("GetClientOptions() returned %d options, expected %d", len(clientOpts), tt.expected)
			}
		})
	}
}

func TestGetSecretManagerClientOptions(t *testing.T) {
	t.Run("uses regular auth options without emulator", func(t *testing.T) {
		t.Setenv("SECRET_MANAGER_EMULATOR_HOST", "")

		clientOpts := GetSecretManagerClientOptions(AuthOptions{
			Credentials: `{"type": "service_account", "project_id": "test"}`,
		})
		if len(clientOpts) != 1 {
			t.Errorf("GetSecretManagerClientOptions() returned %d options, expected regular credentials option", len(clientOpts))
		}
	})

	t.Run("uses plaintext emulator options when configured", func(t *testing.T) {
		t.Setenv("SECRET_MANAGER_EMULATOR_HOST", "localhost:4588")

		clientOpts := GetSecretManagerClientOptions(AuthOptions{
			Credentials: `{"type": "service_account", "project_id": "test"}`,
		})
		if len(clientOpts) != 3 {
			t.Errorf("GetSecretManagerClientOptions() returned %d options, expected emulator endpoint/auth/dial options", len(clientOpts))
		}
	})
}

func TestSecretManagerEmulatorHost(t *testing.T) {
	tests := []struct {
		name string
		env  string
		want string
	}{
		{
			name: "empty",
			want: "",
		},
		{
			name: "host port",
			env:  "localhost:4588",
			want: "localhost:4588",
		},
		{
			name: "http URL",
			env:  "http://localhost:4588",
			want: "localhost:4588",
		},
		{
			name: "https URL with path",
			env:  "https://floci-gcp:4588/api",
			want: "floci-gcp:4588",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("SECRET_MANAGER_EMULATOR_HOST", tt.env)
			if got := secretManagerEmulatorHost(); got != tt.want {
				t.Errorf("secretManagerEmulatorHost() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetCredentialsFromBackend(t *testing.T) {
	tests := []struct {
		name     string
		backend  map[string]any
		expected string
	}{
		{
			name: "credentials present",
			backend: map[string]any{
				"credentials": "/path/to/creds.json",
				"bucket":      "test-bucket",
			},
			expected: "/path/to/creds.json",
		},
		{
			name: "JSON credentials",
			backend: map[string]any{
				"credentials": `{"type": "service_account"}`,
			},
			expected: `{"type": "service_account"}`,
		},
		{
			name: "no credentials",
			backend: map[string]any{
				"bucket": "test-bucket",
			},
			expected: "",
		},
		{
			name:     "empty backend",
			backend:  map[string]any{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetCredentialsFromBackend(tt.backend)
			if result != tt.expected {
				t.Errorf("GetCredentialsFromBackend() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestGetCredentialsFromStore(t *testing.T) {
	tests := []struct {
		name        string
		credentials *string
		expected    string
	}{
		{
			name:        "credentials present",
			credentials: stringPtr("/path/to/creds.json"),
			expected:    "/path/to/creds.json",
		},
		{
			name:        "JSON credentials",
			credentials: stringPtr(`{"type": "service_account"}`),
			expected:    `{"type": "service_account"}`,
		},
		{
			name:        "empty credentials",
			credentials: stringPtr(""),
			expected:    "",
		},
		{
			name:        "nil credentials",
			credentials: nil,
			expected:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetCredentialsFromStore(tt.credentials)
			if result != tt.expected {
				t.Errorf("GetCredentialsFromStore() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestClientOptionsCreation(t *testing.T) {
	// Test that we can actually create valid client options without errors
	tests := []struct {
		name        string
		credentials string
		expectType  string
	}{
		{
			name:        "JSON credentials create WithCredentialsJSON option",
			credentials: `{"type": "service_account", "project_id": "test"}`,
			expectType:  "JSON",
		},
		{
			name:        "file path creates WithCredentialsFile option",
			credentials: "/path/to/service-account.json",
			expectType:  "File",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := GetClientOptions(AuthOptions{
				Credentials: tt.credentials,
			})

			// We can't easily inspect the actual option type without reflection,
			// but we can verify that an option was created
			if len(opts) != 1 {
				t.Errorf("Expected 1 client option, got %d", len(opts))
			}

			// Verify the option is of type option.ClientOption
			var _ option.ClientOption = opts[0]
		})
	}
}

// Helper function to create string pointers for tests
func stringPtr(s string) *string {
	return &s
}
