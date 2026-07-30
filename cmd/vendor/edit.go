package vendor

import (
	"fmt"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/vendoring"
	atmosyaml "github.com/cloudposse/atmos/pkg/yaml"
)

// DefaultVendorManifest is the default vendor manifest filename.
const DefaultVendorManifest = vendoring.DefaultVendorFile

// vendorFileFlag overrides which vendor manifest to read/edit.
var vendorFileFlag string

// vendorGetCmd reads the pinned version of a component from the vendor
// manifest. It is a thin alias of "vendor config get": it resolves the
// component's dot-notation path (spec.sources[<i>].version) and delegates to
// the same path-based engine.
var vendorGetCmd = &cobra.Command{
	Use:     "get <component>",
	Short:   "Read the pinned version of a vendored component",
	Long:    "Read the version pinned for a component in the vendor manifest (vendor.yaml).",
	Example: "atmos vendor get vpc",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(nil, "vendor.getRunE")()

		file, err := resolveVendorFile()
		if err != nil {
			return err
		}
		path, err := vendoring.ComponentVersionPath(file, args[0])
		if err != nil {
			return err
		}
		return runVendorConfigGet(file, path)
	},
}

// vendorSetCmd updates the pinned version of a component in the vendor
// manifest. It is a thin alias of "vendor config set": it resolves the
// component's dot-notation path (spec.sources[<i>].version) and delegates to
// the same path-based engine.
var vendorSetCmd = &cobra.Command{
	Use:   "set <component> <version>",
	Short: "Set the pinned version of a vendored component",
	Long: `Set the version pinned for a component in the vendor manifest (vendor.yaml),
preserving comments, anchors, and Go templates such as {{.Version}} in source URLs.
The source is matched by component name, so manifest ordering does not matter.`,
	Example: "atmos vendor set vpc v1.5.0",
	Args:    cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(nil, "vendor.setRunE")()

		file, err := resolveVendorFile()
		if err != nil {
			return err
		}
		path, err := vendoring.ComponentVersionPath(file, args[0])
		if err != nil {
			return err
		}
		return runVendorConfigSet(file, path, args[1], atmosyaml.TypeString)
	},
}

func init() {
	for _, c := range []*cobra.Command{vendorGetCmd, vendorSetCmd} {
		c.Flags().StringVar(&vendorFileFlag, "file", "", "Vendor manifest file (default: ./vendor.yaml)")
	}
	vendorCmd.AddCommand(vendorGetCmd)
	vendorCmd.AddCommand(vendorSetCmd)
}

// resolveVendorFile picks the vendor manifest to operate on: the --file override,
// otherwise ./vendor.yaml in the current directory.
func resolveVendorFile() (string, error) {
	return resolveVendorFileWithOverride(vendorFileFlag)
}

func resolveVendorFileWithOverride(file string) (string, error) {
	if resolved, ok := vendoring.VendorFilePresent(file); ok {
		return resolved, nil
	}
	return "", errUtils.Build(errUtils.ErrInvalidArgumentError).
		WithExplanation(fmt.Sprintf("No %s found in the current directory.", DefaultVendorManifest)).
		WithHint("Run from a directory containing vendor.yaml, or pass --file to point at one.").
		Err()
}
