package asciicast

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/castplan"
)

func declarationRecording(t *testing.T, version int, middle string) string {
	t.Helper()
	plan := &castplan.Plan{}
	fingerprint, err := plan.Fingerprint()
	require.NoError(t, err)
	marker := func(kind string) string {
		timestamp := 0
		if kind == "complete" {
			timestamp = 1
		}
		encoded, marshalErr := json.Marshal([]any{timestamp, "m", castplan.Marker{Kind: kind, Fingerprint: fingerprint}.Encode()})
		require.NoError(t, marshalErr)
		return string(encoded)
	}
	header, err := json.Marshal(map[string]int{"version": version, "width": 80, "height": 24})
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "recording.cast")
	content := string(header) + "\n" + marker("plan") + "\n" + middle + "\n" + marker("complete") + "\n"
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func TestValidateDeclarationRejectsMalformedEvents(t *testing.T) {
	for name, event := range map[string]string{
		"short":         `[0,"o"]`,
		"long":          `[0,"o","text","extra"]`,
		"null record":   `null`,
		"negative time": `[-0.1,"o","text"]`,
		"infinite time": `[1e999,"o","text"]`,
		"nan time":      `[NaN,"o","text"]`,
		"string time":   `["0","o","text"]`,
		"null time":     `[null,"o","text"]`,
		"number stream": `[0,1,"text"]`,
		"null stream":   `[0,null,"text"]`,
		"number data":   `[0,"o",1]`,
		"null data":     `[0,"o",null]`,
		"array data":    `[0,"o",[]]`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ValidateDeclaration(declarationRecording(t, 2, event), &castplan.Plan{})
			require.Error(t, err)
		})
	}
}

func TestValidateDeclarationVersionsAndReadOnly(t *testing.T) {
	for _, version := range []int{2, 3} {
		path := declarationRecording(t, version, `[0.25,"o","hello"]`)
		before, err := os.ReadFile(path)
		require.NoError(t, err)
		text, err := ValidateDeclaration(path, &castplan.Plan{})
		require.NoError(t, err)
		require.Equal(t, "hello", text)
		after, err := os.ReadFile(path)
		require.NoError(t, err)
		require.Equal(t, before, after)
	}
	_, err := ValidateDeclaration(declarationRecording(t, 4, `[0,"o","hello"]`), &castplan.Plan{})
	require.ErrorContains(t, err, "unsupported cast version")
}
