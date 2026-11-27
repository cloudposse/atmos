package stack

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/stack/adoption"
)

// Each form consumes only its own answers, including across the pre-write review.
func guideAnswers(t *testing.T, answers ...string) {
	t.Helper()
	old := runCreationForm
	runCreationForm = func(ctx context.Context, f *huh.Form) error {
		require.NotEmpty(t, answers, "unexpected prompt")
		input := answers[0]
		answers = answers[1:]
		if input == "ESC" {
			return huh.ErrUserAborted
		}
		return f.WithAccessible(true).WithInput(iotest.OneByteReader(strings.NewReader(input + "\n"))).WithOutput(io.Discard).RunWithContext(ctx)
	}
	t.Cleanup(func() { runCreationForm = old; require.Empty(t, answers) })
}

func guideCommand(t *testing.T, flags ...string) *cobra.Command {
	t.Helper()
	cmd := newStackAddCommand()
	cmd.SetContext(t.Context())
	require.NoError(t, cmd.ParseFlags(flags))
	return cmd
}

func TestGuideFreshPackerAndCorrectSavedOrder(t *testing.T) {
	r := localRegistration(t, "packer")
	t.Chdir(r.Project)
	r.Interactive = true
	r.Stack = "dev"
	out := new(bytes.Buffer)
	r.Output = out
	for _, p := range []string{"common.pkrvars.hcl", "dev.pkrvars.hcl", "image.auto.pkrvars.hcl"} {
		writeAdoptionFile(t, filepath.Join(r.Dir, p), "x = 2")
	}
	guideAnswers(t, "1", "sandbox", "blue", "3", "2", "y")
	require.NoError(t, guideStackAdd(guideCommand(t), &r, r.Project, nil))
	require.Contains(t, out.String(), "Loaded automatically: image.auto.pkrvars.hcl")
	require.Contains(t, out.String(), "--var-file dev.pkrvars.hcl --var-file common.pkrvars.hcl")
	require.FileExists(t, filepath.Join(r.Project, "stacks", "sandbox.yaml"))
	r.Config = localConfig(&r)
	t.Run("restore and replace references after source removal", func(t *testing.T) {
		require.NoError(t, os.Remove(filepath.Join(r.Dir, "dev.pkrvars.hcl")))
		r.VarFiles = nil
		r.DryRun = false
		guideAnswers(t, "2", "2", "y")
		require.NoError(t, guideStackAdd(guideCommand(t, "-s", "sandbox", "--name", "blue"), &r, r.Project, []string{r.Dir}))
		content, err := os.ReadFile(filepath.Join(r.Project, "stacks", "sandbox.yaml"))
		require.NoError(t, err)
		require.Contains(t, string(content), "common.pkrvars.hcl")
		require.NotContains(t, string(content), "dev.pkrvars.hcl")
	})
}

func TestGuideHelmDefaultsAndCancellation(t *testing.T) {
	for _, end := range []string{"y", "n", "ESC"} {
		t.Run(end, func(t *testing.T) {
			r := localRegistration(t, "helm")
			r.Interactive = true
			t.Chdir(r.Project)
			guideAnswers(t, "", "", "", "", end)
			err := guideStackAdd(guideCommand(t), &r, r.Project, []string{r.Dir})
			if end != "y" {
				require.ErrorIs(t, err, errUtils.ErrUserAborted)
				require.NoFileExists(t, filepath.Join(r.Project, "atmos.yaml"))
				return
			}
			require.NoError(t, err)
			require.FileExists(t, filepath.Join(r.Project, "stacks", "dev.yaml"))
		})
	}
}

func TestGuideMissingPathAndManualPath(t *testing.T) {
	root := t.TempDir()
	r := adoption.Registration{Adapters: stackAdapters()}
	_, err := adoptionPath(t.Context(), &r, root, nil)
	require.ErrorContains(t, err, "atmos stack add ./path")
	r.Interactive = true
	t.Run("empty repo", func(t *testing.T) {
		guideAnswers(t, "../another/chart")
		s, err := adoptionPath(t.Context(), &r, root, nil)
		require.NoError(t, err)
		require.Equal(t, "../another/chart", s)
	})
	writeAdoptionFile(t, filepath.Join(root, "dev", "main.tf"), "")
	writeAdoptionFile(t, filepath.Join(root, "prod", "main.tf"), "")
	for _, tc := range []struct{ pick, want string }{{"2", "prod"}, {"3", "manual"}, {"ESC", ""}} {
		t.Run(tc.pick, func(t *testing.T) {
			inputs := []string{tc.pick}
			if tc.want == "manual" {
				inputs = append(inputs, "manual")
			}
			guideAnswers(t, inputs...)
			result, err := adoptionPath(t.Context(), &r, root, nil)
			if tc.pick == "ESC" {
				require.ErrorIs(t, err, errUtils.ErrUserAborted)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, result)
		})
	}
}

func TestGuideNoDuplicatePrompts(t *testing.T) {
	r := localRegistration(t, "packer")
	t.Chdir(r.Project)
	r.Name = "configured"
	r.Interactive = true
	r.DryRun = true
	writeAdoptionFile(t, filepath.Join(r.Dir, "dev.pkrvars.hcl"), "x=1")
	r.VarFiles = []string{"dev.pkrvars.hcl"}
	cmd := guideCommand(t, "--name", "configured", "-s", "dev", "--var-file", "dev.pkrvars.hcl")
	guideAnswers(t)
	require.NoError(t, guideStackAdd(cmd, &r, r.Project, []string{r.Dir}))
	r.Interactive = false
	require.NoError(t, guideStackAdd(cmd, &r, r.Project, []string{r.Dir}))
	_, err := adoptionPath(t.Context(), &r, r.Project, []string{"./x"})
	require.NoError(t, err)
}

func TestGuideRegistrationFailures(t *testing.T) {
	r := localRegistration(t, "helm")
	t.Chdir(r.Project)
	r.Interactive = true
	r.DryRun = false
	for _, args := range [][]string{{"missing"}, {r.Dir}} {
		r.Workspace = "other"
		require.Error(t, guideStackAdd(guideCommand(t), &r, r.Project, args))
	}
	r.Workspace = ""
	for _, i := range []int{0, 1, 2, 3} {
		t.Run(string(rune('a'+i)), func(t *testing.T) {
			local := r
			ans := append(make([]string, i), "ESC")
			guideAnswers(t, ans...)
			require.ErrorIs(t, guideStackAdd(guideCommand(t), &local, r.Project, []string{local.Dir}), errUtils.ErrUserAborted)
		})
	}
}

type legacyGuideAdapter struct {
	dryRuns, writes int
	interactive     bool
}

func (*legacyGuideAdapter) Kind() string      { return "example" }
func (*legacyGuideAdapter) Options() []string { return nil }
func (*legacyGuideAdapter) Detect(s adoption.Source) (adoption.Match, error) {
	return adoption.Match{Path: s.Path}, nil
}

func (a *legacyGuideAdapter) Adopt(_ context.Context, r *adoption.Registration, _ string) error {
	if r.DryRun {
		a.dryRuns++
	} else {
		a.writes++
		a.interactive = r.Interactive
	}
	return nil
}

func TestGuidePreservesLegacyAdapter(t *testing.T) {
	root := t.TempDir()
	a := &legacyGuideAdapter{}
	r := adoption.Registration{Adapters: adoption.Adapters{a}, Project: root, Dir: root, Stack: "dev", Output: io.Discard, Interactive: true}
	guideAnswers(t, "test", "y")
	require.NoError(t, guideStackAdd(guideCommand(t), &r, root, []string{root}))
	require.Equal(t, 1, a.dryRuns)
	require.Equal(t, 1, a.writes)
	require.True(t, a.interactive)
}

func TestGuideSavedInputChoices(t *testing.T) {
	r := localRegistration(t, "packer")
	r.Name = "image"
	r.VarFiles = []string{"first.pkrvars.hcl", "second.pkrvars.hcl"}
	for _, f := range r.VarFiles {
		writeAdoptionFile(t, filepath.Join(r.Dir, f), "x = 1")
	}
	require.NoError(t, r.Save(t.Context()))
	r.Config = localConfig(&r)
	r.VarFiles = nil
	t.Run("keep", func(t *testing.T) {
		guideAnswers(t, "1")
		chose, err := chooseAdoptionInputs(t.Context(), &r, r.Dir, "packer")
		require.NoError(t, err)
		require.True(t, chose)
		require.Equal(t, []string{"first.pkrvars.hcl", "second.pkrvars.hcl"}, r.VarFiles)
	})
	t.Run("cancel saved choice", func(t *testing.T) {
		r.VarFiles = nil
		guideAnswers(t, "ESC")
		_, err := chooseAdoptionInputs(t.Context(), &r, r.Dir, "packer")
		require.ErrorIs(t, err, errUtils.ErrUserAborted)
	})
	t.Run("explicit empty selection", func(t *testing.T) {
		r.VarFiles = nil
		r.ReplaceVarFiles = true
		guideAnswers(t, "1")
		chose, err := chooseAdoptionInputs(t.Context(), &r, r.Dir, "packer")
		require.NoError(t, err)
		require.True(t, chose)
		require.Empty(t, r.VarFiles)
	})
	t.Run("cancel fresh choice", func(t *testing.T) {
		guideAnswers(t, "ESC")
		_, err := chooseAdoptionInputs(t.Context(), &r, r.Dir, "packer")
		require.ErrorIs(t, err, errUtils.ErrUserAborted)
	})
	t.Run("invalid root", func(t *testing.T) {
		_, err := chooseAdoptionInputs(t.Context(), &r, filepath.Join(r.Dir, "missing"), "packer")
		require.Error(t, err)
	})
}

func TestAdoptionTextRejectsInvalidName(t *testing.T) {
	guideAnswers(t, "bad/name\nok")
	name, err := adoptionText(t.Context(), "Name", "dev", adoption.ValidName)
	require.NoError(t, err)
	require.Equal(t, "ok", name)
}

func TestDiscoverCoLocatedAtlantisAndTerraform(t *testing.T) {
	root := t.TempDir()
	writeAdoptionFile(t, filepath.Join(root, "main.tf"), "")
	file := filepath.Join(root, "atlantis.yaml")
	writeAdoptionFile(t, file, "version: 3\nprojects: []\n")
	found, err := stackAdapters().Discover(t.Context(), root)
	require.NoError(t, err)
	require.Equal(t, []adoption.Candidate{{Path: file, Kind: "atlantis"}}, found)
}
