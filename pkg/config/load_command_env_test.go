package config

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/config/casemap"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestRestoreCommandEnvCase(t *testing.T) {
	caseMaps := casemap.New()
	caseMaps.Set(envKey, casemap.CaseMap{
		"path":  "PATH",
		"gobin": "GOBIN",
	})

	atmosConfig := &schema.AtmosConfiguration{
		CaseMaps: caseMaps,
		Commands: []schema.Command{
			{
				Name: "casts",
				Env: []schema.CommandEnv{
					{Key: "path", Value: "/tmp/bin"},
					{Key: "gobin", Value: "/tmp/bin"},
				},
				Commands: []schema.Command{
					{
						Name: "generate",
						Env:  []schema.CommandEnv{{Key: "path", Value: "/tmp/bin"}},
					},
				},
			},
		},
	}

	restoreCaseSensitiveEnvMaps(atmosConfig)

	assert.Equal(t, "PATH", atmosConfig.Commands[0].Env[0].Key)
	assert.Equal(t, "GOBIN", atmosConfig.Commands[0].Env[1].Key)
	assert.Equal(t, "PATH", atmosConfig.Commands[0].Commands[0].Env[0].Key)
}
