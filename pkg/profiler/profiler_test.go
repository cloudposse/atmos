package profiler

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"

	"go.yaml.in/yaml/v3"
)

func getFreeTCPPort(t *testing.T) int {
	t.Helper()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to allocate tcp port: %v", err)
	}
	defer l.Close()

	return l.Addr().(*net.TCPAddr).Port
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config.Enabled {
		t.Error("Default config should have profiler disabled")
	}
	if config.Port != 6060 {
		t.Errorf("Expected default port 6060, got %d", config.Port)
	}
	if config.Host != "localhost" {
		t.Errorf("Expected default host 'localhost', got %s", config.Host)
	}
	if config.ProfileType != ProfileTypeCPU {
		t.Errorf("Expected default profile type CPU, got %s", config.ProfileType)
	}
}

func TestNew(t *testing.T) {
	config := Config{
		Enabled: true,
		Port:    8080,
		Host:    "127.0.0.1",
	}

	profiler := New(config)
	if profiler == nil {
		t.Fatal("New() returned nil profiler")
	}

	if profiler.config.Enabled != config.Enabled {
		t.Errorf("Expected enabled %v, got %v", config.Enabled, profiler.config.Enabled)
	}
	if profiler.config.Port != config.Port {
		t.Errorf("Expected port %d, got %d", config.Port, profiler.config.Port)
	}
	if profiler.config.Host != config.Host {
		t.Errorf("Expected host %s, got %s", config.Host, profiler.config.Host)
	}
}

func TestServerStartStop(t *testing.T) {
	port := getFreeTCPPort(t)
	config := Config{
		Enabled: true,
		Port:    port,
		Host:    "127.0.0.1",
	}

	profiler := New(config)

	// Test start
	err := profiler.Start()
	if err != nil {
		t.Fatalf("Failed to start profiler: %v", err)
	}

	if !profiler.IsRunning() {
		t.Error("Profiler should be running after Start()")
	}

	// Give the server a moment to fully start
	time.Sleep(200 * time.Millisecond)

	// Test that the server is actually listening
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/debug/pprof/", port))
	if err != nil {
		t.Fatalf("Failed to connect to profiler server: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	// Test stop
	err = profiler.Stop()
	if err != nil {
		t.Fatalf("Failed to stop profiler: %v", err)
	}

	if profiler.IsRunning() {
		t.Error("Profiler should not be running after Stop()")
	}
}

func TestServerStartDisabled(t *testing.T) {
	config := Config{
		Enabled: false,
		Port:    9091,
		Host:    "localhost",
	}

	profiler := New(config)

	err := profiler.Start()
	if err != nil {
		t.Fatalf("Start() should not fail when profiler is disabled: %v", err)
	}

	if profiler.IsRunning() {
		t.Error("Profiler should not be running when disabled")
	}
}

func TestGetAddress(t *testing.T) {
	config := Config{
		Enabled: true,
		Port:    8081,
		Host:    "127.0.0.1",
	}

	profiler := New(config)
	address := profiler.GetAddress()
	expected := "127.0.0.1:8081"

	if address != expected {
		t.Errorf("Expected address %s, got %s", expected, address)
	}
}

func TestGetURL(t *testing.T) {
	config := Config{
		Enabled: true,
		Port:    8082,
		Host:    "localhost",
	}

	profiler := New(config)
	url := profiler.GetURL()
	expected := "http://localhost:8082/debug/pprof/"

	if url != expected {
		t.Errorf("Expected URL %s, got %s", expected, url)
	}
}

func TestGetAddressDisabled(t *testing.T) {
	config := Config{
		Enabled: false,
		Port:    8083,
		Host:    "localhost",
	}

	profiler := New(config)
	address := profiler.GetAddress()

	if address != "" {
		t.Errorf("Expected empty address when disabled, got %s", address)
	}
}

func TestGetURLDisabled(t *testing.T) {
	config := Config{
		Enabled: false,
		Port:    8084,
		Host:    "localhost",
	}

	profiler := New(config)
	url := profiler.GetURL()

	if url != "" {
		t.Errorf("Expected empty URL when disabled, got %s", url)
	}
}

func TestMultipleStartCalls(t *testing.T) {
	port := getFreeTCPPort(t)
	config := Config{
		Enabled: true,
		Port:    port,
		Host:    "127.0.0.1",
	}

	profiler := New(config)

	// First start should succeed
	err := profiler.Start()
	if err != nil {
		t.Fatalf("First Start() failed: %v", err)
	}

	// Second start should not fail (should be a no-op)
	err = profiler.Start()
	if err != nil {
		t.Fatalf("Second Start() failed: %v", err)
	}

	if !profiler.IsRunning() {
		t.Error("Profiler should still be running after multiple Start() calls")
	}

	// Cleanup
	err = profiler.Stop()
	if err != nil {
		t.Fatalf("Failed to stop profiler: %v", err)
	}
}

func TestStopWithoutStart(t *testing.T) {
	config := Config{
		Enabled: true,
		Port:    9093,
		Host:    "localhost",
	}

	profiler := New(config)

	// Calling Stop() without Start() should not fail
	err := profiler.Stop()
	if err != nil {
		t.Fatalf("Stop() without Start() should not fail: %v", err)
	}

	if profiler.IsRunning() {
		t.Error("Profiler should not be running")
	}
}

func TestIsValidProfileType(t *testing.T) {
	tests := []struct {
		profileType ProfileType
		expected    bool
	}{
		{ProfileTypeCPU, true},
		{ProfileTypeHeap, true},
		{ProfileTypeAllocs, true},
		{ProfileTypeGoroutine, true},
		{ProfileTypeBlock, true},
		{ProfileTypeMutex, true},
		{ProfileTypeThreadCreate, true},
		{ProfileTypeTrace, true},
		{"invalid", false},
		{"", false},
	}

	for _, test := range tests {
		result := IsValidProfileType(test.profileType)
		if result != test.expected {
			t.Errorf("IsValidProfileType(%s) = %v, expected %v", test.profileType, result, test.expected)
		}
	}
}

func TestParseProfileType(t *testing.T) {
	tests := []struct {
		input       string
		expected    ProfileType
		expectError bool
	}{
		{"cpu", ProfileTypeCPU, false},
		{"CPU", ProfileTypeCPU, false},
		{"heap", ProfileTypeHeap, false},
		{"HEAP", ProfileTypeHeap, false},
		{"allocs", ProfileTypeAllocs, false},
		{"goroutine", ProfileTypeGoroutine, false},
		{"block", ProfileTypeBlock, false},
		{"mutex", ProfileTypeMutex, false},
		{"threadcreate", ProfileTypeThreadCreate, false},
		{"trace", ProfileTypeTrace, false},
		{"invalid", "", true},
		{"", "", true},
	}

	for _, test := range tests {
		result, err := ParseProfileType(test.input)

		if test.expectError && err == nil {
			t.Errorf("ParseProfileType(%s) expected error, but got none", test.input)
			continue
		}

		if !test.expectError && err != nil {
			t.Errorf("ParseProfileType(%s) unexpected error: %v", test.input, err)
			continue
		}

		if !test.expectError && result != test.expected {
			t.Errorf("ParseProfileType(%s) = %s, expected %s", test.input, result, test.expected)
		}
	}
}

func TestGetSupportedProfileTypes(t *testing.T) {
	types := GetSupportedProfileTypes()
	expectedCount := 8 // CPU, Heap, Allocs, Goroutine, Block, Mutex, ThreadCreate, Trace

	if len(types) != expectedCount {
		t.Errorf("Expected %d profile types, got %d", expectedCount, len(types))
	}

	// Check that all types are valid
	for _, profileType := range types {
		if !IsValidProfileType(profileType) {
			t.Errorf("GetSupportedProfileTypes() returned invalid type: %s", profileType)
		}
	}
}

func TestFileBasedProfilingCPU(t *testing.T) {
	tempDir := t.TempDir()

	profileFile := filepath.Join(tempDir, "cpu.prof")
	config := Config{
		Enabled:     true,
		File:        profileFile,
		ProfileType: ProfileTypeCPU,
	}

	profiler := New(config)

	// Start profiling
	err := profiler.Start()
	if err != nil {
		t.Fatalf("Failed to start CPU profiling: %v", err)
	}

	if !profiler.IsRunning() {
		t.Error("Profiler should be running")
	}

	// Let it run for a short time
	time.Sleep(100 * time.Millisecond)

	// Stop profiling
	err = profiler.Stop()
	if err != nil {
		t.Fatalf("Failed to stop CPU profiling: %v", err)
	}

	if profiler.IsRunning() {
		t.Error("Profiler should not be running after stop")
	}

	// Check that the profile file was created
	if _, err := os.Stat(profileFile); os.IsNotExist(err) {
		t.Errorf("Profile file was not created: %s", profileFile)
	}
}

func TestFileBasedProfilingHeap(t *testing.T) {
	tempDir := t.TempDir()

	profileFile := filepath.Join(tempDir, "heap.prof")
	config := Config{
		Enabled:     true,
		File:        profileFile,
		ProfileType: ProfileTypeHeap,
	}

	profiler := New(config)

	// Start profiling
	err := profiler.Start()
	if err != nil {
		t.Fatalf("Failed to start heap profiling: %v", err)
	}

	// Let it run for a short time
	time.Sleep(100 * time.Millisecond)

	// Stop profiling
	err = profiler.Stop()
	if err != nil {
		t.Fatalf("Failed to stop heap profiling: %v", err)
	}

	// Check that the profile file was created
	if _, err := os.Stat(profileFile); os.IsNotExist(err) {
		t.Errorf("Profile file was not created: %s", profileFile)
	}
}

func TestFileBasedProfilingTrace(t *testing.T) {
	tempDir := t.TempDir()

	profileFile := filepath.Join(tempDir, "trace.out")
	config := Config{
		Enabled:     true,
		File:        profileFile,
		ProfileType: ProfileTypeTrace,
	}

	profiler := New(config)

	// Start profiling
	err := profiler.Start()
	if err != nil {
		t.Fatalf("Failed to start trace profiling: %v", err)
	}

	// Let it run for a short time
	time.Sleep(100 * time.Millisecond)

	// Stop profiling
	err = profiler.Stop()
	if err != nil {
		t.Fatalf("Failed to stop trace profiling: %v", err)
	}

	// Check that the profile file was created
	if _, err := os.Stat(profileFile); os.IsNotExist(err) {
		t.Errorf("Profile file was not created: %s", profileFile)
	}
}

func TestFileBasedProfilingInvalidType(t *testing.T) {
	tempDir := t.TempDir()

	profileFile := filepath.Join(tempDir, "invalid.prof")
	config := Config{
		Enabled:     true,
		File:        profileFile,
		ProfileType: "invalid",
	}

	profiler := New(config)

	// Start profiling should fail
	err := profiler.Start()
	if err == nil {
		t.Fatal("Expected error for invalid profile type, but got none")
	}

	if profiler.IsRunning() {
		t.Error("Profiler should not be running with invalid profile type")
	}
}

func TestDefaultProfileType(t *testing.T) {
	tempDir := t.TempDir()

	profileFile := filepath.Join(tempDir, "default.prof")
	config := Config{
		Enabled: true,
		File:    profileFile,
		// ProfileType intentionally omitted to test default
	}

	profiler := New(config)

	// Start profiling (should default to CPU)
	err := profiler.Start()
	if err != nil {
		t.Fatalf("Failed to start profiling with default type: %v", err)
	}

	// Let it run for a short time
	time.Sleep(100 * time.Millisecond)

	// Stop profiling
	err = profiler.Stop()
	if err != nil {
		t.Fatalf("Failed to stop profiling: %v", err)
	}

	// Check that the profile file was created
	if _, err := os.Stat(profileFile); os.IsNotExist(err) {
		t.Errorf("Profile file was not created: %s", profileFile)
	}
}

func TestConfigWithAllFields(t *testing.T) {
	config := Config{
		Enabled:     true,
		Port:        8080,
		Host:        "0.0.0.0",
		File:        "/tmp/test.prof",
		ProfileType: ProfileTypeHeap,
	}

	profiler := New(config)
	if profiler == nil {
		t.Fatal("New() returned nil profiler")
	}

	// Verify all fields are set correctly
	if profiler.config.Enabled != config.Enabled {
		t.Errorf("Expected enabled %v, got %v", config.Enabled, profiler.config.Enabled)
	}
	if profiler.config.Port != config.Port {
		t.Errorf("Expected port %d, got %d", config.Port, profiler.config.Port)
	}
	if profiler.config.Host != config.Host {
		t.Errorf("Expected host %s, got %s", config.Host, profiler.config.Host)
	}
	if profiler.config.File != config.File {
		t.Errorf("Expected file %s, got %s", config.File, profiler.config.File)
	}
	if profiler.config.ProfileType != config.ProfileType {
		t.Errorf("Expected profile type %s, got %s", config.ProfileType, profiler.config.ProfileType)
	}
}

func TestConfigSerialization(t *testing.T) {
	port := getFreeTCPPort(t)
	originalConfig := Config{
		Enabled:     true,
		Port:        port,
		Host:        "127.0.0.1",
		File:        "test.prof",
		ProfileType: ProfileTypeTrace,
	}

	// Test JSON serialization/deserialization
	t.Run("JSON", func(t *testing.T) {
		jsonData, err := json.Marshal(originalConfig)
		if err != nil {
			t.Fatalf("Failed to marshal config to JSON: %v", err)
		}

		var config Config
		err = json.Unmarshal(jsonData, &config)
		if err != nil {
			t.Fatalf("Failed to unmarshal config from JSON: %v", err)
		}

		if !reflect.DeepEqual(originalConfig, config) {
			t.Errorf("Config mismatch after JSON round-trip.\nOriginal: %+v\nResult: %+v", originalConfig, config)
		}
	})

	// Test YAML serialization/deserialization
	t.Run("YAML", func(t *testing.T) {
		yamlData, err := yaml.Marshal(originalConfig)
		if err != nil {
			t.Fatalf("Failed to marshal config to YAML: %v", err)
		}

		var config Config
		err = yaml.Unmarshal(yamlData, &config)
		if err != nil {
			t.Fatalf("Failed to unmarshal config from YAML: %v", err)
		}

		if !reflect.DeepEqual(originalConfig, config) {
			t.Errorf("Config mismatch after YAML round-trip.\nOriginal: %+v\nResult: %+v", originalConfig, config)
		}
	})
}

func TestConfigWithProfileTypeString(t *testing.T) {
	// Test that ProfileType can be set from string (for environment variable support)
	tests := []struct {
		name        string
		profileType string
		expected    ProfileType
		expectError bool
	}{
		{"CPU", "cpu", ProfileTypeCPU, false},
		{"Heap", "heap", ProfileTypeHeap, false},
		{"Allocs", "allocs", ProfileTypeAllocs, false},
		{"Goroutine", "goroutine", ProfileTypeGoroutine, false},
		{"Block", "block", ProfileTypeBlock, false},
		{"Mutex", "mutex", ProfileTypeMutex, false},
		{"ThreadCreate", "threadcreate", ProfileTypeThreadCreate, false},
		{"Trace", "trace", ProfileTypeTrace, false},
		{"Invalid", "invalid", "", true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Simulate how environment variable would be processed
			parsed, err := ParseProfileType(test.profileType)
			if test.expectError {
				if err == nil {
					t.Errorf("Expected error for profile type %s, but got none", test.profileType)
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error for profile type %s: %v", test.profileType, err)
				return
			}

			if parsed != test.expected {
				t.Errorf("Expected profile type %s, got %s", test.expected, parsed)
			}

			// Create config with parsed type
			config := Config{
				Enabled:     true,
				File:        "test.prof",
				ProfileType: parsed,
			}

			profiler := New(config)
			if profiler.config.ProfileType != test.expected {
				t.Errorf("Config profile type mismatch: expected %s, got %s", test.expected, profiler.config.ProfileType)
			}
		})
	}
}

func TestAllProfileTypesFileCreation(t *testing.T) {
	tempDir := t.TempDir()

	profileTypes := GetSupportedProfileTypes()

	for _, profileType := range profileTypes {
		t.Run(string(profileType), func(t *testing.T) {
			var fileExt string
			if profileType == ProfileTypeTrace {
				fileExt = ".out"
			} else {
				fileExt = ".prof"
			}

			profileFile := filepath.Join(tempDir, string(profileType)+fileExt)
			config := Config{
				Enabled:     true,
				File:        profileFile,
				ProfileType: profileType,
			}

			profiler := New(config)

			// Start profiling
			err := profiler.Start()
			if err != nil {
				t.Fatalf("Failed to start %s profiling: %v", profileType, err)
			}

			if !profiler.IsRunning() {
				t.Errorf("Profiler should be running for %s", profileType)
			}

			// Let it run for a short time
			time.Sleep(50 * time.Millisecond)

			// Stop profiling
			err = profiler.Stop()
			if err != nil {
				t.Fatalf("Failed to stop %s profiling: %v", profileType, err)
			}

			if profiler.IsRunning() {
				t.Errorf("Profiler should not be running after stop for %s", profileType)
			}

			// Check that the profile file was created
			if _, err := os.Stat(profileFile); os.IsNotExist(err) {
				t.Errorf("Profile file was not created for %s: %s", profileType, profileFile)
			}
		})
	}
}

func TestProfileTypeDefaultHandling(t *testing.T) {
	tempDir := t.TempDir()

	// Test with empty ProfileType (should default to CPU)
	profileFile := filepath.Join(tempDir, "default.prof")
	config := Config{
		Enabled: true,
		File:    profileFile,
		// ProfileType intentionally left empty
	}

	profiler := New(config)

	// The profiler should internally default to CPU when ProfileType is empty
	err := profiler.Start()
	if err != nil {
		t.Fatalf("Failed to start profiling with empty ProfileType: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	err = profiler.Stop()
	if err != nil {
		t.Fatalf("Failed to stop profiling: %v", err)
	}

	// Check that the profile file was created
	if _, err := os.Stat(profileFile); os.IsNotExist(err) {
		t.Errorf("Profile file was not created: %s", profileFile)
	}
}

func TestConfiguredProfilerDisabled(t *testing.T) {
	config := Config{
		Enabled:     false,
		Port:        8080,
		Host:        "localhost",
		File:        "disabled.prof",
		ProfileType: ProfileTypeHeap,
	}

	profiler := New(config)

	// Even with file configured, disabled profiler should not start
	err := profiler.Start()
	if err != nil {
		t.Fatalf("Start() should not fail when profiler is disabled: %v", err)
	}

	if profiler.IsRunning() {
		t.Error("Profiler should not be running when disabled")
	}

	// File should not be created
	if _, err := os.Stat(config.File); !os.IsNotExist(err) {
		t.Error("Profile file should not be created when profiler is disabled")
	}
}

func TestProfilerStateResetOnErrors(t *testing.T) {
	tempDir := t.TempDir()

	// Create a read-only directory to force file creation failure
	readOnlyDir := filepath.Join(tempDir, "readonly")
	if err := os.Mkdir(readOnlyDir, 0o444); err != nil {
		t.Fatalf("Failed to create read-only dir: %v", err)
	}

	// On Windows, we need to use a different approach to create a permission failure
	// since Windows doesn't respect Unix-style directory permissions the same way
	var invalidPath string
	if runtime.GOOS == "windows" {
		// Use a non-existent deeply nested path that will fail on Windows
		invalidPath = filepath.Join(readOnlyDir, "nonexistent", "deeply", "nested", "path", "test.prof")
	} else {
		invalidPath = filepath.Join(readOnlyDir, "test.prof")
	}

	config := Config{
		Enabled:     true,
		File:        invalidPath,
		ProfileType: ProfileTypeHeap,
	}

	profiler := New(config)

	// This should fail due to permission error or invalid path
	err := profiler.Start()
	if err == nil {
		t.Error("Start() should fail when unable to create profile file")
	}

	// Profiler should not be running after failure
	if profiler.IsRunning() {
		t.Error("Profiler should not be running after start failure")
	}

	// Change to a valid directory and ensure it can start again
	config.File = filepath.Join(tempDir, "valid.prof")
	profiler.config = config

	err = profiler.Start()
	if err != nil {
		t.Fatalf("Start() should succeed with valid file path: %v", err)
	}

	if !profiler.IsRunning() {
		t.Error("Profiler should be running after successful start")
	}

	// Stop should succeed and reset state properly
	err = profiler.Stop()
	if err != nil {
		t.Fatalf("Stop() should succeed: %v", err)
	}

	if profiler.IsRunning() {
		t.Error("Profiler should not be running after stop")
	}

	// Should be able to start again after stop
	err = profiler.Start()
	if err != nil {
		t.Fatalf("Start() should succeed after previous stop: %v", err)
	}

	// Clean up
	profiler.Stop()
}
