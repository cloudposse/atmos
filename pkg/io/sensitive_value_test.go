package io

import (
	"encoding/base64"
	"encoding/json"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func resetMasking(t *testing.T) {
	t.Helper()
	Reset()
	t.Cleanup(Reset)
	require.NoError(t, Initialize())
}

func TestTrivialSecret(t *testing.T) {
	trivial := []string{
		"", " ", "x", "app", "dev", "  }", "},", "{}", "[ ]", "true", "FALSE", "null", "0.1", "42", "  -- ",
	}
	for _, value := range trivial {
		assert.True(t, trivialSecret(value), "%q is trivial", value)
	}
	maskable := []string{
		"abcd", "1234", "not-a-real-password", "  password = \"hunter22\"", "9007199254740993", "émoji✓", "!@#$%^&*",
	}
	for _, value := range maskable {
		assert.False(t, trivialSecret(value), "%q is maskable", value)
	}
}

// A provider's sensitive value registers its distinctive scalars, never its object keys or trivial
// scalars that would mask unrelated text such as digits in timings or "app" in "apply".
func TestRegisterSensitiveValueSkipsKeysAndTrivialScalars(t *testing.T) {
	resetMasking(t)

	RegisterSensitiveValue(map[string]any{
		"name":     "x",
		"count":    json.Number("3"),
		"enabled":  true,
		"unit":     "app",
		"password": "not-a-real-password",
		"nested":   []any{"inner-secret-value", nil, json.Number("123456789012")},
		"encoded":  `{"token":"json-inner-secret","short":"ab"}`,
	})

	masked := MaskString("known after apply, 3 resources, name=x count enabled password unit")
	assert.Equal(t, "known after apply, 3 resources, name=x count enabled password unit", masked)
	for _, secret := range []string{"not-a-real-password", "inner-secret-value", "123456789012", "json-inner-secret"} {
		assert.Equal(t, "<MASKED>", MaskString(secret), secret)
	}
	assert.Equal(t, "ab", MaskString("ab"))
}

// A sensitive text, such as a generated file, is masked whole and by its distinctive lines; its
// structural lines such as a closing brace stay readable everywhere else.
func TestRegisterSensitiveTextSkipsTrivialLines(t *testing.T) {
	resetMasking(t)
	text := "terraform {\n  backend \"s3\" {\n    secret_key = \"backend-secret-key-1234\"\n  }\n}\n"

	RegisterSensitiveText(text)

	assert.Equal(t, "<MASKED>", MaskString(text))
	assert.NotContains(t, MaskString("  secret_key = \"backend-secret-key-1234\""), "backend-secret-key-1234")
	assert.Equal(t, "variable \"marker\" {}\n  }\n}", MaskString("variable \"marker\" {}\n  }\n}"))
	assert.Equal(t, "1: terraform {", MaskString("1: terraform {"), "a block header is structure, not a secret")

	RegisterSensitiveText("}")
	assert.Equal(t, "}", MaskString("}"), "a trivial text is never registered")
	RegisterSensitiveText("single-line-secret-value")
	assert.Equal(t, "<MASKED>", MaskString("single-line-secret-value"))
}

// A multi-line sensitive string, such as a private key, and one with Windows line endings are masked
// whole and line by line, as a sensitive text is.
func TestRegisterSensitiveValueMasksMultilineStrings(t *testing.T) {
	resetMasking(t)

	RegisterSensitiveValue(map[string]any{"key": "-----BEGIN KEY-----\r\nmultiline-secret-line\r\n-----END KEY-----"})

	normalized := "-----BEGIN KEY-----\nmultiline-secret-line\n-----END KEY-----"
	assert.Equal(t, "<MASKED>", MaskString(normalized), "the normalized text is masked whole")
	assert.NotContains(t, MaskString("line: multiline-secret-line"), "multiline-secret-line")
}

// A declared-sensitive value made only of symbols is still a secret worth masking: only short values,
// booleans and null are too common to mask.
func TestRegisterSensitiveValueMasksSymbolOnlySecrets(t *testing.T) {
	resetMasking(t)

	RegisterSensitiveValue(map[string]any{"password": "!@#$%^&*"})

	assert.Equal(t, "password=<MASKED>", MaskString("password=!@#$%^&*"))
}

// The complete sensitive text is masked in its encoded forms too, so a hook that prints a generated file
// base64- or URL-encoded does not reveal it, while punctuation-only lines stay unregistered.
func TestRegisterSensitiveTextMasksEncodedForms(t *testing.T) {
	resetMasking(t)
	text := "terraform {\n  backend \"s3\" {\n    secret_key = \"encoded-secret-key-9f2\"\n  }\n}\n-----\n"

	RegisterSensitiveText(text)

	for _, encoded := range []string{
		base64.StdEncoding.EncodeToString([]byte(text)),
		base64.URLEncoding.EncodeToString([]byte(text)),
		base64.RawStdEncoding.EncodeToString([]byte(text)),
		base64.RawURLEncoding.EncodeToString([]byte(text)),
		url.QueryEscape(text),
	} {
		assert.Equal(t, "value: <MASKED>", MaskString("value: "+encoded), encoded)
	}
	escaped, err := json.Marshal(text)
	require.NoError(t, err)
	assert.NotContains(t, MaskString(string(escaped)), "encoded-secret-key-9f2")
	assert.Equal(t, "-----", MaskString("-----"), "a separator line is structure, not a secret")
}
