package backend

import (
	"testing"
)

func TestLoadStreamConfig_MissingCredentials(t *testing.T) {
	// Clear environment variables.
	t.Setenv("CLOUDFLARE_ACCOUNT_ID", "")
	t.Setenv("CLOUDFLARE_STREAM_API_TOKEN", "")

	// Try to load config with no credentials.
	_, err := LoadStreamConfig(nil)

	// Should return ErrMissingStreamCredentials.
	if err == nil {
		t.Fatal("expected error for missing credentials, got nil")
	}

	// Check error type.
	if !isErrorType(err, ErrMissingStreamCredentials) {
		t.Errorf("expected ErrMissingStreamCredentials, got: %v", err)
	}

	// Error should mention the missing variables.
	errMsg := err.Error()
	if !contains(errMsg, "CLOUDFLARE_ACCOUNT_ID") {
		t.Errorf("error should mention CLOUDFLARE_ACCOUNT_ID: %s", errMsg)
	}
	if !contains(errMsg, "CLOUDFLARE_STREAM_API_TOKEN") {
		t.Errorf("error should mention CLOUDFLARE_STREAM_API_TOKEN: %s", errMsg)
	}
}

func TestLoadStreamConfig_MissingAccountID(t *testing.T) {
	t.Setenv("CLOUDFLARE_ACCOUNT_ID", "")
	t.Setenv("CLOUDFLARE_STREAM_API_TOKEN", "test-token")

	_, err := LoadStreamConfig(nil)

	if err == nil {
		t.Fatal("expected error for missing CLOUDFLARE_ACCOUNT_ID, got nil")
	}

	if !isErrorType(err, ErrMissingStreamCredentials) {
		t.Errorf("expected ErrMissingStreamCredentials, got: %v", err)
	}

	errMsg := err.Error()
	if !contains(errMsg, "CLOUDFLARE_ACCOUNT_ID") {
		t.Errorf("error should mention CLOUDFLARE_ACCOUNT_ID: %s", errMsg)
	}
}

func TestLoadStreamConfig_MissingAPIToken(t *testing.T) {
	t.Setenv("CLOUDFLARE_ACCOUNT_ID", "test-account-id")
	t.Setenv("CLOUDFLARE_STREAM_API_TOKEN", "")

	_, err := LoadStreamConfig(nil)

	if err == nil {
		t.Fatal("expected error for missing CLOUDFLARE_STREAM_API_TOKEN, got nil")
	}

	if !isErrorType(err, ErrMissingStreamCredentials) {
		t.Errorf("expected ErrMissingStreamCredentials, got: %v", err)
	}

	errMsg := err.Error()
	if !contains(errMsg, "CLOUDFLARE_STREAM_API_TOKEN") {
		t.Errorf("error should mention CLOUDFLARE_STREAM_API_TOKEN: %s", errMsg)
	}
}

func TestLoadStreamConfig_Success(t *testing.T) {
	t.Setenv("CLOUDFLARE_ACCOUNT_ID", "test-account-id")
	t.Setenv("CLOUDFLARE_STREAM_API_TOKEN", "test-token")

	config, err := LoadStreamConfig(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if config.AccountID != "test-account-id" {
		t.Errorf("expected AccountID 'test-account-id', got: %s", config.AccountID)
	}

	if config.APIToken != "test-token" {
		t.Errorf("expected APIToken 'test-token', got: %s", config.APIToken)
	}
}

func TestLoadStreamConfig_WithCustomerSubdomain(t *testing.T) {
	t.Setenv("CLOUDFLARE_ACCOUNT_ID", "test-account-id")
	t.Setenv("CLOUDFLARE_STREAM_API_TOKEN", "test-token")
	t.Setenv("CLOUDFLARE_CUSTOMER_SUBDOMAIN", "customer-test.cloudflarestream.com")

	config, err := LoadStreamConfig(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if config.CustomerSubdomain != "customer-test.cloudflarestream.com" {
		t.Errorf("expected CustomerSubdomain 'customer-test.cloudflarestream.com', got: %s", config.CustomerSubdomain)
	}
}

func TestStreamBackend_SupportsFormat(t *testing.T) {
	backend := &StreamBackend{
		config: &StreamConfig{},
	}

	tests := []struct {
		ext      string
		expected bool
	}{
		{"mp4", true},
		{".mp4", true},
		{"MP4", true},
		{"webm", true},
		{"avi", true},
		{"mkv", true},
		{"mov", true},
		{"flv", true},
		{"mpg", true},
		{"mpeg", true},
		{"3gp", true},
		{"m4v", true},
		{"gif", false},
		{"png", false},
		{"jpg", false},
		{"jpeg", false},
		{"txt", false},
	}

	for _, tt := range tests {
		t.Run(tt.ext, func(t *testing.T) {
			result := backend.SupportsFormat(tt.ext)
			if result != tt.expected {
				t.Errorf("SupportsFormat(%q) = %v, expected %v", tt.ext, result, tt.expected)
			}
		})
	}
}

func TestStreamBackend_GetPublicURL(t *testing.T) {
	tests := []struct {
		name              string
		customerSubdomain string
		remotePath        string
		expectedURL       string
	}{
		{
			name:              "with customer subdomain",
			customerSubdomain: "customer-test.cloudflarestream.com",
			remotePath:        "abc123",
			expectedURL:       "https://customer-test.cloudflarestream.com/abc123/watch",
		},
		{
			name:              "without customer subdomain",
			customerSubdomain: "",
			remotePath:        "abc123",
			expectedURL:       "https://customer-unknown.cloudflarestream.com/abc123/watch",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := &StreamBackend{
				config: &StreamConfig{
					CustomerSubdomain: tt.customerSubdomain,
				},
			}

			url := backend.GetPublicURL(tt.remotePath)
			if url != tt.expectedURL {
				t.Errorf("GetPublicURL(%q) = %q, expected %q", tt.remotePath, url, tt.expectedURL)
			}
		})
	}
}

func TestExtractSubdomain(t *testing.T) {
	tests := []struct {
		previewURL        string
		expectedSubdomain string
	}{
		{
			previewURL:        "https://customer-test.cloudflarestream.com/abc123/watch",
			expectedSubdomain: "customer-test.cloudflarestream.com",
		},
		{
			previewURL:        "http://customer-test.cloudflarestream.com/abc123/watch",
			expectedSubdomain: "customer-test.cloudflarestream.com",
		},
		{
			previewURL:        "customer-test.cloudflarestream.com/abc123/watch",
			expectedSubdomain: "customer-test.cloudflarestream.com",
		},
		{
			previewURL:        "",
			expectedSubdomain: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.previewURL, func(t *testing.T) {
			subdomain := extractSubdomain(tt.previewURL)
			if subdomain != tt.expectedSubdomain {
				t.Errorf("extractSubdomain(%q) = %q, expected %q", tt.previewURL, subdomain, tt.expectedSubdomain)
			}
		})
	}
}

// isErrorType checks if an error wraps a specific error type.
func isErrorType(err, target error) bool {
	if err == nil {
		return target == nil
	}
	// Simple string-based check (can be replaced with errors.Is if errors are wrapped).
	return contains(err.Error(), target.Error())
}
