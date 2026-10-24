package sops

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"filippo.io/age"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/secrets/providers"
)

const (
	// Permission for the age keys file (owner read/write only).
	keysFileMode os.FileMode = 0o600
	// Permission for directories holding the age keys file.
	keysDirMode os.FileMode = 0o700
	// Default recipients sink filename.
	defaultSopsConfigName = ".sops.yaml"
	// YAML scalar tag for string nodes written into `.sops.yaml`.
	yamlStrTag = "!!str"
)

// recipientsFileMode is the permission for a written `.sops.yaml` (world-readable: it holds only
// public recipients and is meant to be committed).
const recipientsFileMode os.FileMode = 0o644

// AgeIdentity is a freshly generated age key pair: the private identity decrypts, the public
// recipient encrypts.
type AgeIdentity struct {
	// Private is the age private identity (`AGE-SECRET-KEY-1…`).
	Private string
	// Recipient is the age public recipient (`age1…`).
	Recipient string
}

// Ensure the SOPS provider advertises the keygen capability.
var _ providers.KeyGenerator = (*sopsProvider)(nil)

// isAgeSopsKind reports whether a SOPS vault kind uses a locally-generatable age key. KMS/GPG
// kinds bring externally-managed keys that Atmos does not generate.
func isAgeSopsKind(kind string) bool {
	return kind == "" || kind == "sops" || kind == "sops/age" || strings.HasSuffix(kind, "/age")
}

// HasKey reports whether the vault already resolves a usable private key. For non-age SOPS kinds
// (KMS/GPG) the key is externally managed, so it reports true (Atmos has nothing to generate).
func (p *sopsProvider) HasKey() bool {
	defer perf.Track(nil, "providers.sopsProvider.HasKey")()

	if !isAgeSopsKind(p.kind) {
		return true
	}
	if p.ageKey != "" {
		return true
	}
	if p.ageKeyStore != "" {
		return p.storeHasKey()
	}
	if p.ageKeyFile != "" {
		if path, err := expandKeyPath(p.ageKeyFile); err == nil {
			if _, statErr := os.Stat(path); statErr == nil {
				return true
			}
		}
	}
	if def, err := defaultSopsAgeKeysFile(); err == nil {
		if _, statErr := os.Stat(def); statErr == nil {
			return true
		}
	}
	return false
}

// GenerateKey generates a new age identity for this vault and writes each half to the sink implied
// by the vault's config (private → `age_key_file` or the sops default keys file; public → the
// recipients file, default `.sops.yaml`).
func (p *sopsProvider) GenerateKey(basePath string) (*providers.KeygenResult, error) {
	defer perf.Track(nil, "providers.sopsProvider.GenerateKey")()

	if !isAgeSopsKind(p.kind) {
		return nil, fmt.Errorf("%w: vault %q is %q (only age SOPS vaults generate keys)", providers.ErrKeygenNotSupported, p.name, p.kind)
	}
	if p.ageKey != "" {
		return nil, errUtils.Build(ErrSopsAgeKey).
			WithCausef("vault %q sources its key from inline `spec.age_key`", p.name).
			WithHint("Remove `spec.age_key`, or point `spec.age_key_file` at a writable path, so keygen has a sink to write the new identity.").
			Err()
	}
	if p.ageRecipients != "" {
		return nil, errUtils.Build(ErrSopsRecipients).
			WithCausef("vault %q pins recipients via `spec.age_recipients`", p.name).
			WithHint("keygen records the recipient in a `.sops.yaml` creation rule; remove `spec.age_recipients` to use that, or add the generated recipient there yourself.").
			Err()
	}

	id, err := generateAgeIdentity()
	if err != nil {
		return nil, err
	}

	privateSink, err := p.writePrivateIdentity(id.Private)
	if err != nil {
		return nil, err
	}

	recipientSink := p.recipientSinkPath(basePath)
	if err := p.writeRecipient(recipientSink, id.Recipient); err != nil {
		return nil, err
	}

	return &providers.KeygenResult{
		Vault:   p.name,
		Kind:    p.kind,
		Summary: "Generated an age key pair.",
		Outputs: []providers.KeygenOutput{
			{Label: "private identity", Location: privateSink, Sensitive: true},
			{Label: "public recipient", Location: recipientSink, Sensitive: false},
		},
		Public: id.Recipient,
	}, nil
}

// writePrivateIdentity writes the generated age private identity to the vault's configured sink —
// the `age_key.store` store when set, otherwise the key file — and returns its location for output.
func (p *sopsProvider) writePrivateIdentity(identity string) (string, error) {
	if p.ageKeyStore != "" {
		return p.writeKeyToStore(identity)
	}
	privateSink, err := p.privateKeySink()
	if err != nil {
		return "", err
	}
	if err := appendIdentityToKeysFile(privateSink, identity); err != nil {
		return "", err
	}
	return privateSink, nil
}

// privateKeySink resolves where the private identity is written: an explicit `spec.age_key_file`
// (expanded), otherwise the sops default keys file.
func (p *sopsProvider) privateKeySink() (string, error) {
	if p.ageKeyFile != "" {
		path, err := expandKeyPath(p.ageKeyFile)
		if err != nil {
			return "", ageKeyFileErr(p.ageKeyFile, err)
		}
		return path, nil
	}
	return defaultSopsAgeKeysFile()
}

// recipientSinkPath resolves the recipients file: an explicit `spec.recipients_file` (absolute, or
// relative to basePath), otherwise `.sops.yaml` at basePath.
func (p *sopsProvider) recipientSinkPath(basePath string) string {
	if p.recipientsFile != "" {
		if filepath.IsAbs(p.recipientsFile) {
			return p.recipientsFile
		}
		return filepath.Join(basePath, p.recipientsFile)
	}
	return filepath.Join(basePath, defaultSopsConfigName)
}

// writeRecipient merges the recipient into the recipients file's creation rule for this vault's
// file pattern, preserving existing rules.
func (p *sopsProvider) writeRecipient(sink, recipient string) error {
	existing, err := os.ReadFile(sink)
	if err != nil && !os.IsNotExist(err) {
		return errUtils.Build(ErrSopsRecipients).WithCause(fmt.Errorf("reading %q: %w", sink, err)).Err()
	}
	out, err := upsertSopsCreationRule(existing, fileTemplateToPathRegex(p.file), recipient)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(sink), keysDirMode); err != nil {
		return errUtils.Build(ErrSopsRecipients).WithCause(fmt.Errorf("creating dir for %q: %w", sink, err)).Err()
	}
	if err := os.WriteFile(sink, out, recipientsFileMode); err != nil {
		return errUtils.Build(ErrSopsRecipients).WithCause(fmt.Errorf("writing %q: %w", sink, err)).Err()
	}
	return nil
}

// generateAgeIdentity creates a new X25519 age identity in-process (no `age-keygen` binary).
func generateAgeIdentity() (AgeIdentity, error) {
	id, err := age.GenerateX25519Identity()
	if err != nil {
		return AgeIdentity{}, errUtils.Build(ErrSopsAgeKey).
			WithCause(fmt.Errorf("generating age identity: %w", err)).
			Err()
	}
	return AgeIdentity{Private: id.String(), Recipient: id.Recipient().String()}, nil
}

// defaultSopsAgeKeysFile returns the keys file the sops keysource reads automatically: the
// `SOPS_AGE_KEY_FILE` override when set, otherwise `<user config dir>/sops/age/keys.txt`. The
// config dir is resolved exactly as getsops/sops does (so keygen writes where sops reads) — NOT
// via Atmos's XDG helper, which namespaces paths under `atmos/`.
func defaultSopsAgeKeysFile() (string, error) {
	//nolint:forbidigo // SOPS_AGE_KEY_FILE is sops' own override; match its lookup exactly.
	if f := os.Getenv("SOPS_AGE_KEY_FILE"); f != "" {
		return f, nil
	}
	dir, err := sopsUserConfigDir()
	if err != nil {
		return "", errUtils.Build(ErrSopsAgeKey).
			WithCause(fmt.Errorf("resolving user config dir for keys.txt: %w", err)).Err()
	}
	return filepath.Join(dir, "sops", "age", "keys.txt"), nil
}

// sopsUserConfigDir replicates getsops/sops `getUserConfigDir` (age/keysource.go): on macOS it
// honors `XDG_CONFIG_HOME` (which `os.UserConfigDir` ignores there), otherwise it defers to
// `os.UserConfigDir`. Replicated rather than reused so keygen targets the same file sops decrypts
// from.
func sopsUserConfigDir() (string, error) {
	if runtime.GOOS == "darwin" {
		//nolint:forbidigo // match sops: XDG_CONFIG_HOME overrides os.UserConfigDir on macOS.
		if xch := os.Getenv("XDG_CONFIG_HOME"); xch != "" {
			return xch, nil
		}
	}
	return os.UserConfigDir()
}

// appendIdentityToKeysFile appends an age private identity to a keys file (creating it and its
// parent directory if needed). The keys file holds many identities — one per line — and the sops
// keysource tries them all on decrypt, so appending never clobbers another vault's key. An
// identity already present is left as-is (idempotent).
func appendIdentityToKeysFile(path, identity string) error {
	identity = strings.TrimSpace(identity)

	if err := os.MkdirAll(filepath.Dir(path), keysDirMode); err != nil {
		return errUtils.Build(ErrSopsAgeKey).
			WithCause(fmt.Errorf("creating keys directory %q: %w", filepath.Dir(path), err)).
			Err()
	}

	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return errUtils.Build(ErrSopsAgeKey).WithCause(fmt.Errorf("reading %q: %w", path, err)).Err()
	}
	if identityPresent(string(existing), identity) {
		return nil
	}

	var buf strings.Builder
	buf.Write(existing)
	if len(existing) > 0 && !strings.HasSuffix(string(existing), "\n") {
		buf.WriteByte('\n')
	}
	buf.WriteString(identity)
	buf.WriteByte('\n')

	if err := os.WriteFile(path, []byte(buf.String()), keysFileMode); err != nil {
		return errUtils.Build(ErrSopsAgeKey).WithCause(fmt.Errorf("writing %q: %w", path, err)).Err()
	}
	return nil
}

// identityPresent reports whether identity already appears as a non-comment line in content.
func identityPresent(content, identity string) bool {
	for _, line := range strings.Split(content, "\n") {
		if strings.TrimSpace(line) == identity {
			return true
		}
	}
	return false
}

// fileTemplateToPathRegex converts a SOPS `spec.file` Go-template path (e.g.
// `secrets/{{ .atmos_stack }}.enc.yaml`) into a `.sops.yaml` creation-rule `path_regex` that
// matches every concrete file the template can produce (`secrets/.*\.enc\.yaml$`). Literal
// segments are regex-escaped; each `{{ … }}` expression becomes `.*`.
func fileTemplateToPathRegex(fileTemplate string) string {
	var out strings.Builder
	rest := fileTemplate
	for {
		open := strings.Index(rest, "{{")
		if open < 0 {
			out.WriteString(regexp.QuoteMeta(rest))
			break
		}
		out.WriteString(regexp.QuoteMeta(rest[:open]))
		close := strings.Index(rest[open:], "}}")
		if close < 0 {
			// Unbalanced template: treat the remainder as literal.
			out.WriteString(regexp.QuoteMeta(rest[open:]))
			break
		}
		out.WriteString(".*")
		rest = rest[open+close+2:]
	}
	return out.String() + "$"
}

// upsertSopsCreationRule merges recipient into the `.sops.yaml` creation rule for pathRegex,
// preserving every other rule (and any unrelated top-level keys). When a rule for pathRegex
// already exists, recipient is added to its comma-separated `age` list (idempotent); otherwise a
// new rule is appended. Operating on a yaml.Node preserves field order and comments. An empty
// input yields a minimal valid document. Returns the new file bytes.
func upsertSopsCreationRule(existing []byte, pathRegex, recipient string) ([]byte, error) {
	root, rules, err := loadSopsCreationRules(existing)
	if err != nil {
		return nil, err
	}

	if ruleNode := findCreationRule(rules, pathRegex); ruleNode != nil {
		addRecipientToRule(ruleNode, recipient)
	} else {
		rules.Content = append(rules.Content, newCreationRuleNode(pathRegex, recipient))
	}

	out, err := yaml.Marshal(root)
	if err != nil {
		return nil, errUtils.Build(ErrSopsRecipients).WithCause(fmt.Errorf("marshaling .sops.yaml: %w", err)).Err()
	}
	return out, nil
}

// loadSopsCreationRules parses existing `.sops.yaml` bytes and returns the document root mapping
// plus its `creation_rules` sequence node, creating either if absent.
func loadSopsCreationRules(existing []byte) (root *yaml.Node, rules *yaml.Node, err error) {
	var doc yaml.Node
	if len(strings.TrimSpace(string(existing))) > 0 {
		if err := yaml.Unmarshal(existing, &doc); err != nil {
			return nil, nil, errUtils.Build(ErrSopsRecipients).
				WithCause(fmt.Errorf("parsing .sops.yaml: %w", err)).Err()
		}
	}

	rootMap := documentMapping(&doc)
	rulesNode := mappingValue(rootMap, "creation_rules")
	if rulesNode == nil {
		keyNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: yamlStrTag, Value: "creation_rules"}
		rulesNode = &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		rootMap.Content = append(rootMap.Content, keyNode, rulesNode)
	}
	if rulesNode.Kind != yaml.SequenceNode {
		return nil, nil, errUtils.Build(ErrSopsRecipients).
			WithCausef(".sops.yaml `creation_rules` is not a list").Err()
	}
	return &doc, rulesNode, nil
}

// documentMapping returns the top-level mapping node of a YAML document, initializing the document
// and an empty mapping when needed.
func documentMapping(doc *yaml.Node) *yaml.Node {
	if doc.Kind == 0 {
		doc.Kind = yaml.DocumentNode
	}
	if len(doc.Content) == 0 {
		doc.Content = []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}
	}
	return doc.Content[0]
}

// mappingValue returns the value node for key in a mapping node, or nil.
func mappingValue(mapping *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}
	return nil
}

// findCreationRule returns the rule mapping whose `path_regex` equals pathRegex, or nil.
func findCreationRule(rules *yaml.Node, pathRegex string) *yaml.Node {
	for _, rule := range rules.Content {
		if rule.Kind == yaml.MappingNode {
			if pr := mappingValue(rule, "path_regex"); pr != nil && pr.Value == pathRegex {
				return rule
			}
		}
	}
	return nil
}

// addRecipientToRule adds recipient to a rule's comma-separated `age` value (idempotent),
// creating the `age` field if absent.
func addRecipientToRule(rule *yaml.Node, recipient string) {
	ageNode := mappingValue(rule, "age")
	if ageNode == nil {
		rule.Content = append(rule.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: yamlStrTag, Value: "age"},
			&yaml.Node{Kind: yaml.ScalarNode, Tag: yamlStrTag, Value: recipient})
		return
	}
	for _, r := range strings.Split(ageNode.Value, ",") {
		if strings.TrimSpace(r) == recipient {
			return // already present.
		}
	}
	if strings.TrimSpace(ageNode.Value) == "" {
		ageNode.Value = recipient
	} else {
		ageNode.Value = ageNode.Value + "," + recipient
	}
}

// newCreationRuleNode builds a `{ path_regex: …, age: … }` mapping node.
func newCreationRuleNode(pathRegex, recipient string) *yaml.Node {
	return &yaml.Node{
		Kind: yaml.MappingNode,
		Tag:  "!!map",
		Content: []*yaml.Node{
			{Kind: yaml.ScalarNode, Tag: yamlStrTag, Value: "path_regex"},
			{Kind: yaml.ScalarNode, Tag: yamlStrTag, Value: pathRegex},
			{Kind: yaml.ScalarNode, Tag: yamlStrTag, Value: "age"},
			{Kind: yaml.ScalarNode, Tag: yamlStrTag, Value: recipient},
		},
	}
}
