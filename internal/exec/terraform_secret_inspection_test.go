package exec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	iolib "github.com/cloudposse/atmos/pkg/io"
)

// TestNestedSecretInspection proves that inspection remains credential-free
// across native references, and cannot poison or consume execution caches.
func TestNestedSecretInspection(t *testing.T) {
	references := map[string]string{
		"output":    "!terraform.output producer credential",
		"state":     "!terraform.state producer credential",
		"component": `'{{ (atmos.Component "producer" "dev").vars.credential }}'`,
	}
	for name, expression := range references {
		t.Run(name, func(t *testing.T) {
			for _, provenance := range []bool{false, true} {
				t.Run(map[bool]string{false: "plain", true: "provenance"}[provenance], func(t *testing.T) {
					config, mockStore := setupNativeSecretReference(t)
					stackPath := filepath.Join(config.BasePath, "stacks", "dev.yaml")
					content, err := os.ReadFile(stackPath)
					require.NoError(t, err)
					require.NoError(t, os.WriteFile(stackPath, []byte(strings.ReplaceAll(string(content), "!terraform.output producer credential", expression)), 0o600))
					config.TrackProvenance = provenance
					calls := 0
					allowRetrieval := false
					mockStore.EXPECT().Get("dev", "producer", "CREDENTIAL").DoAndReturn(func(_, _, _ string) (any, error) {
						calls++
						require.True(t, allowRetrieval, "inspection must not contact the secret provider")
						return nativeReferenceSecret, nil
					}).AnyTimes()
					inspect := func() {
						before := calls
						var sections map[string]any
						if provenance {
							result, err := ExecuteDescribeComponentWithContext(DescribeComponentContextParams{
								AtmosConfig: config, Component: "consumer", Stack: "dev", ProcessTemplates: true, ProcessYamlFunctions: true,
							})
							require.NoError(t, err)
							sections = result.ComponentSection
						} else {
							var err error
							sections, err = ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
								AtmosConfig: config, Component: "consumer", Stack: "dev", ProcessTemplates: true, ProcessYamlFunctions: true,
							})
							require.NoError(t, err)
						}
						assert.Equal(t, iolib.MaskReplacement, sections["vars"].(map[string]any)["credential"])
						assert.Equal(t, before, calls, "inspection must not retrieve secrets, even after execution")
					}
					inspect()
					assert.Zero(t, calls)
					allowRetrieval = true
					sections, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
						AtmosConfig: config, Component: "consumer", Stack: "dev", ProcessTemplates: true, ProcessYamlFunctions: true, ResolveSecrets: true,
					})
					require.NoError(t, err)
					assert.Positive(t, calls)
					assert.Equal(t, nativeReferenceSecret, sections["vars"].(map[string]any)["credential"], "inspection must not cache placeholders for execution")
					allowRetrieval = false
					inspect()
				})
			}
		})
	}
}

// TestNestedSecretInspectionMaskDisabled keeps explicit unmasked inspection
// value-producing, rather than suppressing secret retrieval unconditionally.
func TestNestedSecretInspectionMaskDisabled(t *testing.T) {
	config, mockStore := setupNativeSecretReference(t)
	iolib.GetContext().Masker().SetEnabled(false)
	mockStore.EXPECT().Get("dev", "producer", "CREDENTIAL").Return(nativeReferenceSecret, nil).MinTimes(1)
	sections, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
		AtmosConfig: config, Component: "consumer", Stack: "dev", ProcessTemplates: true, ProcessYamlFunctions: true,
	})
	require.NoError(t, err)
	assert.Equal(t, nativeReferenceSecret, sections["vars"].(map[string]any)["credential"])
}
