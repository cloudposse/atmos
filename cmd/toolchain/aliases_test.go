package toolchain

import (
	"bytes"
	"encoding/json"
	"io"
	"maps"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/toolchain"
)

func TestEffectiveToolchainAliases(t *testing.T) {
	builtinBefore := maps.Clone(toolchain.BuiltinAliases)
	config := &schema.AtmosConfiguration{Toolchain: schema.Toolchain{Aliases: map[string]string{
		"tofu": "example/opentofu", "infra": "tofu", "unknown": "unlisted",
	}}}
	configuredBefore := maps.Clone(config.Toolchain.Aliases)
	got := effectiveAliases(config)
	require.Equal(t, "example/opentofu", got["tofu"])
	require.Equal(t, "cloudposse/atmos", got["atmos"])
	require.Equal(t, "tofu", got["infra"], "targets remain exactly as configured")
	require.Equal(t, "unlisted", got["unknown"], "listing does not require registry resolution")
	got["tofu"] = "changed/result"
	require.Equal(t, builtinBefore, toolchain.BuiltinAliases)
	require.Equal(t, configuredBefore, config.Toolchain.Aliases)
	config.Toolchain.Aliases["infra"] = "changed/source"
	require.Equal(t, "tofu", got["infra"])
	require.Equal(t, builtinBefore, effectiveAliases(nil))
	require.Equal(t, builtinBefore, effectiveAliases(&schema.AtmosConfiguration{}))
}

func TestWriteToolchainAliases(t *testing.T) {
	aliases := map[string]string{"zzz": "example/last", "a": "example/first", "middle": "tofu"}
	for _, format := range []string{"table", "json", "yaml"} {
		t.Run(format, func(t *testing.T) {
			var stdout bytes.Buffer
			setAliasOutput(t, &stdout)
			require.NoError(t, writeAliases(aliases, format))
			first := stdout.String()
			stdout.Reset()
			require.NoError(t, writeAliases(aliases, format))
			require.Equal(t, first, stdout.String(), "reruns must produce identical output")
			switch format {
			case "table":
				require.Equal(t, "ALIAS   TARGET\na       example/first\nmiddle  tofu\nzzz     example/last\n", first)
			case "json":
				var decoded map[string]string
				require.NoError(t, json.Unmarshal(stdout.Bytes(), &decoded))
				require.Equal(t, aliases, decoded)
			case "yaml":
				var decoded map[string]string
				require.NoError(t, yaml.Unmarshal(stdout.Bytes(), &decoded))
				require.Equal(t, aliases, decoded)
			}
		})
	}
}

func TestWriteToolchainAliasesErrors(t *testing.T) {
	for _, format := range []string{"table", "json", "yaml"} {
		t.Run(format, func(t *testing.T) {
			setAliasOutput(t, aliasFailWriter{})
			require.ErrorIs(t, writeAliases(map[string]string{"tofu": "opentofu/opentofu"}, format), io.ErrClosedPipe)
		})
	}
	var stdout bytes.Buffer
	setAliasOutput(t, &stdout)
	require.ErrorIs(t, writeAliases(nil, "xml"), errUtils.ErrInvalidFlagValue)
	require.Empty(t, stdout.String())
	require.Equal(t, "ALIAS  TARGET\n", formatAliasesTable(nil))
}

func setAliasOutput(t *testing.T, output io.Writer) {
	t.Helper()
	ioCtx, err := iolib.NewContext(iolib.WithStreams(&aliasStreams{output: output}))
	require.NoError(t, err)
	data.InitWriter(ioCtx)
	t.Cleanup(data.Reset)
}

type aliasStreams struct{ output io.Writer }

func (s *aliasStreams) Input() io.Reader     { return nil }
func (s *aliasStreams) Output() io.Writer    { return s.output }
func (s *aliasStreams) Error() io.Writer     { return io.Discard }
func (s *aliasStreams) RawOutput() io.Writer { return s.output }
func (s *aliasStreams) RawError() io.Writer  { return io.Discard }

type aliasFailWriter struct{}

func (aliasFailWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }
