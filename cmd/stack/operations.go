package stack

import (
	"path/filepath"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/merge"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	pkgstack "github.com/cloudposse/atmos/pkg/stack"
	"github.com/cloudposse/atmos/pkg/ui"
	u "github.com/cloudposse/atmos/pkg/utils"
	atmosyaml "github.com/cloudposse/atmos/pkg/yaml"
)

// Shared flag values for the stack edit subcommands.
var (
	flagStack     string
	flagComponent string
	flagType      string
	flagFile      string
)

// editTarget holds the resolved file and in-file path for an edit, plus the
// effective merged value and where it currently resolves from.
type editTarget struct {
	file       string // manifest file to edit
	inFilePath string // raw dot-path used as the provenance lookup key (components.<type>.<name>.<rel>)
	yqPath     string // escaped dot-path used to address the YAML node safely
	value      string // effective merged value of the path
	provFile   string // file provenance attributes the value to
	provLine   int    // line within provFile
}

var stackGetCmd = &cobra.Command{
	Use:     "get <path>",
	Short:   "Read a component-relative value from a stack",
	Long:    "Read the effective value of a component-relative dot-path (e.g. vars.region) and show which manifest defines it.",
	Example: "atmos stack get vars.region -s plat-ue2-prod -c vpc",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "stack.getRunE")()

		tgt, err := resolveEditTarget(args[0], false)
		if err != nil {
			return err
		}
		if tgt.provFile != "" {
			ui.Infof("%s resolves from %s:%d", args[0], tgt.provFile, tgt.provLine)
		}
		return data.Writeln(tgt.value)
	},
}

var stackSetCmd = &cobra.Command{
	Use:   "set <path> <value>",
	Short: "Set a component-relative value in the manifest that defines it",
	Long: `Set a component-relative value (e.g. vars.region). Atmos uses provenance to find
the manifest file that defines the effective value and edits that file in place,
preserving comments, anchors, YAML functions, and templates. Use --file to target
a specific manifest.`,
	Example: "atmos stack set vars.region us-west-2 -s plat-ue2-prod -c vpc",
	Args:    cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "stack.setRunE")()

		tgt, err := resolveEditTarget(args[0], true)
		if err != nil {
			return err
		}
		if err := atmosyaml.SetFileWithType(tgt.file, tgt.yqPath, args[1], flagType); err != nil {
			return err
		}
		ui.Successf("Updated %s for %s in %s", args[0], flagComponent, tgt.file)
		return nil
	},
}

var stackDeleteCmd = &cobra.Command{
	Use:     "delete <path>",
	Aliases: []string{"del", "unset"},
	Short:   "Delete a component-relative value from the manifest that defines it",
	Example: "atmos stack delete settings.spacelift.workspace_enabled -s plat-ue2-prod -c vpc",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "stack.deleteRunE")()

		tgt, err := resolveEditTarget(args[0], true)
		if err != nil {
			return err
		}
		if err := atmosyaml.DeleteFile(tgt.file, tgt.yqPath); err != nil {
			return err
		}
		ui.Successf("Deleted %s for %s from %s", args[0], flagComponent, tgt.file)
		return nil
	},
}

func init() {
	for _, c := range []*cobra.Command{stackGetCmd, stackSetCmd, stackDeleteCmd} {
		c.Flags().StringVarP(&flagStack, "stack", "s", "", "Stack name (required)")
		c.Flags().StringVarP(&flagComponent, "component", "c", "", "Component name (required)")
		c.Flags().StringVar(&flagFile, "file", "", "Edit this manifest file explicitly instead of resolving via provenance")
		_ = c.MarkFlagRequired("stack")
		_ = c.MarkFlagRequired("component")
	}
	stackSetCmd.Flags().StringVar(&flagType, "type", atmosyaml.TypeString,
		"Value type: string, int, bool, float, null, or yaml (raw literal)")
}

// resolveEditTarget describes the component in the stack (with provenance) and
// resolves the manifest file plus in-file path for the given component-relative
// dot-path. When requireEditable is true (set/delete), the value must resolve to
// a concrete, writable manifest node; otherwise (get) provenance is best-effort
// and an explicit --file is read directly for its effective value.
func resolveEditTarget(dotPath string, requireEditable bool) (*editTarget, error) {
	atmosConfig, result, err := describeComponentForEdit()
	if err != nil {
		return nil, err
	}

	componentType, _ := result.ComponentSection[cfg.ComponentTypeSectionName].(string)

	tgt := &editTarget{
		// Raw path keys provenance lookups; escaped path addresses YAML nodes.
		inFilePath: pkgstack.BuildComponentInFilePath(componentType, flagComponent, dotPath),
		yqPath:     pkgstack.BuildComponentYqPath(componentType, flagComponent, dotPath),
	}

	// Effective merged value (best-effort; used by get and for messaging).
	if sectionYAML, convErr := u.ConvertToYAML(result.ComponentSection); convErr == nil {
		if v, getErr := atmosyaml.Get([]byte(sectionYAML), dotPath); getErr == nil {
			tgt.value = v
		}
	}

	// Explicit file override bypasses provenance resolution.
	if flagFile != "" {
		tgt.file = flagFile
		// For read-only get, reflect the value actually stored in the explicit
		// file rather than the merged value.
		if !requireEditable {
			if v, getErr := atmosyaml.GetFile(flagFile, tgt.yqPath); getErr == nil {
				tgt.value = v
			}
		}
		return tgt, nil
	}

	return resolveTargetByProvenance(&atmosConfig, result, tgt, dotPath, requireEditable)
}

// describeComponentForEdit initializes a config with stacks processed (the root
// config is loaded without stack processing) and provenance tracking enabled,
// then describes the component so we can find the source file for each value.
func describeComponentForEdit() (schema.AtmosConfiguration, *exec.DescribeComponentResult, error) {
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{
		ComponentFromArg: flagComponent,
		Stack:            flagStack,
	}, true)
	if err != nil {
		return atmosConfig, nil, err
	}
	atmosConfig.TrackProvenance = true

	result, err := exec.ExecuteDescribeComponentWithContext(exec.DescribeComponentContextParams{
		AtmosConfig:          &atmosConfig,
		Component:            flagComponent,
		Stack:                flagStack,
		ProcessTemplates:     false,
		ProcessYamlFunctions: false,
		AuthDisabled:         true,
	})
	return atmosConfig, result, err
}

// resolveTargetByProvenance fills tgt.file from provenance: it finds the manifest
// that defines the effective value, resolves it to an absolute path, and verifies
// the reconstructed in-file path exists there.
func resolveTargetByProvenance(atmosConfig *schema.AtmosConfiguration, result *exec.DescribeComponentResult, tgt *editTarget, dotPath string, requireEditable bool) (*editTarget, error) {
	// Provenance is keyed by the raw in-file path (components.<type>.<name>.<rel>),
	// matching how merge provenance keys are recorded.
	var entries []merge.ProvenanceEntry
	if result.MergeContext != nil {
		entries = result.MergeContext.GetProvenance(tgt.inFilePath)
	}
	provFile, provLine, ok := pkgstack.PickProvenanceFile(entries)
	if !ok {
		// For read-only get there is nothing to edit; return the best-effort
		// merged value without provenance instead of erroring.
		if !requireEditable {
			return tgt, nil
		}
		return nil, errUtils.Build(errUtils.ErrInvalidArgumentError).
			WithExplanationf("%q is not defined for component %q in stack %q.", dotPath, flagComponent, flagStack).
			WithHint("Pass --file <manifest> to choose where the value should be written.").
			Err()
	}
	tgt.provFile = provFile
	tgt.provLine = provLine

	// Provenance records the file relative to the stacks base path; resolve it
	// to an absolute path for reading and writing.
	absFile := provFile
	if !filepath.IsAbs(absFile) {
		absFile = filepath.Join(atmosConfig.StacksBaseAbsolutePath, provFile)
	}

	// Verify the reconstructed in-file path actually exists in the resolved file.
	// When it doesn't (e.g. the value is inherited from a base component under a
	// different key), get still reports the provenance location, but set/delete
	// require --file because there is no concrete node to edit here.
	if _, verifyErr := atmosyaml.GetFile(absFile, tgt.yqPath); verifyErr != nil {
		if !requireEditable {
			return tgt, nil
		}
		return nil, errUtils.Build(errUtils.ErrInvalidArgumentError).
			WithExplanationf("%q resolves from %s:%d, but its key there is not %q (likely inherited or imported).",
				dotPath, provFile, provLine, tgt.inFilePath).
			WithHint("Pass --file with the manifest and edit the explicit path, or set it where the component is declared.").
			Err()
	}
	tgt.file = absFile
	return tgt, nil
}
