package main

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// recorder is an entry whose two roles only record that they ran.
type recorder struct {
	agentRoots []string
	agentErr   error
	pluginRuns int
	stderr     bytes.Buffer
}

func (r *recorder) entry() entry {
	return entry{
		agent: func(_ context.Context, root string) error {
			r.agentRoots = append(r.agentRoots, root)
			return r.agentErr
		},
		plugin: func() { r.pluginRuns++ },
		stderr: &r.stderr,
	}
}

func TestAgentServeDoesNotStartThePluginHandshake(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantRoot string
	}{
		{"no flags", []string{"agent-serve"}, ""},
		{"root with a space", []string{"agent-serve", "--root", "/data/ssh"}, "/data/ssh"},
		{"root with equals", []string{"agent-serve", "--root=/data/ssh"}, "/data/ssh"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &recorder{}

			code := dispatch(tt.args, r.entry())

			assert.Equal(t, 0, code)
			assert.Equal(t, []string{tt.wantRoot}, r.agentRoots)
			assert.Zero(t, r.pluginRuns, "plugin.Main would exit for lack of the magic cookie")
		})
	}
}

func TestAnythingElseStartsThePluginServer(t *testing.T) {
	for _, args := range [][]string{nil, {}, {"key", "list"}, {"--root", "/x"}, {"agent-serve-not"}} {
		r := &recorder{}

		code := dispatch(args, r.entry())

		assert.Equal(t, 0, code)
		assert.Equal(t, 1, r.pluginRuns, "args %v", args)
		assert.Empty(t, r.agentRoots)
	}
}

func TestAgentServeRejectsUnknownArguments(t *testing.T) {
	for _, args := range [][]string{{"agent-serve", "--config", "x"}, {"agent-serve", "--root"}, {"agent-serve", "stray"}} {
		r := &recorder{}

		code := dispatch(args, r.entry())

		assert.Equal(t, 2, code, "args %v", args)
		assert.Empty(t, r.agentRoots)
		assert.Zero(t, r.pluginRuns)
		assert.Contains(t, r.stderr.String(), "usage: agent-serve [--root <dir>]")
	}
}

func TestAgentServeReportsAFailure(t *testing.T) {
	r := &recorder{agentErr: errors.New("socket is already in use")}

	code := dispatch([]string{"agent-serve"}, r.entry())

	assert.Equal(t, 1, code)
	assert.Contains(t, r.stderr.String(), "socket is already in use")
}

func TestTheDescriptionAgreesWithTheManifest(t *testing.T) {
	var manifest struct {
		Metadata struct {
			Name    string `yaml:"name"`
			Version string `yaml:"version"`
		} `yaml:"metadata"`
		Spec struct {
			Capabilities struct {
				Commands map[string]any `yaml:"commands"`
				Auth     struct {
					Providers  map[string]any `yaml:"providers"`
					Identities map[string]any `yaml:"identities"`
				} `yaml:"auth"`
			} `yaml:"capabilities"`
		} `yaml:"spec"`
	}
	require.NoError(t, yaml.Unmarshal(manifestData, &manifest))

	d := description()

	assert.Equal(t, "cloudposse/ssh", d.GetPluginName())
	assert.Equal(t, manifest.Metadata.Name, d.GetPluginName())
	assert.Equal(t, manifest.Metadata.Version, d.GetPluginVersion())
	assert.Equal(t, manifest.Metadata.Version, pluginVersion())
	assert.Regexp(t, `^sha256:[0-9a-f]{64}$`, d.GetManifestDigest())

	want := map[string]bool{}
	for kind := range manifest.Spec.Capabilities.Auth.Providers {
		want["auth.provider/"+kind] = true
	}
	for kind := range manifest.Spec.Capabilities.Auth.Identities {
		want["auth.identity/"+kind] = true
	}
	for kind := range manifest.Spec.Capabilities.Commands {
		want["command/"+kind] = true
	}
	got := map[string]bool{}
	for _, e := range d.GetExports() {
		got[e.GetService()+"/"+e.GetKind()] = true
	}
	assert.Equal(t, want, got, "exports must equal the manifest's capabilities")
	assert.Equal(t, map[string]bool{"auth.provider/ssh/self": true, "auth.identity/ssh/key": true, "command/ssh": true}, got)
	assert.NotContains(t, got, "auth.provider/github/user", "the GitHub device login is deferred")

	services := map[string]bool{}
	for _, s := range d.GetServices() {
		services[s.GetService()] = true
	}
	assert.Equal(t, map[string]bool{"auth.provider": true, "auth.identity": true, "command": true}, services)
}
