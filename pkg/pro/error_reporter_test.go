package pro

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestExceptionMetadataPrecedenceAndMasking(t *testing.T) {
	require.NoError(t, iolib.Initialize())
	iolib.RegisterSecret("private-label-value")
	info := schema.ConfigAndStacksInfo{
		Component: "implementation", ComponentFromArg: "vpc", Stack: "prod", ComponentType: "terraform",
		ComponentMetadataSection: map[string]any{"tags": []any{"production", "team", "invalid key"}, "labels": map[string]any{"team": "platform", "credential": "private-label-value", "atmos.component": "spoofed", "too-long": strings.Repeat("x", 201)}},
	}
	event := buildExceptionEvent(errors.New("failure private-label-value"), &info, map[string]string{"team": "configured"}, map[string]string{"execution_id": "invocation"})
	event.Fingerprint = []string{"same", "fingerprint"}
	masked, err := maskedExceptionEvent(event)
	require.NoError(t, err)
	assert.Equal(t, "configured", masked.Tags["team"])
	assert.Equal(t, "true", masked.Tags["production"])
	assert.Equal(t, "vpc", masked.Tags["atmos.component"])
	assert.Equal(t, "invocation", masked.Tags["atmos.execution_id"])
	assert.Equal(t, event.Fingerprint, masked.Fingerprint)
	assert.Equal(t, event.EventID, masked.EventID)
	assert.NotContains(t, masked.Tags, "invalid key")
	assert.NotContains(t, masked.Tags, "too-long")
	assert.NotContains(t, masked.Tags, "atmos.label.team")
	encoded, err := json.Marshal(masked)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "private-label-value")
	assert.Contains(t, string(encoded), "invalid key", "invalid tags remain in the masked metadata context")
	assert.Contains(t, string(encoded), strings.Repeat("x", 201))
	assert.Equal(t, "platform", info.ComponentMetadataSection["labels"].(map[string]any)["team"])
}

func TestErrorReporterProEnablement(t *testing.T) {
	yes, no := true, false
	for _, tc := range []struct {
		name      string
		global    bool
		report    *bool
		component map[string]any
		env       string
		want      bool
	}{
		{name: "off by default"},
		{name: "automatic", global: true, want: true},
		{name: "global opt out", global: true, report: &no},
		{name: "component opt in", component: map[string]any{"enabled": true}, want: true},
		{name: "component opt out", global: true, component: map[string]any{"enabled": false}},
		{name: "component errors off", global: true, component: map[string]any{"errors": map[string]any{"enabled": false}}},
		{name: "component errors on", global: true, report: &no, component: map[string]any{"errors": map[string]any{"enabled": true}}, want: true},
		{name: "environment off wins", global: true, report: &yes, env: "false", component: map[string]any{"errors": map[string]any{"enabled": true}}},
		{name: "environment on wins", global: true, report: &no, env: "true", want: true},
		{name: "errors on does not enable Pro", report: &yes, env: "true"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("ATMOS_PRO_ERRORS_ENABLED", tc.env)
			config := &schema.AtmosConfiguration{}
			config.Settings.Pro = schema.ProSettings{Enabled: tc.global, Errors: schema.ProErrorsSettings{Enabled: tc.report}}
			reporter := NewErrorReporter(config, "", ExceptionTransportOptions{})
			assert.Equal(t, tc.want, reporter.proEnabled(&schema.ConfigAndStacksInfo{ComponentSettingsSection: map[string]any{"pro": tc.component}}))
		})
	}
}

func TestErrorReporterFanoutAndDistinctOccurrences(t *testing.T) {
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("GITHUB_REPOSITORY_ID", "123")
	t.Setenv("ATMOS_PRO_ERRORS_ENABLED", "")
	proEvents, sentryEvents := make(chan *sentry.Event, 8), make(chan *sentry.Event, 8)
	handler := func(events chan<- *sentry.Event) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			data, err := io.ReadAll(r.Body)
			assert.NoError(t, err)
			events <- decodeExceptionEnvelope(t, data)
			w.WriteHeader(http.StatusOK)
		})
	}
	proServer := httptest.NewServer(handler(proEvents))
	defer proServer.Close()
	sentryServer := httptest.NewServer(handler(sentryEvents))
	defer sentryServer.Close()
	config := &schema.AtmosConfiguration{}
	config.Settings.Pro = schema.ProSettings{Enabled: true, BaseURL: proServer.URL, GithubOIDC: schema.GithubOIDCSettings{RequestURL: "https://unused.example", RequestToken: "deterministic"}}
	config.Errors.Sentry = schema.SentryConfig{Enabled: true, DSN: strings.Replace(sentryServer.URL, "://", "://public@", 1) + "/1", Tags: map[string]string{"team": "global", "atmos.component": "spoofed"}}
	reporter := NewErrorReporter(config, "invocation", ExceptionTransportOptions{Token: func(context.Context) (string, error) { return "test-token", nil }})
	info := schema.ConfigAndStacksInfo{Component: "vpc", Stack: "prod", ComponentMetadataSection: map[string]any{"tags": []string{"production"}, "labels": map[string]string{"team": "platform"}}, ComponentSettingsSection: map[string]any{"errors": map[string]any{"sentry": map[string]any{"tags": map[string]any{"team": "component"}}}}}
	first := errUtils.WithReportingContext(errors.New("same failure"), &info, "invocation")
	silent := errUtils.WithReportingContext(errUtils.ExitCodeError{Code: 0}, &info, "invocation")
	reporter.Capture(errors.Join(silent, first), nil)
	reporter.Capture(errors.Join(first, first), nil)
	second := errUtils.WithReportingContext(errors.New("same failure"), &info, "invocation")
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); reporter.Capture(second, nil) }()
	}
	wg.Wait()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	reporter.Flush(ctx)
	reporter.Flush(ctx)
	require.Len(t, proEvents, 2)
	require.Len(t, sentryEvents, 2)
	for i := 0; i < 2; i++ {
		p, s := <-proEvents, <-sentryEvents
		assert.Equal(t, p.EventID, s.EventID)
		assert.Equal(t, p.Fingerprint, s.Fingerprint)
		for _, event := range []*sentry.Event{p, s} {
			assert.Equal(t, "component", event.Tags["team"])
			assert.Equal(t, "true", event.Tags["production"])
			assert.Equal(t, "vpc", event.Tags["atmos.component"])
			assert.Equal(t, "invocation", event.Contexts["atmos_execution"]["execution_id"])
		}
	}
}

func TestErrorReporterProOnlyAndEligibility(t *testing.T) {
	for _, tc := range []struct {
		name, actions, repo string
		credentials         bool
		want                int
	}{
		{"Pro without Sentry", "true", "123", true, 1},
		{"local process", "false", "123", true, 0},
		{"missing repository", "true", "", true, 0},
		{"missing OIDC", "true", "123", false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("GITHUB_ACTIONS", tc.actions)
			t.Setenv("GITHUB_REPOSITORY_ID", tc.repo)
			t.Setenv("ATMOS_PRO_ERRORS_ENABLED", "")
			events := make(chan string, 2)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				events <- r.URL.Path
				w.WriteHeader(http.StatusUnauthorized)
			}))
			defer server.Close()
			config := &schema.AtmosConfiguration{}
			config.Settings.Pro = schema.ProSettings{Enabled: true, BaseURL: server.URL}
			if tc.credentials {
				config.Settings.Pro.GithubOIDC = schema.GithubOIDCSettings{RequestURL: "https://unused.example", RequestToken: "test"}
			}
			reporter := NewErrorReporter(config, "invocation", ExceptionTransportOptions{Token: func(context.Context) (string, error) { return "test-token", nil }})
			failure := errUtils.ExitCodeError{Code: 42}
			reporter.Capture(failure, nil)
			reporter.Capture(errUtils.ExitCodeError{Code: 0}, nil)
			reporter.Capture(errUtils.ExitCodeError{Code: 1, Silent: true}, nil)
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			reporter.Flush(ctx)
			assert.Len(t, events, tc.want)
			assert.Equal(t, 42, errUtils.GetExitCode(failure))
		})
	}
}
