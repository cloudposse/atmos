package profiler

import (
	"net/http"
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
