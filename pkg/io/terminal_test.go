package io

import (
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestTerminal_ColorProfile(t *testing.T) {
	// Terminal color profile is determined at initialization based on TTY status.
	// Since tests may not run in a TTY, we just verify the method doesn't panic
	// and returns a valid profile. The actual color detection logic is tested
	// in context_test.go via Config.DetectColorProfile().
	cfg := &Config{}
	term := newTerminal(cfg)

	profile := term.ColorProfile()

	// Should return a valid profile
	if profile < ColorNone || profile > ColorTrue {
		t.Errorf("ColorProfile() returned invalid profile: %v", profile)
	}
}

func TestTerminal_IsTTY(t *testing.T) {
	cfg := &Config{}
	term := newTerminal(cfg)

	// Test all stream types (actual TTY detection depends on test environment)
	// We just verify the method doesn't panic
	_ = term.IsTTY(StreamInput)
	_ = term.IsTTY(StreamOutput)
	_ = term.IsTTY(StreamError)

	// Test invalid stream type
	if term.IsTTY(StreamType(999)) {
		t.Error("expected false for invalid stream type")
	}
}

func TestTerminal_Width(t *testing.T) {
	cfg := &Config{}
	term := newTerminal(cfg)

	// Width detection depends on terminal, just verify it doesn't panic
	width := term.Width(StreamOutput)
	if width < 0 {
		t.Errorf("Width() returned negative value: %d", width)
	}

	// Test invalid stream type
	width = term.Width(StreamType(999))
	if width != 0 {
		t.Errorf("expected 0 for invalid stream type, got %d", width)
	}
}

func TestTerminal_Height(t *testing.T) {
	cfg := &Config{}
	term := newTerminal(cfg)

	// Height detection depends on terminal, just verify it doesn't panic
	height := term.Height(StreamOutput)
	if height < 0 {
		t.Errorf("Height() returned negative value: %d", height)
	}

	// Test invalid stream type
	height = term.Height(StreamType(999))
	if height != 0 {
		t.Errorf("expected 0 for invalid stream type, got %d", height)
	}
}

func TestTerminal_SetTitle(t *testing.T) {
	tests := []struct {
		name   string
		config *Config
		title  string
	}{
		{
			name: "set title enabled",
			config: &Config{
				AtmosConfig: schema.AtmosConfiguration{
					Settings: schema.AtmosSettings{
						Terminal: schema.Terminal{
							Title: true,
						},
					},
				},
			},
			title: "Test Title",
		},
		{
			name: "set title disabled",
			config: &Config{
				AtmosConfig: schema.AtmosConfiguration{
					Settings: schema.AtmosSettings{
						Terminal: schema.Terminal{
							Title: false,
						},
					},
				},
			},
			title: "Should Not Set",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			term := newTerminal(tt.config)

			// Verify SetTitle doesn't panic
			term.SetTitle(tt.title)
			term.RestoreTitle()
		})
	}
}

func TestTerminal_Alert(t *testing.T) {
	tests := []struct {
		name   string
		config *Config
	}{
		{
			name: "alerts enabled",
			config: &Config{
				AtmosConfig: schema.AtmosConfiguration{
					Settings: schema.AtmosSettings{
						Terminal: schema.Terminal{
							Alerts: true,
						},
					},
				},
			},
		},
		{
			name: "alerts disabled",
			config: &Config{
				AtmosConfig: schema.AtmosConfiguration{
					Settings: schema.AtmosSettings{
						Terminal: schema.Terminal{
							Alerts: false,
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			term := newTerminal(tt.config)

			// Verify Alert doesn't panic
			term.Alert()
		})
	}
}
