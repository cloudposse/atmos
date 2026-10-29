package main

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	pluginv1 "github.com/cloudposse/atmos/sdk/proto/atmos/plugin/v1"
)

func TestEnvironmentTemplates(t *testing.T) {
	expires := timestamppb.New(time.Now().Add(time.Minute))
	upstream := &pluginv1.Credentials{Type: "secret-values/v1", ExpiresAt: expires, Data: object(t, map[string]any{
		"api-access-key": "private-access", "secret": "private-secret", "unused": "not-exported",
	})}
	request := &pluginv1.IdentityRequest{Spec: object(t, map[string]any{"env": map[string]any{
		"ACCESS":        `{{ index .credentials "api-access-key" }}`,
		"SECRET":        `{{ .credentials.secret }}`,
		"AUTHORIZATION": `Bearer {{ index .credentials "api-access-key" }}:{{ .credentials.secret }}`,
		"LITERAL":       "secret",
	}})}
	_, err := (&identity{}).Validate(t.Context(), request)
	require.NoError(t, err)
	result, err := (&identity{}).authenticate(&pluginv1.IdentityAuthenticateRequest{Identity: request, Upstream: upstream})
	require.NoError(t, err)
	require.Equal(t, map[string]any{"ACCESS": "private-access", "SECRET": "private-secret", "AUTHORIZATION": "Bearer private-access:private-secret", "LITERAL": "secret"}, result.GetData().AsMap())
	require.Equal(t, expires, result.GetExpiresAt())
	require.Equal(t, "environment/v1", result.GetType())
}

func TestEnvironmentTemplatesRejectUnsafeOrMissingValues(t *testing.T) {
	for _, source := range []string{
		`{{ .credentials.missing }}`, `{{ index .credentials "missing" }}`,
		`{{ index . "missing" }}`, `{{ index .credentials "token" "missing" }}`,
		`{{ .credentials.token`, `{{ env "TOKEN" }}`, `{{ exec "command" }}`,
		`{{ readFile "secret" }}`, "nul\x00byte",
		strings.Repeat("x", (1<<20)+1), `{{ .credentials.large }}x`,
		`{{ .credentials.token }}{{ index .credentials "missing" }}`,
	} {
		t.Run(source[:min(len(source), 70)], func(t *testing.T) {
			upstream := &pluginv1.Credentials{Type: "secret-values/v1", ExpiresAt: timestamppb.New(time.Now().Add(time.Minute)), Data: object(t, map[string]any{"token": "must-not-leak", "large": strings.Repeat("x", 1<<20)})}
			_, err := (&identity{}).authenticate(&pluginv1.IdentityAuthenticateRequest{
				Identity: &pluginv1.IdentityRequest{Spec: object(t, map[string]any{"env": map[string]any{"TOKEN": source}})}, Upstream: upstream,
			})
			require.Error(t, err)
			require.NotContains(t, err.Error(), "must-not-leak")
		})
	}
}

func TestEnvironmentTemplatesRejectLegacyPrincipal(t *testing.T) {
	_, err := (&identity{}).Validate(t.Context(), &pluginv1.IdentityRequest{Principal: object(t, map[string]any{"environment": map[string]any{"TOKEN": "token"}})})
	require.ErrorContains(t, err, "spec.env")
}

func TestEnvironmentTemplatesBoundsAndStrictIndex(t *testing.T) {
	large := strings.Repeat("x", maxEnvironmentValueBytes)
	upstream := &pluginv1.Credentials{Type: "secret-values/v1", ExpiresAt: timestamppb.New(time.Now().Add(time.Minute)), Data: object(t, map[string]any{"token": large})}
	result, err := (&identity{}).authenticate(&pluginv1.IdentityAuthenticateRequest{Identity: &pluginv1.IdentityRequest{Spec: identitySpec(t)}, Upstream: upstream})
	require.NoError(t, err)
	require.Equal(t, large, result.GetData().AsMap()["GITHUB_TOKEN"])

	env := make(map[string]any)
	for index := range 257 {
		env[fmt.Sprintf("TOKEN_%d", index)] = "literal"
	}
	_, err = environmentTemplates(&pluginv1.IdentityRequest{Spec: object(t, map[string]any{"env": env})})
	require.Error(t, err)

	values := map[string]any{"items": []string{"first", "last"}, "token": "abc"}
	value, err := strictIndex(values, "items", 1)
	require.NoError(t, err)
	require.Equal(t, "last", value)
	for _, indexes := range [][]any{{"missing"}, {"items", -1}, {"items", 2}, {"items", "bad"}, {true}, {"token", "bad"}} {
		_, err := strictIndex(values, indexes...)
		require.Error(t, err)
	}
	_, err = strictIndex(nil, "key")
	require.Error(t, err)
	_, err = strictIndex(nil)
	require.Error(t, err)
}
