package exec

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoadAWSConfig(t *testing.T) {
	tests := []struct {
		name       string
		region     string
		roleArn    string
		setupEnv   func()
		cleanupEnv func()
		wantErr    bool
	}{
		{
			name:    "basic config without region or role",
			region:  "",
			roleArn: "",
			setupEnv: func() {
				os.Setenv("AWS_ACCESS_KEY_ID", "test-key")
				os.Setenv("AWS_SECRET_ACCESS_KEY", "test-secret")
			},
			cleanupEnv: func() {
				os.Unsetenv("AWS_ACCESS_KEY_ID")
				os.Unsetenv("AWS_SECRET_ACCESS_KEY")
			},
			wantErr: false,
		},
		{
			name:    "config with custom region",
			region:  "us-east-2",
			roleArn: "",
			setupEnv: func() {
				os.Setenv("AWS_ACCESS_KEY_ID", "test-key")
				os.Setenv("AWS_SECRET_ACCESS_KEY", "test-secret")
			},
			cleanupEnv: func() {
				os.Unsetenv("AWS_ACCESS_KEY_ID")
				os.Unsetenv("AWS_SECRET_ACCESS_KEY")
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			if tt.setupEnv != nil {
				tt.setupEnv()
			}

			// Cleanup
			if tt.cleanupEnv != nil {
				defer tt.cleanupEnv()
			}

			// Execute
			cfg, err := loadAWSConfig(context.Background(), tt.region, tt.roleArn)

			// Assert
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tt.region != "" {
					assert.Equal(t, tt.region, cfg.Region)
				}
			}
		})
	}
}
