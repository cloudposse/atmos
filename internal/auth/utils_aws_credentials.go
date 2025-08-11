package auth

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	log "github.com/charmbracelet/log"
	"gopkg.in/ini.v1"
)

func WriteAwsCredentials(profile, accessKeyID, secretAccessKey, sessionToken, identity string) error {
	if profile == "" {
		return fmt.Errorf("profile is required")
	}

	// Figure out where to write ~/.aws/credentials
	targetPath := os.Getenv("AWS_SHARED_CREDENTIALS_FILE")
	if targetPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("resolve home dir: %w", err)
		}
		targetPath = filepath.Join(home, ".aws", "credentials")
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
	var err error
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
