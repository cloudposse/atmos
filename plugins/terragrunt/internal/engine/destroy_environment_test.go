package engine

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPreventDestroyEnvironment(t *testing.T) {
	for _, test := range []struct {
		name    string
		env     map[string]string
		args    []string
		extra   string
		blocked bool
	}{
		{name: "command environment", env: map[string]string{"TF_CLI_ARGS_apply": "-destroy"}, blocked: true},
		{name: "global environment", env: map[string]string{"TF_CLI_ARGS": "-destroy"}, blocked: true},
		{name: "quoted and whitespace", env: map[string]string{"TF_CLI_ARGS_apply": "-lock=false\t\"-destroy=true\"\n"}, blocked: true},
		{name: "explicit false wins", env: map[string]string{"TF_CLI_ARGS_apply": "-destroy"}, args: []string{"-destroy=false"}},
		{name: "global inserted after command", env: map[string]string{"TF_CLI_ARGS_apply": "-destroy=false", "TF_CLI_ARGS": "-destroy"}, blocked: true},
		{name: "global false wins", env: map[string]string{"TF_CLI_ARGS_apply": "-destroy", "TF_CLI_ARGS": "-destroy=false"}},
		{name: "other command ignored", env: map[string]string{"TF_CLI_ARGS_plan": "-destroy"}},
		{name: "extra environment", extra: `extra_arguments "env" {
   commands = ["apply"]
   env_vars = { TF_CLI_ARGS_apply = "-destroy" }
  }`, blocked: true},
		{name: "extra clears inherited", env: map[string]string{"TF_CLI_ARGS_apply": "-destroy"}, extra: `extra_arguments "env" {
   commands = ["apply"]
   env_vars = { TF_CLI_ARGS_apply = "" }
  }`},
		{name: "extra arguments win", env: map[string]string{"TF_CLI_ARGS_apply": "-destroy"}, extra: `extra_arguments "args" {
   commands = ["apply"]
   arguments = ["-destroy=false"]
  }`},
		{name: "last extra environment wins", extra: `extra_arguments "first" {
   commands = ["apply"]
   env_vars = { TF_CLI_ARGS_apply = "-destroy=false" }
  }
  extra_arguments "last" {
   commands = ["apply"]
   env_vars = { TF_CLI_ARGS_apply = "-destroy" }
  }`, blocked: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := executionUnit(t, test.extra)
			data, err := os.ReadFile(request.Path)
			require.NoError(t, err)
			data = append(data, []byte("\nprevent_destroy = true\n")...)
			if test.blocked {
				data = append(data, []byte("inputs = { value = run_cmd(\"must-not-execute\") }\n")...)
			}
			require.NoError(t, os.WriteFile(request.Path, data, 0o600))
			request.Command, request.Arguments = "apply", test.args
			if test.env != nil {
				request.Environment = test.env
			}
			_, err = Resolve(t.Context(), request)
			if test.blocked {
				require.ErrorIs(t, err, ErrPreventDestroy)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestPreventDestroyMalformedEnvironmentFailsClosed(t *testing.T) {
	request := nativeUnit(t, "prevent_destroy = true")
	request.Command = "apply"
	request.Environment["TF_CLI_ARGS_apply"] = "\"-destroy"
	_, err := Resolve(t.Context(), request)
	require.ErrorContains(t, err, "invalid TF_CLI_ARGS_apply")
}
