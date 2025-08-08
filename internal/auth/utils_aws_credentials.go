package auth

import (
	"fmt"
	"os"
	"path/filepath"
)

func WriteAwsCredentials(profile string, AccessKeyId string, SecretAccessKey string, SessionToken string) error {
	// Resolve home directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		panic(fmt.Errorf("failed to get user home directory: %w", err))
	}

	// Path to ~/.aws/credentials
	awsCredentialsPath := filepath.Join(homeDir, ".aws", "credentials")
	content := fmt.Sprintf(`
[%s]
aws_access_key_id=%s
aws_secret_access_key=%s
aws_session_token=%s
`, profile, AccessKeyId, SecretAccessKey, SessionToken)

	err = os.WriteFile(awsCredentialsPath, []byte(content), 0600)
	if err != nil {
		return fmt.Errorf("failed to write credentials file: %w", err)
	}
	return nil
}
