package terraform

import (
	"fmt"
	"os"
	"strings"

	"github.com/cloudposse/atmos/pkg/component/configuration"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// encryptionEnvironment is the variable through which OpenTofu also accepts an encryption configuration.
const encryptionEnvironment = "TF_ENCRYPTION"

// writeConfigurationFiles writes the provider's generated files into the provisioned module and
// warns about state encryption that a kept file dropped. A dry run writes nothing.
func writeConfigurationFiles(info *schema.ConfigAndStacksInfo, componentPath string, prepared *schema.PreparedComponentConfiguration) error {
	if prepared == nil || info.DryRun {
		return nil
	}
	report, err := configuration.WriteFilesReport(componentPath, prepared.Files)
	if err != nil {
		return fmt.Errorf("component %s in stack %s, working directory %s: %w", info.ComponentFromArg, info.Stack, componentPath, err)
	}
	warnRetainedUnencrypted(info, componentPath, report)
	return nil
}

// warnRetainedUnencrypted warns when a generated file that configured state encryption was not
// written, because if_exists = "skip" kept an existing file, and nothing else still configures
// encryption. Without the warning the state would be written unencrypted with no sign that the
// configured encryption was dropped.
func warnRetainedUnencrypted(info *schema.ConfigAndStacksInfo, componentPath string, report configuration.WriteReport) {
	if len(report.Retained) == 0 || encryptionFromEnvironment(info) {
		return
	}
	for _, path := range configuration.UnencryptedRetainedFiles(componentPath, report) {
		ui.Warningf("Component `%s` in `%s` configures state encryption in the generated file `%s`, but an existing `%s` was kept because of `if_exists = \"skip\"`, and no file of the module configures encryption. The state will not be encrypted. Remove the existing file or change `if_exists` so the generated file is written.", info.ComponentFromArg, info.Stack, path, path)
	}
}

// encryptionFromEnvironment reports whether the component's environment or the process sets
// TF_ENCRYPTION, which OpenTofu merges into the module's encryption configuration. Files are written
// before the subprocess environment is assembled, so the stack `env` section is consulted directly.
func encryptionFromEnvironment(info *schema.ConfigAndStacksInfo) bool {
	if value, found := lastEnvListValue(info.ComponentEnvList, encryptionEnvironment); found {
		return strings.TrimSpace(value) != ""
	}
	if value, found := info.ComponentEnvSection[encryptionEnvironment]; found {
		return strings.TrimSpace(fmt.Sprint(value)) != ""
	}
	value, _ := lastEnvListValue(os.Environ(), encryptionEnvironment)
	return strings.TrimSpace(value) != ""
}
