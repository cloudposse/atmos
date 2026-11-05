package credential

import (
	"regexp"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
)

var environmentName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

const (
	maxStringCredentials    = 256
	maxCredentialKeyBytes   = 256
	maxCredentialValueBytes = 1 << 20
)

func decodeStrings(envelope *pluginv1.Credentials) (types.ICredentials, error) {
	data := envelope.GetData().AsMap()
	if len(data) == 0 || len(data) > maxStringCredentials {
		return nil, errUtils.ErrPluginProtocol
	}
	keys := make([]string, 0, len(data))
	seen := make(map[string]bool)
	for key, raw := range data {
		if !validStringCredential(key, raw) {
			return nil, errUtils.ErrPluginProtocol
		}
		if envelope.GetType() == "environment/v1" {
			canonical := strings.ToUpper(key)
			if !environmentName.MatchString(key) || seen[canonical] {
				return nil, errUtils.ErrPluginProtocol
			}
			seen[canonical] = true
		}
		keys = append(keys, key)
	}
	return &types.PluginCredentials{Type: envelope.GetType(), ExpiresAt: envelope.GetExpiresAt().AsTime(), Data: data, SensitiveKeys: keys}, nil
}

func validStringCredential(key string, raw any) bool {
	value, ok := raw.(string)
	return ok && key != "" && len(key) <= maxCredentialKeyBytes && !strings.ContainsRune(value, 0) && len(value) <= maxCredentialValueBytes
}
