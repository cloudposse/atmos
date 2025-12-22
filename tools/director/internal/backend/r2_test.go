package backend

import (
	"os"
	"testing"
)

func TestLoadR2Config_MissingCredentials(t *testing.T) {
	// Clear all R2 environment variables.
	os.Unsetenv("CLOUDFLARE_ACCOUNT_ID")
	os.Unsetenv("CLOUDFLARE_R2_ACCESS_KEY_ID")
	os.Unsetenv("CLOUDFLARE_R2_SECRET_ACCESS_KEY")
	os.Unsetenv("CLOUDFLARE_BUCKET_NAME")

	backendConfig := map[string]interface{}{
		"r2": map[string]interface{}{
			"bucket_name": "test-bucket",
		},
	}

	_, err := LoadR2Config(backendConfig)
	if err == nil {
		t.Fatal("expected error for missing credentials, got nil")
	}

	// Check that error mentions missing variables.
	errMsg := err.Error()
	if !contains(errMsg, "CLOUDFLARE_ACCOUNT_ID") {
		t.Errorf("error should mention CLOUDFLARE_ACCOUNT_ID, got: %s", errMsg)
	}
	if !contains(errMsg, "CLOUDFLARE_R2_ACCESS_KEY_ID") {
		t.Errorf("error should mention CLOUDFLARE_R2_ACCESS_KEY_ID, got: %s", errMsg)
	}
	if !contains(errMsg, "CLOUDFLARE_R2_SECRET_ACCESS_KEY") {
		t.Errorf("error should mention CLOUDFLARE_R2_SECRET_ACCESS_KEY, got: %s", errMsg)
	}
}

func TestLoadR2Config_Success(t *testing.T) {
	// Set required environment variables.
	os.Setenv("CLOUDFLARE_ACCOUNT_ID", "test-account-id")
	os.Setenv("CLOUDFLARE_R2_ACCESS_KEY_ID", "test-access-key")
	os.Setenv("CLOUDFLARE_R2_SECRET_ACCESS_KEY", "test-secret-key")
	defer func() {
		os.Unsetenv("CLOUDFLARE_ACCOUNT_ID")
		os.Unsetenv("CLOUDFLARE_R2_ACCESS_KEY_ID")
		os.Unsetenv("CLOUDFLARE_R2_SECRET_ACCESS_KEY")
	}()

	backendConfig := map[string]interface{}{
		"r2": map[string]interface{}{
			"bucket_name": "test-bucket",
			"prefix":      "vhs/",
			"base_url":    "https://pub-test.r2.dev/vhs",
		},
	}

	config, err := LoadR2Config(backendConfig)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if config.AccountID != "test-account-id" {
		t.Errorf("expected AccountID test-account-id, got %s", config.AccountID)
	}
	if config.AccessKeyID != "test-access-key" {
		t.Errorf("expected AccessKeyID test-access-key, got %s", config.AccessKeyID)
	}
	if config.SecretKey != "test-secret-key" {
		t.Errorf("expected SecretKey test-secret-key, got %s", config.SecretKey)
	}
	if config.BucketName != "test-bucket" {
		t.Errorf("expected BucketName test-bucket, got %s", config.BucketName)
	}
	if config.Prefix != "vhs/" {
		t.Errorf("expected Prefix vhs/, got %s", config.Prefix)
	}
	if config.BaseURL != "https://pub-test.r2.dev/vhs" {
		t.Errorf("expected BaseURL https://pub-test.r2.dev/vhs, got %s", config.BaseURL)
	}
}

func TestLoadR2Config_EnvOverride(t *testing.T) {
	// Set required environment variables.
	os.Setenv("CLOUDFLARE_ACCOUNT_ID", "test-account-id")
	os.Setenv("CLOUDFLARE_R2_ACCESS_KEY_ID", "test-access-key")
	os.Setenv("CLOUDFLARE_R2_SECRET_ACCESS_KEY", "test-secret-key")
	os.Setenv("CLOUDFLARE_BUCKET_NAME", "env-bucket")
	defer func() {
		os.Unsetenv("CLOUDFLARE_ACCOUNT_ID")
		os.Unsetenv("CLOUDFLARE_R2_ACCESS_KEY_ID")
		os.Unsetenv("CLOUDFLARE_R2_SECRET_ACCESS_KEY")
		os.Unsetenv("CLOUDFLARE_BUCKET_NAME")
	}()

	backendConfig := map[string]interface{}{
		"r2": map[string]interface{}{
			"bucket_name": "config-bucket",
		},
	}

	config, err := LoadR2Config(backendConfig)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Environment variable should override config.
	if config.BucketName != "env-bucket" {
		t.Errorf("expected BucketName env-bucket (from env var), got %s", config.BucketName)
	}
}

func TestLoadR2Config_MissingBucketName(t *testing.T) {
	// Set required environment variables but no bucket name.
	os.Setenv("CLOUDFLARE_ACCOUNT_ID", "test-account-id")
	os.Setenv("CLOUDFLARE_R2_ACCESS_KEY_ID", "test-access-key")
	os.Setenv("CLOUDFLARE_R2_SECRET_ACCESS_KEY", "test-secret-key")
	defer func() {
		os.Unsetenv("CLOUDFLARE_ACCOUNT_ID")
		os.Unsetenv("CLOUDFLARE_R2_ACCESS_KEY_ID")
		os.Unsetenv("CLOUDFLARE_R2_SECRET_ACCESS_KEY")
	}()

	backendConfig := map[string]interface{}{
		"r2": map[string]interface{}{
			// No bucket_name
		},
	}

	_, err := LoadR2Config(backendConfig)
	if err == nil {
		t.Fatal("expected error for missing bucket_name, got nil")
	}

	errMsg := err.Error()
	if !contains(errMsg, "bucket_name") {
		t.Errorf("error should mention bucket_name, got: %s", errMsg)
	}
}

func TestR2Config_GetEndpoint(t *testing.T) {
	config := &R2Config{
		AccountID: "test-account-123",
	}

	expected := "https://test-account-123.r2.cloudflarestorage.com"
	if config.GetEndpoint() != expected {
		t.Errorf("expected endpoint %s, got %s", expected, config.GetEndpoint())
	}
}

func TestR2Backend_GetPublicURL(t *testing.T) {
	tests := []struct {
		name       string
		config     *R2Config
		remotePath string
		expected   string
	}{
		{
			name: "with base URL and prefix",
			config: &R2Config{
				BucketName: "test-bucket",
				Prefix:     "vhs/",
				BaseURL:    "https://pub-test.r2.dev/vhs",
			},
			remotePath: "scene.gif",
			expected:   "https://pub-test.r2.dev/vhs/vhs/scene.gif",
		},
		{
			name: "with base URL no prefix",
			config: &R2Config{
				BucketName: "test-bucket",
				BaseURL:    "https://pub-test.r2.dev",
			},
			remotePath: "scene.gif",
			expected:   "https://pub-test.r2.dev/scene.gif",
		},
		{
			name: "without base URL",
			config: &R2Config{
				BucketName: "test-bucket",
				Prefix:     "vhs/",
			},
			remotePath: "scene.gif",
			expected:   "https://test-bucket/vhs/scene.gif",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend, err := NewR2Backend(tt.config)
			if err != nil {
				t.Fatalf("failed to create backend: %v", err)
			}

			url := backend.GetPublicURL(tt.remotePath)
			if url != tt.expected {
				t.Errorf("expected URL %s, got %s", tt.expected, url)
			}
		})
	}
}

// Helper function to check if string contains substring.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsAt(s, substr))
}

func containsAt(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
