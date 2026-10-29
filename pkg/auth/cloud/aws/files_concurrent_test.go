package aws

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth/types"
)

func TestAWSFileManagerConcurrentWritesPreserveEveryProfile(t *testing.T) {
	manager, err := NewAWSFileManager(t.TempDir(), "")
	require.NoError(t, err)
	const workers = 12
	start := make(chan struct{})
	results := make(chan error, workers*2)
	var wg sync.WaitGroup
	for i := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			name := fmt.Sprintf("profile-%d", i)
			results <- manager.WriteCredentials("provider", name, &types.AWSCredentials{
				AccessKeyID: "key-" + name, SecretAccessKey: "test-secret", SessionToken: "token-" + name,
			})
			results <- manager.WriteConfig("provider", name, "us-east-2", "json")
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	for err := range results {
		require.NoError(t, err)
	}
	credentials, err := LoadINIFile(manager.GetCredentialsPath("provider"))
	require.NoError(t, err)
	config, err := LoadINIFile(manager.GetConfigPath("provider"))
	require.NoError(t, err)
	for i := range workers {
		name := fmt.Sprintf("profile-%d", i)
		profile, err := credentials.GetSection(name)
		require.NoError(t, err)
		require.Equal(t, "key-"+name, profile.Key("aws_access_key_id").String())
		require.Equal(t, "token-"+name, profile.Key("aws_session_token").String())
		settings, err := config.GetSection("profile " + name)
		require.NoError(t, err)
		require.Equal(t, "us-east-2", settings.Key("region").String())
		require.Equal(t, "json", settings.Key("output").String())
	}
}
