package profiler

import (
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

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
	config := Config{
		Enabled: true,
		Port:    9090,
		Host:    "localhost",
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
	resp, err := http.Get("http://localhost:9090/debug/pprof/")
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
	config := Config{
		Enabled: true,
		Port:    9092,
		Host:    "localhost",
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
		if test.expectError {
			if err == nil {
				t.Errorf("ParseProfileType(%s) expected error, but got none", test.input)
			}
		} else {
			if err != nil {
				t.Errorf("ParseProfileType(%s) unexpected error: %v", test.input, err)
			}
			if result != test.expected {
				t.Errorf("ParseProfileType(%s) = %s, expected %s", test.input, result, test.expected)
			}
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
	tempDir, err := ioutil.TempDir("", "profiler_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	profileFile := filepath.Join(tempDir, "cpu.prof")
	config := Config{
		Enabled:     true,
		File:        profileFile,
		ProfileType: ProfileTypeCPU,
	}

	profiler := New(config)

	// Start profiling
	err = profiler.Start()
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
	tempDir, err := ioutil.TempDir("", "profiler_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	profileFile := filepath.Join(tempDir, "heap.prof")
	config := Config{
		Enabled:     true,
		File:        profileFile,
		ProfileType: ProfileTypeHeap,
	}

	profiler := New(config)

	// Start profiling
	err = profiler.Start()
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
	tempDir, err := ioutil.TempDir("", "profiler_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	profileFile := filepath.Join(tempDir, "trace.out")
	config := Config{
		Enabled:     true,
		File:        profileFile,
		ProfileType: ProfileTypeTrace,
	}

	profiler := New(config)

	// Start profiling
	err = profiler.Start()
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
	tempDir, err := ioutil.TempDir("", "profiler_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	profileFile := filepath.Join(tempDir, "invalid.prof")
	config := Config{
		Enabled:     true,
		File:        profileFile,
		ProfileType: "invalid",
	}

	profiler := New(config)

	// Start profiling should fail
	err = profiler.Start()
	if err == nil {
		t.Fatal("Expected error for invalid profile type, but got none")
	}

	if profiler.IsRunning() {
		t.Error("Profiler should not be running with invalid profile type")
	}
}

func TestDefaultProfileType(t *testing.T) {
	tempDir, err := ioutil.TempDir("", "profiler_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	profileFile := filepath.Join(tempDir, "default.prof")
	config := Config{
		Enabled: true,
		File:    profileFile,
		// ProfileType intentionally omitted to test default
	}

	profiler := New(config)

	// Start profiling (should default to CPU)
	err = profiler.Start()
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
