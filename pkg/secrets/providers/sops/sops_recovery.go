package sops

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"filippo.io/age"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/secrets/providers"
)

var _ providers.KeyRecoverer = (*sopsProvider)(nil)

// RestoreRecipients recovers a missing creation rule without changing private key material.
// Existing rules and explicit recipients are preserved. A multi-identity source must be narrowed
// explicitly so recovery never grants access to unrelated private keys in a shared keyring.
func (p *sopsProvider) RestoreRecipients(basePath string) (*providers.KeygenResult, error) {
	defer perf.Track(nil, "providers.sopsProvider.RestoreRecipients")()

	if !isAgeSopsKind(p.kind) || p.ageRecipients != "" {
		return nil, nil
	}
	sink := p.recipientSinkPath(basePath)
	configured, err := p.hasRecipientRule(basePath, sink)
	if err != nil || configured {
		return nil, err
	}
	material, err := p.existingKeyMaterial()
	if err != nil {
		return nil, err
	}
	recipient, err := recoveryRecipient(material)
	if err != nil {
		return nil, err
	}
	if err := p.writeRecipient(basePath, sink, recipient); err != nil {
		return nil, err
	}
	return &providers.KeygenResult{
		Vault:   p.name,
		Kind:    p.kind,
		Summary: "Recovered the public recipient from existing age key material.",
		Outputs: []providers.KeygenOutput{{Label: "public recipient", Location: sink}},
		Public:  recipient,
	}, nil
}

// hasRecipientRule preserves rules matching the vault's template, including catch-all rules.
func (p *sopsProvider) hasRecipientRule(basePath, sink string) (bool, error) {
	existing, err := os.ReadFile(sink)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, errUtils.Build(ErrSopsRecipients).WithCause(err).Err()
	}
	_, rules, err := loadSopsCreationRules(existing)
	if err != nil {
		return false, err
	}
	pattern, err := p.recipientFilePattern(basePath, sink)
	if err != nil {
		return false, err
	}
	for _, rule := range rules.Content {
		matches, err := ruleMatchesTemplate(rule, pattern)
		if err != nil || matches {
			return matches, err
		}
	}
	return false, nil
}

// ruleMatchesTemplate recognizes explicit and catch-all creation rules without changing them.
func ruleMatchesTemplate(rule *yaml.Node, pattern string) (bool, error) {
	regex := mappingValue(rule, "path_regex")
	if regex == nil || regex.Value == "" {
		return true, nil
	}
	matches, err := regexp.MatchString(regex.Value, pattern)
	if err != nil {
		return false, errUtils.Build(ErrSopsRecipients).WithCause(err).Err()
	}
	return matches || regex.Value == fileTemplateToPathRegex(pattern), nil
}

// existingKeyMaterial reads the configured source without updating it.
func (p *sopsProvider) existingKeyMaterial() (string, error) {
	if p.ageKey != "" {
		return p.ageKey, nil
	}
	if p.ageKeyStore != "" {
		st, ok := p.stores[p.ageKeyStore]
		if !ok || st == nil {
			return "", ageKeyStoreErr(p.ageKeyStore, providers.ErrStoreNotFound)
		}
		stack, component, key := p.ageKeyStoreTriple()
		value, err := st.Get(stack, component, key)
		if err != nil {
			return "", ageKeyStoreErr(p.ageKeyStore, err)
		}
		material, ok := value.(string)
		if !ok || strings.TrimSpace(material) == "" {
			return "", ageKeyStoreErr(p.ageKeyStore, ErrSopsAgeKey)
		}
		return material, nil
	}
	path, err := p.privateKeySink()
	if err != nil {
		return "", err
	}
	material, err := os.ReadFile(path)
	if err != nil {
		return "", ageKeyFileErr(path, err)
	}
	return string(material), nil
}

// recoveryRecipient requires one distinct X25519 identity, avoiding implicit recipient expansion.
func recoveryRecipient(material string) (string, error) {
	identities, err := age.ParseIdentities(strings.NewReader(material))
	if err != nil {
		// Parse errors may contain private input; expose only the stable error and remediation.
		return "", errUtils.Build(ErrSopsAgeKey).WithHint("The configured key source must contain a valid age X25519 private identity.").Err()
	}
	recipients := make(map[string]struct{})
	for _, identity := range identities {
		id, ok := identity.(*age.X25519Identity)
		if !ok {
			return "", fmt.Errorf("%w: recipient recovery requires an age X25519 identity", ErrSopsAgeKey)
		}
		recipients[id.Recipient().String()] = struct{}{}
	}
	if len(recipients) != 1 {
		return "", errUtils.Build(ErrSopsAgeKey).
			WithCausef("recipient recovery requires one identity; the configured key source contains %d distinct identities", len(recipients)).
			WithHint("Set spec.age_key_file to a file containing the intended identity, or configure the intended public recipients explicitly with spec.age_recipients.").Err()
	}
	for recipient := range recipients {
		return recipient, nil
	}
	return "", ErrSopsAgeKey
}
