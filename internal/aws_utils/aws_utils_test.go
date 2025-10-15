package aws_utils

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/tests"
)

func TestLoadAWSConfig(t *testing.T) {
	// Check for AWS profile precondition
	tests.RequireAWSProfile(t, "cplive-core-gbl-identity")
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
				t.Setenv("AWS_ACCESS_KEY_ID", "test-key")
				t.Setenv("AWS_SECRET_ACCESS_KEY", "test-secret")
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
				t.Setenv("AWS_ACCESS_KEY_ID", "test-key")
				t.Setenv("AWS_SECRET_ACCESS_KEY", "test-secret")
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
			// Clear AWS_PROFILE to prevent conflicts with local AWS configuration.
			t.Setenv("AWS_PROFILE", "")

			// Setup
			if tt.setupEnv != nil {
				tt.setupEnv()
			}

			// Cleanup
			if tt.cleanupEnv != nil {
				defer tt.cleanupEnv()
			}

			// Execute
			cfg, err := LoadAWSConfig(context.Background(), tt.region, tt.roleArn, time.Minute*15)

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
