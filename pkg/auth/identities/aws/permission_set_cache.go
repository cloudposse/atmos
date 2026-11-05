package aws

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sso"

	"github.com/cloudposse/atmos/pkg/auth/cachepaths"
	"github.com/cloudposse/atmos/pkg/filesystem"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/xdg"
)

const (
	permissionSetAccountCacheTTL       = time.Hour
	permissionSetAccountCacheDirPerms  = 0o700
	permissionSetAccountCacheFilePerms = 0o600
)

// permissionSetAccountCache persists name resolution across command invocations.
// The session digest prevents reuse after changing SSO users or sessions without
// storing the bearer token. The TTL bounds staleness after an account is renamed.
type permissionSetAccountCache struct {
	AccountID   string    `json:"account_id"`
	SessionHash string    `json:"session_hash"`
	ExpiresAt   time.Time `json:"expires_at"`
}

// accountCachePath isolates account names by auth realm, region, and SSO endpoint.
// Replacing the entry on session changes avoids accumulating a file per login.
func (i *permissionSetIdentity) accountCachePath(client *sso.Client, accountName string) string {
	dir, err := xdg.GetXDGCacheDir(filepath.Join(cachepaths.AWSSSOSubdir, "account-ids"), permissionSetAccountCacheDirPerms)
	if err != nil {
		log.Debug("Failed to create SSO account cache directory", "error", err)
		return ""
	}
	options := client.Options()
	key := fmt.Sprintf("%q", []string{i.realm, options.Region, aws.ToString(options.BaseEndpoint), accountName})
	digest := sha256.Sum256([]byte(key))
	return filepath.Join(dir, fmt.Sprintf("%x.json", digest))
}

func loadPermissionSetAccountID(path, accessToken string) string {
	if path == "" {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var entry permissionSetAccountCache
	if err := json.Unmarshal(data, &entry); err != nil {
		return ""
	}
	sessionHash := fmt.Sprintf("%x", sha256.Sum256([]byte(accessToken)))
	if entry.SessionHash != sessionHash || !time.Now().Before(entry.ExpiresAt) {
		return ""
	}
	return entry.AccountID
}

func savePermissionSetAccountID(path, accessToken, accountID string) {
	if path == "" || accountID == "" {
		return
	}
	entry := permissionSetAccountCache{
		AccountID:   accountID,
		SessionHash: fmt.Sprintf("%x", sha256.Sum256([]byte(accessToken))),
		ExpiresAt:   time.Now().Add(permissionSetAccountCacheTTL),
	}
	data, err := json.Marshal(entry)
	if err != nil {
		log.Debug("Failed to encode SSO account cache", "error", err)
		return
	}
	if err := filesystem.NewOSFileSystem().WriteFileAtomic(path, data, permissionSetAccountCacheFilePerms); err != nil {
		log.Debug("Failed to write SSO account cache", "error", err)
	}
}
