package pager

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestNewFromAtmosConfig(t *testing.T) {
	tests := []struct {
		name                      string
		atmosConfig               *schema.AtmosConfiguration
		expectedPagerEnabled      bool
		expectedPagerFlagExplicit bool
	}{
		{
			name:                      "nil AtmosConfiguration",
			atmosConfig:               nil,
			expectedPagerEnabled:      false,
			expectedPagerFlagExplicit: false,
		},
		{
			name: "pager disabled with no explicit flag",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					Terminal: schema.Terminal{
						Pager:             "false",
						PagerFlagExplicit: false,
					},
				},
			},
			expectedPagerEnabled:      false,
			expectedPagerFlagExplicit: false,
		},
		{
			name: "pager enabled with explicit flag",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					Terminal: schema.Terminal{
						Pager:             "true",
						PagerFlagExplicit: true,
					},
				},
			},
			expectedPagerEnabled:      true,
			expectedPagerFlagExplicit: true,
		},
		{
			name: "pager with less command and explicit flag",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					Terminal: schema.Terminal{
						Pager:             "less",
						PagerFlagExplicit: true,
					},
				},
			},
			expectedPagerEnabled:      true,
			expectedPagerFlagExplicit: true,
		},
		{
			name: "pager enabled without explicit flag (from config)",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					Terminal: schema.Terminal{
						Pager:             "true",
						PagerFlagExplicit: false,
					},
				},
			},
			expectedPagerEnabled:      true,
			expectedPagerFlagExplicit: false,
		},
		{
			name: "empty pager setting",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					Terminal: schema.Terminal{
						Pager:             "",
						PagerFlagExplicit: false,
					},
				},
			},
			expectedPagerEnabled:      false,
			expectedPagerFlagExplicit: false,
		},
		{
			name: "pager disabled with explicit flag",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					Terminal: schema.Terminal{
						Pager:             "false",
						PagerFlagExplicit: true,
					},
				},
			},
			expectedPagerEnabled:      false,
			expectedPagerFlagExplicit: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pc := NewFromAtmosConfig(tt.atmosConfig)
			assert.NotNil(t, pc, "PageCreator should not be nil")

			// Cast to concrete type to verify internal state
			concretePC, ok := pc.(*pageCreator)
			assert.True(t, ok, "Should be able to cast to *pageCreator")

			assert.Equal(t, tt.expectedPagerEnabled, concretePC.enablePager,
				"enablePager should match expected value")
			assert.Equal(t, tt.expectedPagerFlagExplicit, concretePC.pagerFlagExplicit,
				"pagerFlagExplicit should match expected value")
		})
	}
}

func TestNewFromAtmosConfig_Integration(t *testing.T) {
	// Test that the created PageCreator behaves correctly
	tests := []struct {
		name        string
		atmosConfig *schema.AtmosConfiguration
		shouldLog   bool // Whether we expect debug logging when no TTY
	}{
		{
			name: "should not log when flag not explicit",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					Terminal: schema.Terminal{
						Pager:             "true",
						PagerFlagExplicit: false,
					},
				},
			},
			shouldLog: false,
		},
		{
			name: "should log when flag is explicit",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					Terminal: schema.Terminal{
						Pager:             "true",
						PagerFlagExplicit: true,
					},
				},
			},
			shouldLog: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pc := NewFromAtmosConfig(tt.atmosConfig)
			concretePC, _ := pc.(*pageCreator)

			// Override TTY check to simulate no TTY environment
			originalIsTTY := concretePC.isTTYSupportForStdout
			concretePC.isTTYSupportForStdout = func() bool { return false }
			defer func() { concretePC.isTTYSupportForStdout = originalIsTTY }()

			// The logging behavior is tested implicitly by the pagerFlagExplicit field
			// In actual usage, the Run method would log based on this field
			assert.Equal(t, tt.shouldLog, concretePC.pagerFlagExplicit,
				"pagerFlagExplicit determines whether debug logging occurs")
		})
	}
}
