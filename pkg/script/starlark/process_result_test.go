package starlark

import (
	"context"
	"io"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.starlark.net/starlark"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/process"
	"github.com/cloudposse/atmos/pkg/script"
)

func TestProcessResultData(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, stdout, expression, want, wantError string
	}{
		{"objects", `[{"component":"api","stack":"dev"},{"component":"worker","stack":"prod"}]`, `[x["component"] for x in r.data]`, `["api","worker"]`, ""},
		{"scalars", `[null,true,false,1,1.5,"text"]`, `r.data`, `[null,true,false,1,1.5,"text"]`, ""},
		{"large integer", `123456789012345678901234567890`, `r.data + 1`, `123456789012345678901234567891`, ""},
		{"null", `null`, `[r.data, r.data == None]`, `[null,true]`, ""},
		{"raw whitespace", "  [1, 2]\n", `[r.data, r.stdout, r.stderr, r.exit_code]`, `[[1,2],"  [1, 2]\n","diagnostic",0]`, ""},
		{"plain text untouched", "plain text", `[r.stdout, r.exit_code]`, `["plain text",0]`, ""},
		{"serialize plain text", "plain text", `r`, `{"stdout":"plain text","stderr":"diagnostic","exit_code":0}`, ""},
		{"invalid", "plain text", `r.data`, "", "result.data requires valid JSON on stdout"},
		{"empty", "", `r.data`, "", "result.data requires valid JSON on stdout"},
		{"multiple documents", "{}\n{}", `r.data`, "", "result.data requires valid JSON on stdout"},
		{"frozen dictionary", `{"items":[]}`, `r.data.update({"new":1})`, "", "frozen"},
		{"frozen nested list", `{"items":[]}`, `r.data["items"].append(1)`, "", "frozen"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runner := NewMockRunner(gomock.NewController(t))
			runner.EXPECT().Run(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, spec process.TaskSpec) process.Result {
				_, _ = io.WriteString(spec.Streams.Stdout, tc.stdout)
				_, _ = io.WriteString(spec.Streams.Stderr, "diagnostic")
				return process.Result{Started: true}
			})
			result, err := runSource(t, "r = exec.run([\"tool\"], output=\"capture\")\noutput = "+tc.expression, WithProcessRunner(runner))
			if tc.wantError != "" {
				require.ErrorIs(t, err, errUtils.ErrStarlark)
				assert.ErrorContains(t, err, tc.wantError)
				return
			}
			require.NoError(t, err)
			assert.JSONEq(t, tc.want, result.Value)
		})
	}
}

func TestResultDataConcurrentAccess(t *testing.T) {
	t.Parallel()
	r := newProcessResult(script.ProcessOutput{Stdout: `{"items":[1,2]}`})
	r.Freeze()
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			value, err := r.Attr("data")
			assert.NoError(t, err)
			assert.Equal(t, `{"items": [1, 2]}`, value.String())
		})
	}
	wg.Wait()
	first, err := r.Attr("data")
	require.NoError(t, err)
	second, err := r.Attr("data")
	require.NoError(t, err)
	assert.Same(t, first, second)
	assert.Contains(t, r.AttrNames(), "data")
	assert.Equal(t, starlark.True, r.Truth())
	_, err = r.Hash()
	require.NoError(t, err)
	equal, err := starlark.Equal(r, newProcessResult(script.ProcessOutput{Stdout: r.payload}))
	require.NoError(t, err)
	assert.True(t, equal)
}

func TestResultDataNonzeroExit(t *testing.T) {
	t.Parallel()
	runner := NewMockRunner(gomock.NewController(t))
	runner.EXPECT().Run(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, spec process.TaskSpec) process.Result {
		_, _ = io.WriteString(spec.Streams.Stdout, `{"reason":"denied"}`)
		return process.Result{Started: true, ExitCode: 7}
	})
	result, err := runSource(t, `r = atmos.run(["custom"], check=False, output="capture")
output = [r.exit_code, r.data["reason"], r.stdout]`, WithProcessRunner(runner))
	require.NoError(t, err)
	assert.JSONEq(t, `[7,"denied","{\"reason\":\"denied\"}"]`, result.Value)
}
