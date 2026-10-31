package artifacts

import (
	"github.com/cloudposse/atmos/pkg/flags"
)

// Flag names. Every flag is read through the standard flag parser, so command-line values win
// over environment variables, which win over defaults.
const (
	flagRepository = "repository"
	flagName       = "name"
	flagVersion    = "version"
	flagKind       = "kind"
	flagForce      = "force"
	flagDryRun     = "dry-run"
	flagLabels     = "labels"
	flagReadme     = "readme"
	flagChangelog  = "changelog"
	flagOutput     = "output"
	flagFrom       = "from"
	flagTo         = "to"
	flagRefresh    = "refresh"
	flagFormat     = "format"
	flagYes        = "yes"
)

// Shorthands. The global flags own -C (chdir) and -i (identity).
const (
	shortRepository = "r"
	shortForce      = "f"
	shortDryRun     = "n"
	shortYes        = "y"
)

// Environment variables, all under the ATMOS_ARTIFACTS_ prefix.
const (
	envRepository = "ATMOS_ARTIFACTS_REPOSITORY"
	envName       = "ATMOS_ARTIFACTS_NAME"
	envVersion    = "ATMOS_ARTIFACTS_VERSION"
	envKind       = "ATMOS_ARTIFACTS_KIND"
	envForce      = "ATMOS_ARTIFACTS_FORCE"
	envDryRun     = "ATMOS_ARTIFACTS_DRY_RUN"
	envLabels     = "ATMOS_ARTIFACTS_LABELS"
	envReadme     = "ATMOS_ARTIFACTS_README"
	envChangelog  = "ATMOS_ARTIFACTS_CHANGELOG"
	envOutput     = "ATMOS_ARTIFACTS_OUTPUT"
	envFrom       = "ATMOS_ARTIFACTS_FROM"
	envTo         = "ATMOS_ARTIFACTS_TO"
	envRefresh    = "ATMOS_ARTIFACTS_REFRESH"
	envFormat     = "ATMOS_ARTIFACTS_FORMAT"
	envYes        = "ATMOS_ARTIFACTS_YES"
)

// Per-command Viper key prefixes. The same flag name carries the same environment variable on every
// subcommand, but each subcommand reads its own key, so one command's value never bleeds into another.
const (
	publishPrefix = "artifacts.publish"
	pullPrefix    = "artifacts.pull"
	copyPrefix    = "artifacts.copy"
	inspectPrefix = "artifacts.inspect"
	listPrefix    = "artifacts.list"
	searchPrefix  = "artifacts.search"
	deletePrefix  = "artifacts.delete"
	signPrefix    = "artifacts.sign"
	verifyPrefix  = "artifacts.verify"
	showPrefix    = "artifacts.show"
)

// Output formats. Operations print a result; list and search print a collection.
const (
	formatText  = "text"
	formatTable = "table"
	formatJSON  = "json"
	formatYAML  = "yaml"
	formatCSV   = "csv"
	formatTSV   = "tsv"
)

// resultFormats are the formats of a single-result command.
var resultFormats = []string{formatText, formatJSON, formatYAML}

// listFormats are the formats of a collection command.
var listFormats = []string{formatTable, formatJSON, formatYAML, formatCSV, formatTSV}

// repositoryFlag adds the required single repository selector.
func repositoryFlag() []flags.Option {
	return []flags.Option{
		flags.WithStringFlag(flagRepository, shortRepository, "", "Configured artifact repository alias (artifacts.repositories)"),
		flags.WithEnvVars(flagRepository, envRepository),
	}
}

// formatFlag adds the output format flag with the given default.
func formatFlag(defaultFormat string, valid []string) []flags.Option {
	return []flags.Option{
		flags.WithStringFlag(flagFormat, "", defaultFormat, "Output format: "+joinValues(valid)),
		flags.WithEnvVars(flagFormat, envFormat),
	}
}

// build assembles a parser for a command from option groups.
func newParser(prefix string, groups ...[]flags.Option) *flags.StandardParser {
	options := []flags.Option{flags.WithViperPrefix(prefix)}
	for _, group := range groups {
		options = append(options, group...)
	}
	return flags.NewStandardParser(options...)
}

func newPublishParser() *flags.StandardParser {
	return newPublishParserFor(publishPrefix, true)
}

// newPublishParserFor builds the publish flag set under a Viper prefix. A convenience command that
// supplies its own artifact kind passes withKind=false, so it has no --kind flag to contradict.
func newPublishParserFor(prefix string, withKind bool) *flags.StandardParser {
	options := []flags.Option{
		flags.WithStringFlag(flagName, "", "", "Artifact name (default: producer metadata, then the source basename)"),
		flags.WithStringFlag(flagVersion, "", "", "Artifact version (required unless the producer supplies one)"),
		flags.WithBoolFlag(flagForce, shortForce, false, "Replace an existing artifact with different content"),
		flags.WithBoolFlag(flagDryRun, shortDryRun, false, "Validate and plan the publication without writing anything"),
		flags.WithStringArrayFlag(flagLabels, "", nil, "Exact key=value labels (comma-separated and/or repeated)"),
		flags.WithStringFlag(flagReadme, "", "", "README path, relative to the source directory"),
		flags.WithStringFlag(flagChangelog, "", "", "CHANGELOG path, relative to the source directory"),
		flags.WithEnvVars(flagName, envName),
		flags.WithEnvVars(flagVersion, envVersion),
		flags.WithEnvVars(flagForce, envForce),
		flags.WithEnvVars(flagDryRun, envDryRun),
		flags.WithEnvVars(flagLabels, envLabels),
		flags.WithViperKey(flagLabels, prefix+"."+flagLabels),
		flags.WithEnvVars(flagReadme, envReadme),
		flags.WithEnvVars(flagChangelog, envChangelog),
	}
	if withKind {
		options = append(
			options,
			flags.WithStringFlag(flagKind, "", "", "Artifact kind (default: files)"),
			flags.WithEnvVars(flagKind, envKind),
		)
	}
	return newParser(prefix, repositoryFlag(), options, formatFlag(formatText, resultFormats))
}

func newPullParser() *flags.StandardParser {
	return newParser(pullPrefix, repositoryFlag(), []flags.Option{
		flags.WithStringFlag(flagOutput, "", "", "Directory to materialize the artifact into"),
		flags.WithBoolFlag(flagForce, shortForce, false, "Replace an existing, different output directory"),
		flags.WithEnvVars(flagOutput, envOutput),
		flags.WithEnvVars(flagForce, envForce),
	}, formatFlag(formatText, resultFormats))
}

func newCopyParser() *flags.StandardParser {
	return newParser(copyPrefix, []flags.Option{
		flags.WithStringFlag(flagFrom, "", "", "Source repository alias"),
		flags.WithStringFlag(flagTo, "", "", "Destination repository alias"),
		flags.WithBoolFlag(flagForce, shortForce, false, "Replace a conflicting destination; never bypasses verification"),
		flags.WithEnvVars(flagFrom, envFrom),
		flags.WithEnvVars(flagTo, envTo),
		flags.WithEnvVars(flagForce, envForce),
	}, formatFlag(formatText, resultFormats))
}

func newRepositoryParser(prefix string) *flags.StandardParser {
	return newParser(prefix, repositoryFlag(), formatFlag(formatText, resultFormats))
}

func newSignParser() *flags.StandardParser {
	return newParser(signPrefix, repositoryFlag(), []flags.Option{
		flags.WithBoolFlag(flagYes, shortYes, false, "Accept Sigstore transparency log publication without prompting"),
		flags.WithEnvVars(flagYes, envYes),
	}, formatFlag(formatText, resultFormats))
}

func newListParser(prefix string) *flags.StandardParser {
	return newParser(prefix, []flags.Option{
		flags.WithStringSliceFlag(flagRepository, shortRepository, nil, "Restrict to these configured repository aliases (default: all configured repositories)"),
		flags.WithEnvVars(flagRepository, envRepository),
		flags.WithStringArrayFlag(flagLabels, "", nil, "Keep entries whose labels match every key=value selector (comma-separated and/or repeated)"),
		flags.WithBoolFlag(flagRefresh, "", false, "Bypass cached discovery snapshots"),
		flags.WithEnvVars(flagLabels, envLabels),
		flags.WithViperKey(flagLabels, prefix+"."+flagLabels),
		flags.WithEnvVars(flagRefresh, envRefresh),
	}, formatFlag(formatTable, listFormats))
}
