package utils

import (
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/assert"
)

func TestCommandEnvToMap(t *testing.T) {
	type args struct {
		envs []schema.CommandEnv
	}
	tests := []struct {
		name string
		args args
		want map[string]string
	}{
		{
			name: "empty input returns empty map",
			args: args{envs: []schema.CommandEnv{}},
			want: map[string]string{},
		},
		{
			name: "single entry maps key to value",
			args: args{envs: []schema.CommandEnv{{Key: "FOO", Value: "bar"}}},
			want: map[string]string{"FOO": "bar"},
		},
		{
			name: "multiple entries map correctly",
			args: args{envs: []schema.CommandEnv{
				{Key: "FOO", Value: "bar"},
				{Key: "HELLO", Value: "world"},
			}},
			want: map[string]string{
				"FOO":   "bar",
				"HELLO": "world",
			},
		},
		{
			name: "duplicate keys: last wins",
			args: args{envs: []schema.CommandEnv{
				{Key: "FOO", Value: "first"},
				{Key: "FOO", Value: "second"},
				{Key: "FOO", Value: "third"},
			}},
			want: map[string]string{"FOO": "third"},
		},
		{
			name: "key casing is preserved and ValueCommand is ignored",
			args: args{envs: []schema.CommandEnv{
				{Key: "Aws_Profile", Value: "val1", ValueCommand: "echo ignored"},
				{Key: "aws_profile", Value: "val2"},
			}},
			want: map[string]string{
				// Both keys should exist independently due to casing differences
				"Aws_Profile": "val1",
				"aws_profile": "val2",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, CommandEnvToMap(tt.args.envs), "CommandEnvToMap(%v)", tt.args.envs)
		})
	}
}
