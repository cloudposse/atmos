package rc

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"sync"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// managedFingerprints records provenance only for live files generated in this process. Keeping
// the original digest prevents edits/replacements of a generated file from being hidden by its
// stable identity. Entries are removed with the file's pipeline-scoped cleanup, including errors.
var managedFingerprints sync.Map

type managedFingerprint struct {
	content  [sha256.Size]byte
	identity string
}

// GenerateManaged renders a CLI config containing an Atmos-owned ephemeral proxy, while recording
// its stable semantic identity for smart init. User-controlled CLI config retains normal precedence
// and is never registered. A nil semanticMap uses ordinary content identity. Otherwise, semanticMap
// contains the same effective user settings and installation rules, with only the Atmos-generated
// endpoint replaced; identity covers cache location and trust.
func GenerateManaged(rcMap, semanticMap map[string]any, identity string, atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) ([]string, func() error, error) {
	defer perf.Track(atmosConfig, "rc.GenerateManaged")()

	if semanticMap == nil {
		return Generate(rcMap, atmosConfig, info)
	}

	if userControlsCLIConfig(atmosConfig, info) {
		return nil, noop, nil
	}
	semantic, err := Render(semanticMap)
	if err != nil {
		return nil, noop, err
	}
	rendered, err := Render(rcMap)
	if err != nil {
		return nil, noop, err
	}
	env, cleanup, err := Generate(rcMap, atmosConfig, info)
	if err != nil {
		return env, cleanup, err
	}
	var path string
	for _, entry := range env {
		if p, ok := strings.CutPrefix(entry, EnvKeyTFCLIConfigFile+"="); ok {
			path = p
			break
		}
	}
	if path == "" {
		return env, cleanup, nil
	}
	semanticIdentity := fmt.Sprintf("atmos-managed-rc-v1:%x", sha256.Sum256(append(semantic, []byte("\n"+identity)...)))
	managedFingerprints.Store(path, managedFingerprint{content: sha256.Sum256(rendered), identity: semanticIdentity})
	return env, func() error { managedFingerprints.Delete(path); return cleanup() }, nil
}

// FingerprintContent substitutes a stable identity only for an unchanged, explicitly registered
// generated file. Arbitrary user files, including localhost mirrors, retain their exact content.
func FingerprintContent(path string, content []byte) string {
	defer perf.Track(nil, "rc.FingerprintContent")()

	if v, ok := managedFingerprints.Load(path); ok {
		record := v.(managedFingerprint)
		if record.content == sha256.Sum256(content) {
			return record.identity
		}
	}
	return string(content)
}
