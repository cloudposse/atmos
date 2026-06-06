package providers

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestGenerateAgeIdentity(t *testing.T) {
	id, err := generateAgeIdentity()
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(id.Private, "AGE-SECRET-KEY-1"), "private: %s", id.Private)
	assert.True(t, strings.HasPrefix(id.Recipient, "age1"), "recipient: %s", id.Recipient)

	// Two generations differ.
	id2, err := generateAgeIdentity()
	require.NoError(t, err)
	assert.NotEqual(t, id.Private, id2.Private)
}

func TestAppendIdentityToKeysFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "keys.txt")

	require.NoError(t, appendIdentityToKeysFile(path, "AGE-SECRET-KEY-1AAA"))
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "AGE-SECRET-KEY-1AAA\n", string(data))

	// Mode is owner-only.
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	// Appending a second identity keeps the first (multi-identity pool).
	require.NoError(t, appendIdentityToKeysFile(path, "AGE-SECRET-KEY-1BBB"))
	data, err = os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "AGE-SECRET-KEY-1AAA\nAGE-SECRET-KEY-1BBB\n", string(data))

	// Re-appending an existing identity is idempotent.
	require.NoError(t, appendIdentityToKeysFile(path, "AGE-SECRET-KEY-1AAA"))
	data, err = os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "AGE-SECRET-KEY-1AAA\nAGE-SECRET-KEY-1BBB\n", string(data))
}

func TestAppendIdentityToKeysFile_NoTrailingNewline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "keys.txt")
	require.NoError(t, os.WriteFile(path, []byte("AGE-SECRET-KEY-1AAA"), 0o600)) // no trailing newline.

	require.NoError(t, appendIdentityToKeysFile(path, "AGE-SECRET-KEY-1BBB"))
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "AGE-SECRET-KEY-1AAA\nAGE-SECRET-KEY-1BBB\n", string(data))
}

func TestFileTemplateToPathRegex(t *testing.T) {
	tests := []struct {
		name    string
		file    string
		want    string
		matches []string
		rejects []string
	}{
		{
			name:    "templated stack",
			file:    "secrets/{{ .atmos_stack }}.enc.yaml",
			want:    `secrets/.*\.enc\.yaml$`,
			matches: []string{"secrets/plat-ue2-dev.enc.yaml", "secrets/x.enc.yaml"},
			rejects: []string{"secrets/dev.enc.json", "other/dev.enc.yaml"},
		},
		{
			name:    "stack and component",
			file:    "secrets/{{ .atmos_stack }}/{{ .atmos_component }}.enc.yaml",
			want:    `secrets/.*/.*\.enc\.yaml$`,
			matches: []string{"secrets/dev/vpc.enc.yaml"},
			rejects: []string{"secrets/dev.enc.yaml"},
		},
		{
			name:    "static path",
			file:    "secrets/global.enc.yaml",
			want:    `secrets/global\.enc\.yaml$`,
			matches: []string{"secrets/global.enc.yaml"},
			rejects: []string{"secrets/globalXenc.yaml"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fileTemplateToPathRegex(tt.file)
			assert.Equal(t, tt.want, got)

			re, err := regexp.Compile(got)
			require.NoError(t, err, "derived path_regex must compile")
			for _, m := range tt.matches {
				assert.Truef(t, re.MatchString(m), "expected %q to match %q", m, got)
			}
			for _, r := range tt.rejects {
				assert.Falsef(t, re.MatchString(r), "expected %q NOT to match %q", r, got)
			}
		})
	}
}

// parsedSopsConfig is a minimal view of .sops.yaml for assertions.
type parsedSopsConfig struct {
	CreationRules []struct {
		PathRegex string `yaml:"path_regex"`
		Age       string `yaml:"age"`
	} `yaml:"creation_rules"`
	Other string `yaml:"other"`
}

func parseSops(t *testing.T, b []byte) parsedSopsConfig {
	t.Helper()
	var c parsedSopsConfig
	require.NoError(t, yaml.Unmarshal(b, &c))
	return c
}

func TestUpsertSopsCreationRule_EmptyInput(t *testing.T) {
	out, err := upsertSopsCreationRule(nil, `secrets/.*\.enc\.yaml$`, "age1aaa")
	require.NoError(t, err)

	c := parseSops(t, out)
	require.Len(t, c.CreationRules, 1)
	assert.Equal(t, `secrets/.*\.enc\.yaml$`, c.CreationRules[0].PathRegex)
	assert.Equal(t, "age1aaa", c.CreationRules[0].Age)

	// Output must round-trip through the SOPS loader (compiles as a real .sops.yaml).
	assert.Contains(t, string(out), "creation_rules:")
}

func TestUpsertSopsCreationRule_AppendsToExistingRule(t *testing.T) {
	existing := []byte("creation_rules:\n  - path_regex: secrets/.*\\.enc\\.yaml$\n    age: age1aaa\n")

	out, err := upsertSopsCreationRule(existing, `secrets/.*\.enc\.yaml$`, "age1bbb")
	require.NoError(t, err)

	c := parseSops(t, out)
	require.Len(t, c.CreationRules, 1, "same path_regex must merge, not duplicate")
	assert.Equal(t, "age1aaa,age1bbb", c.CreationRules[0].Age)
}

func TestUpsertSopsCreationRule_Idempotent(t *testing.T) {
	existing := []byte("creation_rules:\n  - path_regex: secrets/.*\\.enc\\.yaml$\n    age: age1aaa\n")

	out, err := upsertSopsCreationRule(existing, `secrets/.*\.enc\.yaml$`, "age1aaa")
	require.NoError(t, err)
	c := parseSops(t, out)
	require.Len(t, c.CreationRules, 1)
	assert.Equal(t, "age1aaa", c.CreationRules[0].Age)
}

func TestUpsertSopsCreationRule_NewRulePreservesOthers(t *testing.T) {
	existing := []byte("other: keepme\ncreation_rules:\n  - path_regex: a/.*$\n    age: age1aaa\n")

	out, err := upsertSopsCreationRule(existing, `b/.*$`, "age1bbb")
	require.NoError(t, err)

	c := parseSops(t, out)
	assert.Equal(t, "keepme", c.Other, "unrelated top-level keys must be preserved")
	require.Len(t, c.CreationRules, 2)
	assert.Equal(t, "a/.*$", c.CreationRules[0].PathRegex)
	assert.Equal(t, "age1aaa", c.CreationRules[0].Age)
	assert.Equal(t, "b/.*$", c.CreationRules[1].PathRegex)
	assert.Equal(t, "age1bbb", c.CreationRules[1].Age)
}

func TestUpsertSopsCreationRule_RoundTripStable(t *testing.T) {
	// Applying the same recipient twice yields identical bytes the second time.
	out1, err := upsertSopsCreationRule(nil, `x/.*$`, "age1aaa")
	require.NoError(t, err)
	out2, err := upsertSopsCreationRule(out1, `x/.*$`, "age1aaa")
	require.NoError(t, err)
	assert.Equal(t, string(out1), string(out2))
}

// TestSopsKeygen_EndToEnd proves the DX: with no key, keygen writes the identity (to the
// SOPS_AGE_KEY_FILE the sops keysource reads) and the recipient (to .sops.yaml), after which the
// provider can Set (create+encrypt the first file in-process) and Get (decrypt) — no `sops` binary,
// no manual age-keygen.
func TestSopsKeygen_EndToEnd(t *testing.T) {
	dir := t.TempDir()
	// sops matches a creation-rule path_regex relative to the .sops.yaml location, and resolveFile
	// renders relative to CWD — so mirror real usage: a relative spec.file under the base path.
	t.Chdir(dir)
	keysFile := filepath.Join(dir, "keys.txt")
	// Point sops at our temp keys file for both write (keygen sink) and read (decrypt).
	t.Setenv("SOPS_AGE_KEY_FILE", keysFile)
	t.Setenv("SOPS_AGE_KEY", "")

	p := &sopsProvider{
		name:           "sops",
		kind:           "sops/age",
		file:           filepath.Join("secrets", "{{ .atmos_stack }}.enc.yaml"),
		recipientsFile: ".sops.yaml",
	}

	require.False(t, p.HasKey(), "no key should exist before keygen")

	res, err := p.GenerateKey(dir)
	require.NoError(t, err)
	assert.Equal(t, "sops", res.Vault)
	assert.Equal(t, "sops/age", res.Kind)
	assert.True(t, strings.HasPrefix(res.Public, "age1"))

	privateSink := keygenOutputLocation(t, res, "private identity")
	recipientSink := keygenOutputLocation(t, res, "public recipient")
	assert.Equal(t, keysFile, privateSink)

	kb, err := os.ReadFile(keysFile)
	require.NoError(t, err)
	assert.Contains(t, string(kb), "AGE-SECRET-KEY-1", "private identity written to keys file")

	sb, err := os.ReadFile(recipientSink)
	require.NoError(t, err)
	assert.Contains(t, string(sb), res.Public, "recipient recorded in .sops.yaml")

	require.True(t, p.HasKey(), "key should exist after keygen")

	// First write creates + encrypts the file using the .sops.yaml recipient.
	coord := Coordinate{Stack: "dev", Component: "app", Key: "DB_PASSWORD"}
	require.NoError(t, p.Set(coord, "hunter2"))

	enc, err := os.ReadFile(filepath.Join(dir, "secrets", "dev.enc.yaml"))
	require.NoError(t, err)
	assert.NotContains(t, string(enc), "hunter2", "value must be encrypted at rest")
	assert.Contains(t, string(enc), "sops", "file carries sops metadata")

	// Round-trip: decrypt with the generated identity from the keys file.
	got, err := p.Get(coord)
	require.NoError(t, err)
	assert.Equal(t, "hunter2", got)
}

func TestSopsKeygen_RejectsInlineAgeKey(t *testing.T) {
	p := &sopsProvider{name: "sops", kind: "sops/age", file: "secrets/x.enc.yaml", ageKey: "AGE-SECRET-KEY-1AAA"}
	_, err := p.GenerateKey(t.TempDir())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrSopsAgeKey)
}

func TestSopsKeygen_RejectsPinnedRecipients(t *testing.T) {
	p := &sopsProvider{name: "sops", kind: "sops/age", file: "secrets/x.enc.yaml", ageRecipients: "age1xyz"}
	_, err := p.GenerateKey(t.TempDir())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrSopsRecipients)
}

// keygenOutputLocation returns the Location of the keygen output with the given label, failing the
// test if absent.
func keygenOutputLocation(t *testing.T, res *KeygenResult, label string) string {
	t.Helper()
	for _, out := range res.Outputs {
		if out.Label == label {
			return out.Location
		}
	}
	t.Fatalf("keygen result has no output labeled %q", label)
	return ""
}

func TestSopsKeygen_RejectsNonAgeKind(t *testing.T) {
	p := &sopsProvider{name: "kms", kind: "sops/aws-kms", file: "secrets/x.enc.yaml"}
	assert.True(t, p.HasKey(), "non-age kinds report key present (externally managed)")
	_, err := p.GenerateKey(t.TempDir())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrKeygenNotSupported)
}
