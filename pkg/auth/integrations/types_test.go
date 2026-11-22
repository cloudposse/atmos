package integrations

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/auth/realm"
)

func TestIntegrationConfig_RealmExplicit(t *testing.T) {
	for _, tt := range []struct {
		name, value, source string
		want                bool
	}{
		{name: "empty realm", source: realm.SourceConfig},
		{name: "derived realm", value: "462813558259d41c", source: realm.SourceAuto},
		{name: "config realm", value: "acme", source: realm.SourceConfig, want: true},
		{name: "env realm", value: "acme", source: realm.SourceEnv, want: true},
		{name: "realm without source", value: "acme", want: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			config := &IntegrationConfig{Realm: tt.value, RealmSource: tt.source}
			assert.Equal(t, tt.want, config.RealmExplicit())
		})
	}
}
