package policy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/template/render"
)

// renderWithPolicy renders text with the policy wrapper, either with plain
// data or with a "." context datasource (the path datasource-backed sections
// use).
func renderWithPolicy(t *testing.T, text string, withContext bool) (string, error) {
	t.Helper()
	req := &render.Request{Name: "stack.yaml", Text: text, WrapFuncs: FuncsWrapper("stack.yaml")}
	if withContext {
		// An HTTP source keeps this portable: gomplate cannot read file:///C:/... URLs on Windows.
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"name":"atmos"}`))
		}))
		t.Cleanup(server.Close)
		req.ContextSources = map[string]render.Datasource{".": {URL: server.URL + "/top.json"}}
	} else {
		req.Data = map[string]any{"name": "atmos"}
	}
	return render.New().Render(context.Background(), req)
}

func TestPolicyTmplFunctionsAreAdmitted(t *testing.T) {
	paths := map[string]bool{"plain data": false, "context datasource": true}

	for pathName, withContext := range paths {
		t.Run(pathName, func(t *testing.T) {
			t.Run("tpl denied by name", func(t *testing.T) {
				templatePolicy(t, "function", "input.name == 'tpl'", nil)
				_, err := renderWithPolicy(t, `{{ tpl "{{ .name }}" . }}`, withContext)
				require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
			})

			t.Run("tmpl.Inline denied by method name", func(t *testing.T) {
				templatePolicy(t, "function", "input.name == 'tmpl.Inline'", nil)
				_, err := renderWithPolicy(t, `{{ tmpl.Inline "{{ .name }}" . }}`, withContext)
				require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
			})

			t.Run("tmpl.Exec denied by method name", func(t *testing.T) {
				templatePolicy(t, "function", "input.name == 'tmpl.Exec'", nil)
				_, err := renderWithPolicy(t, `{{ define "x" }}{{ .name }}{{ end }}{{ tmpl.Exec "x" . }}`, withContext)
				require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
			})

			t.Run("functions inside an inline template stay guarded", func(t *testing.T) {
				templatePolicy(t, "function", "input.name == 'upper'", nil)
				_, err := renderWithPolicy(t, `{{ tpl "{{ upper .name }}" . }}`, withContext)
				require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
			})

			t.Run("tmpl.Path and tmpl.PathDir denied by method name", func(t *testing.T) {
				templatePolicy(t, "function", "input.name == 'tmpl.Path' || input.name == 'tmpl.PathDir'", nil)
				_, err := renderWithPolicy(t, `{{ tmpl.Path }}`, withContext)
				require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
				_, err = renderWithPolicy(t, `{{ tmpl.PathDir }}`, withContext)
				require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
			})

			// Negative path: rules that match other functions admit tmpl/tpl.
			t.Run("admitted when no rule matches", func(t *testing.T) {
				templatePolicy(t, "function", "input.name == 'never.called'", nil)
				out, err := renderWithPolicy(t, `{{ tpl "{{ .name }}" . }}-{{ tmpl.Inline "{{ .name }}" . }}`, withContext)
				require.NoError(t, err)
				require.Equal(t, "atmos-atmos", out)
				out, err = renderWithPolicy(t, `{{ define "x" }}{{ .name }}{{ end }}{{ tmpl.Exec "x" . }}`, withContext)
				require.NoError(t, err)
				require.Equal(t, "atmos", out)
				_, err = renderWithPolicy(t, `{{ tmpl.Path }}{{ tmpl.PathDir }}`, withContext)
				require.NoError(t, err)
			})
		})
	}
}
