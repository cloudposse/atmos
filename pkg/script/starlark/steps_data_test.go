package starlark

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/automation"
	"github.com/cloudposse/atmos/pkg/script"
)

func TestStepResultData(t *testing.T) {
	for _, tc := range []struct{ value, source, expected, failure string }{
		{`{"healthy":true}`, `output = [r.data["healthy"], r.value, r.metadata["status_code"], r.outputs["body"]]`, `[true,"{\"healthy\":true}",200,"raw output"]`, ""},
		{"text", `output = [r.value, r.metadata["status_code"]]`, `["text",200]`, ""},
		{"text", `output = r.data`, "", "result.data requires valid JSON on value"},
		{`[1,2]`, `def read():
    return r.data
output = steps.parallel(functions=[read,read])`, `[[1,2],[1,2]]`, ""},
	} {
		t.Run(tc.source, func(t *testing.T) {
			library := NewMockStepLibrary(gomock.NewController(t))
			library.EXPECT().Names().Return([]string{"http"})
			library.EXPECT().Fork().Return(library).AnyTimes()
			library.EXPECT().Run(gomock.Any(), gomock.Any()).Return(&automation.StepResult{
				Value: tc.value, Metadata: map[string]any{"status_code": 200}, Outputs: map[string]string{"body": "raw output"},
			}, nil)
			result, err := New().Execute(t.Context(), script.Spec{Steps: library, Source: "r = steps.http(url=\"https://example.invalid\")\n" + tc.source})
			if tc.failure != "" {
				require.ErrorContains(t, err, tc.failure)
				return
			}
			require.NoError(t, err)
			assert.JSONEq(t, tc.expected, result.Value)
		})
	}
}
