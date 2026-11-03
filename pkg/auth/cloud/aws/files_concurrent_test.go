package aws

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth/types"
)

func TestWithFileLock_ContentionHonorsDeadline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials")
	require.NoError(t, withFileLock(context.Background(), path, func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
		defer cancel()

		called := false
		err := withFileLock(ctx, path, func() error {
			called = true
			return nil
		})
		assert.ErrorIs(t, err, ErrFileLockTimeout)
		assert.ErrorIs(t, err, context.DeadlineExceeded)
		assert.False(t, called, "a competing writer must not run while the lock is held")
		return nil
	}))

	// A failed callback must also release the lock for the next writer.
	err := withFileLock(context.Background(), path, func() error { return ErrWriteConfigFile })
	require.ErrorIs(t, err, ErrWriteConfigFile)
	require.NotErrorIs(t, err, ErrFileLockTimeout)
	require.NoError(t, withFileLock(context.Background(), path, func() error { return nil }))
}

func TestAWSFileManager_ConcurrentWritesPreserveProfiles(t *testing.T) {
	const writers = 12
	baseDir := t.TempDir()
	start := make(chan struct{})
	results := make(chan error, writers)
	for i := range writers {
		go func() {
			<-start
			// Independent managers share the provider files, as isolated auth does.
			manager := &AWSFileManager{baseDir: baseDir, realm: "test"}
			profile := fmt.Sprintf("profile-%d", i)
			creds := &types.AWSCredentials{
				AccessKeyID:     "key-" + profile,
				SecretAccessKey: "secret-" + profile,
				SessionToken:    "token-" + profile,
			}
			if err := manager.WriteCredentials("provider", profile, creds); err != nil {
				results <- err
				return
			}
			results <- manager.WriteConfig("provider", profile, "us-east-1", "json")
		}()
	}
	close(start)
	for range writers {
		assert.NoError(t, <-results)
	}

	manager := &AWSFileManager{baseDir: baseDir, realm: "test"}
	credentials, err := LoadINIFile(manager.GetCredentialsPath("provider"))
	require.NoError(t, err)
	config, err := LoadINIFile(manager.GetConfigPath("provider"))
	require.NoError(t, err)
	for i := range writers {
		profile := fmt.Sprintf("profile-%d", i)
		section, sectionErr := credentials.GetSection(profile)
		require.NoError(t, sectionErr)
		assert.Equal(t, "key-"+profile, section.Key("aws_access_key_id").String())
		assert.Equal(t, "secret-"+profile, section.Key("aws_secret_access_key").String())
		assert.Equal(t, "token-"+profile, section.Key("aws_session_token").String())
		section, sectionErr = config.GetSection("profile " + profile)
		require.NoError(t, sectionErr)
		assert.Equal(t, "us-east-1", section.Key("region").String())
		assert.Equal(t, "json", section.Key("output").String())
	}
}
