package downloader

import (
	"testing"
)

func TestMaskBasicAuth_WithUsernameAndPassword(t *testing.T) {
	input := "https://user:secret@github.com/path?query=1"
	expected := "https://***@github.com/path?query=1"
	masked, err := maskBasicAuth(input)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if masked != expected {
		t.Errorf("Expected masked URL %q, got %q", expected, masked)
	}
}

func TestMaskBasicAuth_WithUsernameOnly(t *testing.T) {
	input := "https://user@github.com/path"
	expected := "https://***@github.com/path"
	masked, err := maskBasicAuth(input)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if masked != expected {
		t.Errorf("Expected masked URL %q, got %q", expected, masked)
	}
}

func TestMaskBasicAuth_NoCredentials(t *testing.T) {
	input := "https://github.com/path"
	expected := "https://github.com/path"
	masked, err := maskBasicAuth(input)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if masked != expected {
		t.Errorf("Expected masked URL %q, got %q", expected, masked)
	}
}

func TestMaskBasicAuth_InvalidURL(t *testing.T) {
	input := "://invalid-url"
	_, err := maskBasicAuth(input)
	if err == nil {
		t.Errorf("Expected error for invalid URL, but got none")
	}
}
