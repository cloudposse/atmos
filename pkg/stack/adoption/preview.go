package adoption

import (
	"cmp"
	"fmt"
	"strings"

	"al.essio.dev/pkg/shellescape"

	"github.com/cloudposse/atmos/pkg/perf"
)

// SaveCommand reproduces a registration, including ordered inputs, without prompting.
func (r *Registration) SaveCommand() string {
	defer perf.Track(nil, "adoption.Registration.SaveCommand")()
	path := cmp.Or(r.SourcePath, r.Dir)
	if strings.HasPrefix(path, "-") {
		path = "./" + path
	}
	args := []string{"atmos", "stack", "add", path, "-s", r.Stack}
	for _, flag := range []struct{ name, value string }{
		{"--name", r.Name},
		{"--command", r.Explicit},
		{"--project", r.AtlantisProject},
		{"--release-name", r.ReleaseName},
		{"--namespace", r.Namespace},
		{"--workspace", r.Workspace},
	} {
		if flag.value != "" {
			args = append(args, flag.name, flag.value)
		}
	}
	if r.AtlantisProject == "" {
		for _, file := range r.VarFiles {
			args = append(args, "--var-file", file)
		}
	}
	for _, file := range r.Values {
		args = append(args, "--values", file)
	}
	if r.ShareState {
		args = append(args, "--share-state")
	}
	if r.ReplaceVarFiles {
		args = append(args, "--replace-var-files")
	}
	return shellescape.QuoteCommand(append(args, "--interactive=false"))
}

// PreviewSave reports the exact next command after the adapter has resolved native choices.
func (r *Registration) PreviewSave() {
	defer perf.Track(nil, "adoption.Registration.PreviewSave")()
	fmt.Fprintln(r.Output, "Save:", r.SaveCommand())
	if r.OnPreview != nil {
		r.OnPreview(*r)
	}
}
