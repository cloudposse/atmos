package auth

import (
	"bytes"
	"fmt"
	"github.com/cloudposse/atmos/pkg/config/go-homedir"
	"github.com/cloudposse/atmos/pkg/schema"
	"os"
	"path/filepath"
	"runtime"
	"time"

	log "github.com/charmbracelet/log"
	"gopkg.in/ini.v1"
)

const (
	AwsConfigFilePerm = 0644
)

func SetAwsEnvVars(info *schema.ConfigAndStacksInfo, profile, provider string) error {
	if profile == "" {
		return fmt.Errorf("profile is required")
	}
	info.ComponentEnvSection["AWS_PROFILE"] = profile
	configFilePath, err := GetAwsAtmosConfigFilepath(provider)
	if err != nil {
		return err
	}
	info.ComponentEnvSection["AWS_CONFIG_FILE"] = configFilePath //GetAwsAtmosConfigFilepath

	return nil
}

func WriteAwsCredentials(profile, accessKeyID, secretAccessKey, sessionToken, identity string) error {
	if profile == "" {
		return fmt.Errorf("profile is required")
	}

	targetPath, err := GetAwsCredentialsFilepath()
	if err != nil {
		return err
	}

	// Ensure parent dir exists
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o700); err != nil {
		return fmt.Errorf("ensure aws dir: %w", err)
	}

	// Load (or create) ini
	loadOpts := ini.LoadOptions{
		IgnoreInlineComment: true,
		Loose:               true, // don't error if file doesn't exist
	}
	var f *ini.File
	err = nil
	if _, statErr := os.Stat(targetPath); statErr == nil {
		f, err = ini.LoadSources(loadOpts, targetPath)
		if err != nil {
			return fmt.Errorf("load credentials ini: %w", err)
		}
	} else {
		f = ini.Empty()
	}

	// Update only this profile section
	// Note: for "default", section name is literally "default" (no "profile " prefix in credentials file)
	sec := f.Section(profile)
	sec.Key("aws_access_key_id").SetValue(accessKeyID)
	sec.Key("aws_secret_access_key").SetValue(secretAccessKey)
	sec.Key("aws_session_token").SetValue(sessionToken)
	// Some legacy consumers still look at aws_security_token; set it too.
	sec.Key("aws_security_token").SetValue(sessionToken)
	sec.Comment = fmt.Sprintf("atmos-auth [identity=%s] generated on %s (%s)", identity, time.Now().Format(time.RFC3339), runtime.GOOS)

	// Write atomically: dump to memory, write temp w/ 0600, then rename
	var buf bytes.Buffer
	if _, err := f.WriteTo(&buf); err != nil {
		return fmt.Errorf("serialize ini: %w", err)
	}

	tmp := targetPath + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0o600); err != nil {
		return fmt.Errorf("write temp credentials: %w", err)
	}
	// Best-effort fsync on POSIX (optional)
	if ftmp, err := os.OpenFile(tmp, os.O_RDWR, 0o600); err == nil {
		_ = ftmp.Sync()
		_ = ftmp.Close()
	}

	if err := os.Rename(tmp, targetPath); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("atomic rename credentials: %w", err)
	}

	log.Debug("Updated AWS credentials", "profile", profile, "path", targetPath)
	return nil
}

func GetAwsCredentialsFilepath() (string, error) {
	// Figure out where to write ~/.aws/credentials
	targetPath := os.Getenv("AWS_SHARED_CREDENTIALS_FILE")
	if targetPath == "" {
		homeDir, err := homedir.Dir()
		if err != nil {
			return "", fmt.Errorf("resolve home dir: %w", err)
		}
		targetPath = filepath.Join(homeDir, ".aws", "credentials")
	}
	return targetPath, nil
}

func GetAwsAtmosConfigFilepath(provider string) (string, error) {
	// Figure out where to write ~/.aws/credentials
	targetPath := os.Getenv("ATMOS_AWS_CONFIG_FILE")
	if targetPath == "" {
		homeDir, err := homedir.Dir()
		if err != nil {
			return "", fmt.Errorf("resolve home dir: %w", err)
		}
		targetPath = filepath.Join(homeDir, ".aws", "atmos", provider, "config")
		_ = os.MkdirAll(targetPath, AwsConfigFilePerm)

	}
	return targetPath, nil
}

// RemoveAwsCredentials removes a specific AWS profile from the credentials file.
// If the profile doesn't exist, this is a no-op.
func RemoveAwsCredentials(profile string) error {
	if profile == "" {
		return fmt.Errorf("profile is required")
	}

	// Determine credentials file path
	targetPath := os.Getenv("AWS_SHARED_CREDENTIALS_FILE")
	if targetPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("resolve home dir: %w", err)
		}
		targetPath = filepath.Join(home, ".aws", "credentials")
	}

	// Check if file exists first
	if _, statErr := os.Stat(targetPath); os.IsNotExist(statErr) {
		log.Debug("AWS credentials file doesn't exist, nothing to remove", "path", targetPath)
		return nil // No file exists, so nothing to remove
	}

	// Load credentials file
	loadOpts := ini.LoadOptions{
		IgnoreInlineComment: true,
		Loose:               true,
	}
	f, err := ini.LoadSources(loadOpts, targetPath)
	if err != nil {
		return fmt.Errorf("load credentials ini: %w", err)
	}

	// Check if profile exists
	if !f.HasSection(profile) {
		log.Debug("Profile not found in credentials file", "profile", profile, "path", targetPath)
		return nil // Profile doesn't exist, nothing to remove
	}

	// Remove the profile section
	f.DeleteSection(profile)
	log.Debug("Removed profile from AWS credentials", "profile", profile)

	// Write atomically: dump to memory, write temp w/ 0600, then rename
	var buf bytes.Buffer
	if _, err := f.WriteTo(&buf); err != nil {
		return fmt.Errorf("serialize ini: %w", err)
	}

	tmp := targetPath + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0o600); err != nil {
		return fmt.Errorf("write temp credentials: %w", err)
	}

	// Best-effort fsync on POSIX (optional)
	if ftmp, err := os.OpenFile(tmp, os.O_RDWR, 0o600); err == nil {
		_ = ftmp.Sync()
		_ = ftmp.Close()
	}

	if err := os.Rename(tmp, targetPath); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("atomic rename credentials: %w", err)
	}

	log.Info("Removed AWS credentials", "profile", profile, "path", targetPath)
	return nil
}
