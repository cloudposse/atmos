package marketplace

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseSource(t *testing.T) {
	tests := []struct {
		name          string
		source        string
		wantType      string
		wantOwner     string
		wantRepo      string
		wantRef       string
		wantURL       string
		wantFullPath  string
		wantAgentName string
		wantErr       bool
	}{
		{
			name:          "github short format",
			source:        "github.com/cloudposse/atmos-agent-terraform",
			wantType:      "github",
			wantOwner:     "cloudposse",
			wantRepo:      "atmos-agent-terraform",
			wantRef:       "",
			wantURL:       "https://github.com/cloudposse/atmos-agent-terraform.git",
			wantFullPath:  "github.com/cloudposse/atmos-agent-terraform",
			wantAgentName: "atmos-agent-terraform",
			wantErr:       false,
		},
		{
			name:          "github with version tag",
			source:        "github.com/cloudposse/atmos-agent-terraform@v1.2.3",
			wantType:      "github",
			wantOwner:     "cloudposse",
			wantRepo:      "atmos-agent-terraform",
			wantRef:       "v1.2.3",
			wantURL:       "https://github.com/cloudposse/atmos-agent-terraform.git",
			wantFullPath:  "github.com/cloudposse/atmos-agent-terraform",
			wantAgentName: "atmos-agent-terraform",
			wantErr:       false,
		},
		{
			name:          "github with branch",
			source:        "github.com/cloudposse/atmos-agent-terraform@main",
			wantType:      "github",
			wantOwner:     "cloudposse",
			wantRepo:      "atmos-agent-terraform",
			wantRef:       "main",
			wantURL:       "https://github.com/cloudposse/atmos-agent-terraform.git",
			wantFullPath:  "github.com/cloudposse/atmos-agent-terraform",
			wantAgentName: "atmos-agent-terraform",
			wantErr:       false,
		},
		{
			name:          "https URL",
			source:        "https://github.com/cloudposse/atmos-agent-terraform.git",
			wantType:      "github",
			wantOwner:     "cloudposse",
			wantRepo:      "atmos-agent-terraform",
			wantRef:       "",
			wantURL:       "https://github.com/cloudposse/atmos-agent-terraform.git",
			wantFullPath:  "github.com/cloudposse/atmos-agent-terraform",
			wantAgentName: "atmos-agent-terraform",
			wantErr:       false,
		},
		{
			name:          "ssh URL",
			source:        "git@github.com:cloudposse/atmos-agent-terraform.git",
			wantType:      "github",
			wantOwner:     "cloudposse",
			wantRepo:      "atmos-agent-terraform",
			wantRef:       "",
			wantURL:       "https://github.com/cloudposse/atmos-agent-terraform.git",
			wantFullPath:  "github.com/cloudposse/atmos-agent-terraform",
			wantAgentName: "atmos-agent-terraform",
			wantErr:       false,
		},
		{
			name:    "invalid format",
			source:  "invalid-source",
			wantErr: true,
		},
		{
			name:    "empty source",
			source:  "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sourceInfo, err := ParseSource(tt.source)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, sourceInfo)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, sourceInfo)

			assert.Equal(t, tt.wantType, sourceInfo.Type)
			assert.Equal(t, tt.wantOwner, sourceInfo.Owner)
			assert.Equal(t, tt.wantRepo, sourceInfo.Repo)
			assert.Equal(t, tt.wantRef, sourceInfo.Ref)
			assert.Equal(t, tt.wantURL, sourceInfo.URL)
			assert.Equal(t, tt.wantFullPath, sourceInfo.FullPath)
			assert.Equal(t, tt.wantAgentName, sourceInfo.Name)
		})
	}
}
