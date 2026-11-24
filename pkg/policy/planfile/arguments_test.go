package planfile

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSelectArtifact(t *testing.T) {
	for name, args := range map[string][]string{
		"plan joined":               {"plan", "-out=selected.plan", "-destroy"},
		"plan separate":             {"plan", "-var-file", "values.json", "-out", "selected.plan"},
		"plan double-dash joined":   {"plan", "--out=selected.plan", "-destroy"},
		"plan double-dash separate": {"plan", "-var-file", "values.json", "--out", "selected.plan"},
		"apply":                     {"apply", "selected.plan"},
		"apply flags":               {"apply", "-input=false", "-lock=true", "-lock-timeout", "1m", "-parallelism=5", "selected.plan"},
	} {
		t.Run(name, func(t *testing.T) {
			directory := t.TempDir()
			selection, err := selectArtifact(&Invocation{Dir: directory, Args: args})
			require.NoError(t, err)
			require.Equal(t, filepath.Join(directory, "selected.plan"), selection.path)
			original := append([]string{}, args...)
			changed := selection.replace(args, "private.plan")
			require.NotEqual(t, args, changed)
			require.Equal(t, original, args)
		})
	}
}

// The private artifact replaces the user's path in the spelling the user wrote.
func TestSelectArtifactReplaceKeepsTheOutputFlagSpelling(t *testing.T) {
	for args, want := range map[[2]string][]string{
		{"plan", "--out=selected.plan"}: {"plan", "--out=private.plan"},
		{"plan", "-out=selected.plan"}:  {"plan", "-out=private.plan"},
	} {
		selection, err := selectArtifact(&Invocation{Dir: t.TempDir(), Args: args[:]})
		require.NoError(t, err)
		require.Equal(t, want, selection.replace(args[:], "private.plan"))
	}
	args := []string{"plan", "--out", "selected.plan"}
	selection, err := selectArtifact(&Invocation{Dir: t.TempDir(), Args: args})
	require.NoError(t, err)
	require.Equal(t, []string{"plan", "--out", "private.plan"}, selection.replace(args, "private.plan"))
}

func TestRejectUnsafeArguments(t *testing.T) {
	for name, args := range map[string][]string{
		"empty": {}, "missing artifact": {"plan"}, "disabled planfile": {"plan", "-no-color"},
		"out no value": {"plan", "-out"}, "empty value": {"plan", "-out="},
		"out duplicate": {"plan", "-out=a", "-out", "b"}, "out flag value": {"plan", "-out", "-no-color"},
		"out duplicate mixed dashes": {"plan", "-out=a", "--out", "b"}, "double-dash out no value": {"plan", "--out"},
		"double-dash empty value": {"plan", "--out="},
		"destroy":                 {"destroy", "-auto-approve"}, "apply no artifact": {"apply", "-auto-approve"},
		"apply extra artifact":  {"apply", "other.plan", "selected.plan"},
		"apply planning":        {"apply", "-target=thing", "selected.plan"},
		"apply missing timeout": {"apply", "-lock-timeout", "selected.plan"},
		"apply invalid timeout": {"apply", "-lock-timeout", "-no-color", "selected.plan"},
		"apply empty":           {"apply", ""},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := selectArtifact(&Invocation{Dir: t.TempDir(), Args: args})
			require.Error(t, err)
		})
	}
	for _, key := range []string{"TF_CLI_ARGS", "TF_CLI_ARGS_plan", "TF_CLI_ARGS_apply", "TF_CLI_ARGS_show"} {
		_, err := selectArtifact(&Invocation{Args: []string{"apply", "selected.plan"}, Env: []string{key + "=-out=other"}})
		require.Error(t, err)
	}
	_, err := selectArtifact(&Invocation{Args: []string{"apply", "selected.plan"}, Env: []string{"TF_CLI_ARGS=", "TF_CLI_ARGS_init=-no-color"}})
	require.NoError(t, err)
}
