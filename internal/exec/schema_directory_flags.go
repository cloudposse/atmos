package exec

import (
	"strings"

	"github.com/spf13/cobra"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// applyParsedSchemaDirectoryFlags retains CLI overrides after Cobra strips flags
// from args. Raw-argument commands already populate these fields directly.
func applyParsedSchemaDirectoryFlags(cmd *cobra.Command, info *schema.ConfigAndStacksInfo) {
	for name, destination := range map[string]*string{
		cfg.JsonSchemaDirFlag: &info.JsonSchemaDir,
		cfg.OpaDirFlag:        &info.OpaDir,
	} {
		if flag := cmd.Flag(strings.TrimPrefix(name, "--")); flag != nil && flag.Changed {
			*destination = flag.Value.String()
		}
	}
}
