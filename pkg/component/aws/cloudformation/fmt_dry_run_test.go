package cloudformation

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRunOperation_DryRunFmtPreservesTemplate(t *testing.T) {
	for _, check := range []bool{false, true} {
		t.Run(map[bool]string{false: "format", true: "check"}[check], func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "template.yaml")
			original := "Resources: {Bucket: {Type: 'AWS::S3::Bucket'}}\n"
			require.NoError(t, os.WriteFile(path, []byte(original), 0o600))
			assertOperationDryRun(t, OperationFmt, &stackSpec{
				StackName: "vpc", TemplateBody: original, TemplateAbsPath: path,
			}, map[string]any{"check": check})
			contents, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, original, string(contents))
		})
	}
}
