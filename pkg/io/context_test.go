package io

import (
	stdIo "io"
	"os"
	"regexp"
	"testing"

	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestNewContext(t *testing.T) {
	// Reset viper state
	viper.Reset()

	ctx, err := NewContext()
	if err != nil {
		t.Fatalf("NewContext() error = %v", err)
	}

	if ctx == nil {
		t.Fatal("NewContext() returned nil")
	}

	if ctx.Streams() == nil {
		t.Error("Streams() returned nil")
	}

	if ctx.Config() == nil {
		t.Error("Config() returned nil")
	}

	if ctx.Masker() == nil {
		t.Error("Masker() returned nil")
	}
}

func TestBuildConfig(t *testing.T) {
	// Reset viper state before test
	viper.Reset()

	// Test with minimal viper configuration
	viper.Set("redirect-stderr", "")
	viper.Set("disable-masking", false)

	cfg := buildConfig()

	if cfg == nil {
		t.Fatal("buildConfig() returned nil")
	}

	// Test with atmos.yaml config
	viper.Set("settings", schema.Settings{})

	cfg = buildConfig()

	if cfg == nil {
		t.Fatal("buildConfig() with atmos.yaml returned nil")
	}
}

func TestWithOptions(t *testing.T) {
	// Create test streams
	testInput := stdIo.NopCloser(os.Stdin)
	testOutput := os.Stdout
	testError := os.Stderr

	testStreams := &streams{
		input:  testInput,
		output: testOutput,
		error:  testError,
	}

	testMasker := &masker{
		enabled:  true,
		literals: make(map[string]bool),
		patterns: make([]*regexp.Regexp, 0),
	}

	tests := []struct {
		name string
		opts []ContextOption
	}{
		{
			name: "WithStreams option",
			opts: []ContextOption{WithStreams(testStreams)},
		},
		{
			name: "WithMasker option",
			opts: []ContextOption{WithMasker(testMasker)},
		},
		{
			name: "Multiple options",
			opts: []ContextOption{
				WithStreams(testStreams),
				WithMasker(testMasker),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, err := NewContext(tt.opts...)
			if err != nil {
				t.Fatalf("NewContext() with options error = %v", err)
			}

			if ctx == nil {
				t.Fatal("NewContext() with options returned nil")
			}
		})
	}
}

func TestStreamString(t *testing.T) {
	tests := []struct {
		name   string
		stream Stream
		want   string
	}{
		{
			name:   "DataStream returns 'data'",
			stream: DataStream,
			want:   "data",
		},
		{
			name:   "UIStream returns 'ui'",
			stream: UIStream,
			want:   "ui",
		},
		{
			name:   "Unknown stream returns 'unknown'",
			stream: Stream(999),
			want:   "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.stream.String()
			if got != tt.want {
				t.Errorf("Stream.String() = %v, want %v", got, tt.want)
			}
		})
	}
}
