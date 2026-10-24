package starlark

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/script"
)

func configTestHTTP(t *testing.T, handler http.HandlerFunc) (*Engine, string) {
	t.Helper()
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)
	engine := New()
	engine.schemaHTTP.Transport = server.Client().Transport
	return engine, server.URL
}

func TestConfigRemoteSchema(t *testing.T) {
	t.Parallel()
	var rootReads, refReads atomic.Int32
	engine, base := configTestHTTP(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/schemas/app.json":
			rootReads.Add(1)
			_, _ = fmt.Fprint(w, `{"$schema":"http://json-schema.org/draft-07/schema#","type":"object","properties":{"port":{"$ref":"types/port.yaml"}},"required":["port"],"additionalProperties":false}`)
		case "/schemas/types/port.yaml":
			refReads.Add(1)
			_, _ = fmt.Fprint(w, "type: integer\nminimum: 1\nmaximum: 65535")
		default:
			http.NotFound(w, r)
		}
	})
	dir := writeModules(t, map[string]string{"app.yaml": "port: 5432", "bad.json": "{\n\"port\": \"bad\"\n}"})
	result, err := engine.Execute(t.Context(), script.Spec{WorkingDirectory: dir, Source: fmt.Sprintf(`
url = %q
def read():
    return config.load("app.yaml",schema=url).port
output = steps.parallel(functions=[read,read])
`, base+"/schemas/app.json")})
	require.NoError(t, err)
	assert.JSONEq(t, `[5432,5432]`, result.Value)
	assert.Equal(t, int32(2), rootReads.Load())
	assert.Equal(t, int32(2), refReads.Load())
	_, err = engine.Execute(t.Context(), script.Spec{WorkingDirectory: dir, Source: fmt.Sprintf(`config.load("bad.json",schema=%q)`, base+"/schemas/app.json")})
	require.ErrorContains(t, err, "bad.json:2:9: port: expected integer")
}

func TestConfigRemoteSchemaResponses(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, body, contentType, path, want string
		status                              int
	}{
		{name: "extensionless JSON", body: `{"type":"integer"}`, path: "/schema", status: 200},
		{name: "extensionless YAML", body: "type: integer", contentType: "application/yaml; charset=utf-8", path: "/schema", status: 200},
		{name: "not found", body: "not found", path: "/schema.json", status: 404, want: "HTTP 404"},
		{name: "size limit", body: strings.Repeat(" ", configSchemaMaxBytes+1), path: "/schema.json", status: 200, want: "byte limit"},
		{name: "malformed JSON", body: "{", path: "/schema.json", status: 200, want: "invalid JSON"},
		{name: "local ref", body: `{"$ref":"file:///private/schema.json"}`, path: "/schema.json", status: 200, want: "cannot reference local files"},
		{name: "local id", body: `{"$id":"file:///private/","$ref":"schema.json"}`, path: "/schema.json", status: 200, want: "cannot reference local files"},
		{name: "nested local ref", body: `{"allOf":[{"$ref":"file:///private/schema.json"}]}`, path: "/schema.json", status: 200, want: "cannot reference local files"},
		{name: "local meta schema", body: `{"$schema":"file:///private/schema.json"}`, path: "/schema.json", status: 200, want: "cannot reference local files"},
		{name: "insecure reference", body: `{"$ref":"http://example.com/schema.json"}`, path: "/schema.json", status: 200, want: "local files or HTTPS"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			engine, base := configTestHTTP(t, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", tc.contentType)
				w.WriteHeader(tc.status)
				_, _ = fmt.Fprint(w, tc.body)
			})
			engine.readFile = func(string) ([]byte, error) { t.Error("unexpected disk read from remote schema"); return nil, nil }
			result, err := engine.Execute(t.Context(), script.Spec{Source: fmt.Sprintf(`output = config.validate(42,schema=%q)`, base+tc.path)})
			if tc.want != "" {
				require.ErrorContains(t, err, tc.want)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, "42", result.Value)
		})
	}
}

func TestConfigRemoteSchemaRedirects(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, target, want string }{
		{"HTTPS", "/ok.json", ""},
		{"downgrade", "http://example.com/schema.json", "must use HTTPS"},
		{"credentials", "https://user:secret@example.com/schema.json", "without embedded credentials"},
		{"loop", "/redirect.json", "redirect limit exceeded"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			engine, base := configTestHTTP(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/ok.json" {
					_, _ = fmt.Fprint(w, `{"type":"integer"}`)
					return
				}
				http.Redirect(w, r, tc.target, http.StatusFound)
			})
			result, err := engine.Execute(t.Context(), script.Spec{Source: fmt.Sprintf(`output = config.validate(42,schema=%q)`, base+"/redirect.json")})
			if tc.want != "" {
				require.ErrorContains(t, err, tc.want)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, "42", result.Value)
		})
	}
}

func TestConfigRemoteSchemaCancellation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	engine, base := configTestHTTP(t, func(_ http.ResponseWriter, r *http.Request) {
		cancel()
		<-r.Context().Done()
	})
	_, err := engine.Execute(ctx, script.Spec{Source: fmt.Sprintf(`config.validate({},schema=%q)`, base+"/schema.json")})
	require.ErrorIs(t, err, context.Canceled)
}

func TestConfigRemoteSchemaTimeout(t *testing.T) {
	t.Parallel()
	engine, base := configTestHTTP(t, func(_ http.ResponseWriter, r *http.Request) { <-r.Context().Done() })
	engine.schemaHTTP.Timeout = 50 * time.Millisecond
	_, err := engine.Execute(t.Context(), script.Spec{Source: fmt.Sprintf(`config.validate({},schema=%q)`, base+"/schema.json")})
	require.ErrorContains(t, err, "Client.Timeout")
}

func TestConfigRemoteSchemaResourceLimit(t *testing.T) {
	t.Parallel()
	engine, base := configTestHTTP(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, `{"$ref":%q}`, r.URL.Path+"/next")
	})
	_, err := engine.Execute(t.Context(), script.Spec{Source: fmt.Sprintf(`config.validate({},schema=%q)`, base+"/schema")})
	require.ErrorContains(t, err, "resource limit")
}

func TestConfigRemoteSchemaDryRun(t *testing.T) {
	t.Parallel()
	engine, base := configTestHTTP(t, func(http.ResponseWriter, *http.Request) { t.Error("unexpected HTTP request") })
	_, err := engine.Execute(t.Context(), script.Spec{DryRun: true, Source: fmt.Sprintf(`config.load("missing.yaml",schema=%q)`, base+"/schema.json")})
	require.NoError(t, err)
}
