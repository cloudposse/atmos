package ai

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestGetMemoryFilePath(t *testing.T) {
	tests := []struct {
		name         string
		atmosConfig  *schema.AtmosConfiguration
		expectedPath string
	}{
		{
			name: "default file path",
			atmosConfig: &schema.AtmosConfiguration{
				BasePath: "/test/project",
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						Memory: schema.AIMemorySettings{
							FilePath: "",
						},
					},
				},
			},
			expectedPath: "/test/project/ATMOS.md",
		},
		{
			name: "custom relative path",
			atmosConfig: &schema.AtmosConfiguration{
				BasePath: "/test/project",
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						Memory: schema.AIMemorySettings{
							FilePath: "docs/MEMORY.md",
						},
					},
				},
			},
			expectedPath: "/test/project/docs/MEMORY.md",
		},
		{
			name: "absolute path",
			atmosConfig: &schema.AtmosConfiguration{
				BasePath: "/test/project",
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						Memory: schema.AIMemorySettings{
							FilePath: "/absolute/path/ATMOS.md",
						},
					},
				},
			},
			expectedPath: "/absolute/path/ATMOS.md",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getMemoryFilePath(tt.atmosConfig)
			assert.Equal(t, tt.expectedPath, result)
		})
	}
}
