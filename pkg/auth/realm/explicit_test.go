package realm

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsExplicitSource(t *testing.T) {
	for source, want := range map[string]bool{
		SourceEnv:        true,
		SourceConfig:     true,
		"":               true,
		SourceAuto:       false,
		SourceConfigPath: false,
		SourceDefault:    false,
	} {
		assert.Equal(t, want, IsExplicitSource(source), "source %q", source)
	}
}

func TestIsExplicit(t *testing.T) {
	for _, tt := range []struct {
		name         string
		info         RealmInfo
		configSource string
		want         bool
	}{
		{name: "empty realm", info: RealmInfo{Value: "", Source: SourceConfig}, configSource: SourceConfig},
		{name: "auto source", info: RealmInfo{Value: "acme", Source: SourceAuto}, configSource: SourceConfig},
		{name: "env realm", info: RealmInfo{Value: "acme", Source: SourceEnv}, configSource: SourceConfigPath, want: true},
		{name: "configured realm", info: RealmInfo{Value: "acme", Source: SourceConfig}, configSource: SourceConfig, want: true},
		{name: "realm set directly", info: RealmInfo{Value: "acme", Source: SourceConfig}, want: true},
		{name: "realm without source", info: RealmInfo{Value: "acme"}, want: true},
		{name: "hash of the config path", info: RealmInfo{Value: "462813558259d41c", Source: SourceConfig}, configSource: SourceConfigPath},
		{name: "default realm", info: RealmInfo{Value: "default", Source: SourceConfig}, configSource: SourceDefault},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsExplicit(tt.info, tt.configSource))
		})
	}
}
