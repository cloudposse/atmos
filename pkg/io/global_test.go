package io

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
	"testing"
)

func TestInitialize(t *testing.T) {
	// Reset global state
	resetGlobals()

	err := Initialize()
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	if Data == nil {
		t.Error("Data writer is nil after Initialize()")
	}

	if UI == nil {
		t.Error("UI writer is nil after Initialize()")
	}

	if globalContext == nil {
		t.Error("globalContext is nil after Initialize()")
	}
}

func TestInitialize_OnlyOnce(t *testing.T) {
	// Reset global state
	resetGlobals()

	// Call Initialize multiple times
	err1 := Initialize()
	dataWriter1 := Data
	uiWriter1 := UI

	err2 := Initialize()
	dataWriter2 := Data
	uiWriter2 := UI

	if err1 != nil || err2 != nil {
		t.Fatalf("Initialize() errors = %v, %v", err1, err2)
	}

	// Should return same writers (only initialized once)
	if fmt.Sprintf("%p", dataWriter1) != fmt.Sprintf("%p", dataWriter2) {
		t.Error("Data writer changed after second Initialize()")
	}

	if fmt.Sprintf("%p", uiWriter1) != fmt.Sprintf("%p", uiWriter2) {
		t.Error("UI writer changed after second Initialize()")
	}
}

func TestGlobalWriters_AutomaticMasking(t *testing.T) {
	// Reset and initialize with test environment
	resetGlobals()

	// Set test environment variable
	t.Setenv("TEST_SECRET", "my_secret_value_12345")

	// Initialize
	err := Initialize()
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	// Register the test secret
	RegisterSecret("my_secret_value_12345")

	// Create test buffers
	dataBuffer := &bytes.Buffer{}
	uiBuffer := &bytes.Buffer{}

	// Replace global writers with test buffers (wrapped with masking)
	Data = MaskWriter(dataBuffer)
	UI = MaskWriter(uiBuffer)

	// Write content with secret to Data
	fmt.Fprintf(Data, "The secret is: my_secret_value_12345\n")

	// Verify secret is masked
	dataOutput := dataBuffer.String()
	if strings.Contains(dataOutput, "my_secret_value_12345") {
		t.Errorf("Secret not masked in Data output: %s", dataOutput)
	}
	if !strings.Contains(dataOutput, MaskReplacement) {
		t.Errorf("Mask replacement not found in Data output: %s", dataOutput)
	}

	// Write content with secret to UI
	fmt.Fprintf(UI, "Using secret: my_secret_value_12345\n")

	// Verify secret is masked
	uiOutput := uiBuffer.String()
	if strings.Contains(uiOutput, "my_secret_value_12345") {
		t.Errorf("Secret not masked in UI output: %s", uiOutput)
	}
	if !strings.Contains(uiOutput, MaskReplacement) {
		t.Errorf("Mask replacement not found in UI output: %s", uiOutput)
	}
}

func TestRegisterSecret(t *testing.T) {
	resetGlobals()
	Initialize()

	secret := "test_secret_abc123"

	// Register secret FIRST
	RegisterSecret(secret)

	// Then create masked writer (it will use the global masker that now has the secret)
	buf := &bytes.Buffer{}
	masked := MaskWriter(buf)

	// Write secret
	fmt.Fprintf(masked, "Secret: %s\n", secret)

	// Verify masked
	output := buf.String()
	if strings.Contains(output, secret) {
		t.Errorf("Secret not masked: %s", output)
	}
	if !strings.Contains(output, MaskReplacement) {
		t.Errorf("Expected mask replacement in output: %s", output)
	}
}

func TestRegisterSecret_WithEncodings(t *testing.T) {
	resetGlobals()
	Initialize()

	secret := "my-api-key"
	RegisterSecret(secret)

	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "plain secret",
			input: "my-api-key",
		},
		{
			name:  "base64 encoded",
			input: "bXktYXBpLWtleQ==", // base64 of "my-api-key"
		},
		{
			name:  "url encoded",
			input: "my-api-key", // No special chars, so same
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			masked := MaskWriter(buf)

			fmt.Fprintf(masked, "Value: %s\n", tt.input)

			output := buf.String()
			if strings.Contains(output, tt.input) && tt.input != "my-api-key" {
				// Allow plain secret in output as long as it's in masked form
				if !strings.Contains(output, MaskReplacement) {
					t.Errorf("%s not masked: %s", tt.name, output)
				}
			}
		})
	}
}

func TestRegisterValue(t *testing.T) {
	resetGlobals()
	Initialize()

	value := "simple_value_xyz"
	RegisterValue(value)

	buf := &bytes.Buffer{}
	masked := MaskWriter(buf)

	fmt.Fprintf(masked, "Value: %s\n", value)

	output := buf.String()
	if strings.Contains(output, value) {
		t.Errorf("Value not masked: %s", output)
	}
}

func TestRegisterValue_Empty(t *testing.T) {
	resetGlobals()
	Initialize()

	// Should not panic or error
	RegisterValue("")

	buf := &bytes.Buffer{}
	masked := MaskWriter(buf)

	fmt.Fprintf(masked, "Test output\n")

	output := buf.String()
	if output != "Test output\n" {
		t.Errorf("Output modified unexpectedly: %s", output)
	}
}

func TestRegisterPattern(t *testing.T) {
	resetGlobals()
	Initialize()

	// Register pattern for API keys
	err := RegisterPattern(`api_key=[A-Za-z0-9]+`)
	if err != nil {
		t.Fatalf("RegisterPattern() error = %v", err)
	}

	buf := &bytes.Buffer{}
	masked := MaskWriter(buf)

	fmt.Fprintf(masked, "Config: api_key=abc123xyz\n")

	output := buf.String()
	if strings.Contains(output, "api_key=abc123xyz") {
		t.Errorf("Pattern not masked: %s", output)
	}
	if !strings.Contains(output, MaskReplacement) {
		t.Errorf("Expected mask replacement: %s", output)
	}
}

func TestRegisterPattern_Invalid(t *testing.T) {
	resetGlobals()
	Initialize()

	// Invalid regex pattern
	err := RegisterPattern(`[invalid(regex`)
	if err == nil {
		t.Error("RegisterPattern() should return error for invalid pattern")
	}
}

func TestRegisterPattern_Empty(t *testing.T) {
	resetGlobals()
	Initialize()

	// Empty pattern should not error
	err := RegisterPattern("")
	if err != nil {
		t.Errorf("RegisterPattern() with empty string error = %v", err)
	}
}

func TestMaskWriter(t *testing.T) {
	resetGlobals()
	Initialize()

	secret := "mask_this_secret"
	RegisterSecret(secret)

	// Create custom buffer
	buf := &bytes.Buffer{}

	// Wrap with masking
	masked := MaskWriter(buf)

	// Write secret
	fmt.Fprintf(masked, "The secret is: %s\n", secret)

	// Verify masked in buffer
	output := buf.String()
	if strings.Contains(output, secret) {
		t.Errorf("Secret not masked: %s", output)
	}
	if !strings.Contains(output, MaskReplacement) {
		t.Errorf("Expected mask replacement: %s", output)
	}
}

func TestMaskWriter_BeforeInitialize(t *testing.T) {
	resetGlobals()

	// Call MaskWriter before explicit Initialize
	buf := &bytes.Buffer{}
	masked := MaskWriter(buf)

	if masked == nil {
		t.Error("MaskWriter() returned nil")
	}

	// Should auto-initialize
	if globalContext == nil {
		t.Error("MaskWriter() did not auto-initialize global context")
	}
}

func TestRegisterCommonSecrets(t *testing.T) {
	resetGlobals()

	// Set test environment variables
	testEnv := map[string]string{
		"AWS_ACCESS_KEY_ID":     "AKIAIOSFODNN7EXAMPLE",
		"AWS_SECRET_ACCESS_KEY": "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		"GITHUB_TOKEN":          "ghp_" + strings.Repeat("a", 36),
		"DATADOG_API_KEY":       "dd_api_key_example",
	}

	for key, value := range testEnv {
		t.Setenv(key, value)
	}

	// Initialize (will call registerCommonSecrets)
	Initialize()

	// Verify secrets are registered
	buf := &bytes.Buffer{}
	masked := MaskWriter(buf)

	for key, value := range testEnv {
		fmt.Fprintf(masked, "%s=%s\n", key, value)
	}

	output := buf.String()

	// Check that secrets are masked
	for _, value := range testEnv {
		if strings.Contains(output, value) {
			t.Errorf("Secret value not masked: %s", value)
		}
	}

	// Should contain multiple mask replacements
	maskCount := strings.Count(output, MaskReplacement)
	if maskCount < len(testEnv) {
		t.Errorf("Expected at least %d mask replacements, got %d", len(testEnv), maskCount)
	}
}

func TestGetContext(t *testing.T) {
	resetGlobals()

	// Get context (should auto-initialize)
	ctx := GetContext()

	if ctx == nil {
		t.Fatal("GetContext() returned nil")
	}

	if ctx.Streams() == nil {
		t.Error("Context.Streams() is nil")
	}

	if ctx.Masker() == nil {
		t.Error("Context.Masker() is nil")
	}

	if ctx.Config() == nil {
		t.Error("Context.Config() is nil")
	}
}

func TestGetContext_BeforeInitialize(t *testing.T) {
	resetGlobals()

	// Get context before explicit initialize
	ctx := GetContext()

	if ctx == nil {
		t.Error("GetContext() should auto-initialize and return context")
	}

	// Verify global writers are also initialized
	if Data == nil {
		t.Error("Data writer not initialized by GetContext()")
	}

	if UI == nil {
		t.Error("UI writer not initialized by GetContext()")
	}
}

func TestConcurrentAccess(t *testing.T) {
	resetGlobals()

	var wg sync.WaitGroup
	goroutines := 10

	// Concurrent initialization attempts
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = Initialize()
			_ = GetContext()
		}()
	}

	wg.Wait()

	// Should only initialize once
	if globalContext == nil {
		t.Error("Global context is nil after concurrent access")
	}

	if Data == nil {
		t.Error("Data writer is nil after concurrent access")
	}

	if UI == nil {
		t.Error("UI writer is nil after concurrent access")
	}
}

func TestConcurrentMasking(t *testing.T) {
	resetGlobals()
	Initialize()

	secret := "concurrent_secret_xyz"
	RegisterSecret(secret)

	var wg sync.WaitGroup
	goroutines := 10

	// Concurrent writes with masking
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			buf := &bytes.Buffer{}
			masked := MaskWriter(buf)

			fmt.Fprintf(masked, "Goroutine %d: secret=%s\n", id, secret)

			output := buf.String()
			if strings.Contains(output, secret) {
				t.Errorf("Secret not masked in goroutine %d: %s", id, output)
			}
		}(i)
	}

	wg.Wait()
}

// resetGlobals resets global state for testing.
func resetGlobals() {
	Data = nil
	UI = nil
	globalContext = nil
	initOnce = sync.Once{}
	initErr = nil
}
