package commandline

import (
	"fmt"
	"strconv"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	terminalenv "github.com/cloudposse/atmos/pkg/terminal/env"
	u "github.com/cloudposse/atmos/pkg/utils"
)

const flagValueSeparator = "="

// hasBoolFlagLiteral reports whether the argument after args[i] is an explicit
// "true" or "false" value for the boolean flag at args[i].
func hasBoolFlagLiteral(args []string, i int) bool {
	return i+1 < len(args) && terminalenv.IsBoolLiteral(args[i+1])
}

// boolFlagArgValue returns the value of the bare boolean flag at args[i]:
// false when it is followed by the literal "false", true otherwise.
func boolFlagArgValue(args []string, i int) bool {
	return !hasBoolFlagLiteral(args, i) || !strings.EqualFold(args[i+1], "false")
}

// parseFlagValue extracts the value for a CLI flag from the current argument.
// It handles both space-separated ("--flag value") and equals-separated ("--flag=value") forms.
// Returns ("", false, nil) when arg does not match flag.
// Returns (value, true, nil) on success.
// Returns ("", false, err) when the flag matches but its value is missing or malformed.
//
// SplitN(arg, "=", 2) is used so that values containing "=" (e.g., --query=.tags[?env==prod])
// are handled correctly — only the first "=" is treated as the separator.
func parseFlagValue(flag, arg string, args []string, index int) (string, bool, error) {
	if arg == flag {
		if index+1 >= len(args) {
			return "", false, fmt.Errorf(errFlagFormat, errUtils.ErrInvalidFlag, arg)
		}
		return args[index+1], true, nil
	}
	if strings.HasPrefix(arg, flag+flagValueSeparator) {
		// SplitN(..., 2) keeps any additional "=" in the value intact.
		parts := strings.SplitN(arg, flagValueSeparator, 2)
		return parts[1], true, nil
	}
	return "", false, nil
}

// parseIdentityFlag handles --identity / -i which support optional and empty values.
//
//   - --identity / -i         → __SELECT__ (interactive selection).
//   - --identity value / -i value   → use value.
//   - --identity=value / -i=value   → use value.
//   - --identity= / -i=            → __SELECT__ (interactive selection).
//
// Boolean-false representations of the value ("false", "0", "no", "off",
// case-insensitive) are normalized to cfg.IdentityFlagDisabledValue so that
// --identity=false disables authentication, matching the documented behavior
// of ATMOS_IDENTITY=false. Normalization is identity-on for all other values.
//
// SplitN(arg, "=", 2) is used so that identity values containing "=" (e.g., ARN-like
// strings with key=value parameters) are handled correctly.
func parseIdentityFlag(info *schema.ArgsAndFlagsInfo, arg string, args []string, index int) {
	if arg == cfg.IdentityFlag || arg == cfg.IdentityFlagShort {
		// Has value: --identity <value> (next arg exists and is not another flag).
		if len(args) > index+1 && !strings.HasPrefix(args[index+1], "-") {
			info.Identity = cfg.NormalizeIdentityValue(args[index+1])
		} else {
			// No value: --identity (interactive selection).
			info.Identity = cfg.IdentityFlagSelectValue
		}
		return
	}

	for _, prefix := range []string{cfg.IdentityFlag + flagValueSeparator, "-i="} {
		if !strings.HasPrefix(arg, prefix) {
			continue
		}

		// SplitN(..., 2) keeps any additional "=" in the value intact.
		parts := strings.SplitN(arg, flagValueSeparator, 2)
		if parts[1] == "" {
			// Empty value: --identity= (interactive selection).
			info.Identity = cfg.IdentityFlagSelectValue
		} else {
			info.Identity = cfg.NormalizeIdentityValue(parts[1])
		}
		return
	}
}

// parseFromPlanFlag handles --from-plan which has optional value semantics.
//
//   - --from-plan           → UseTerraformPlan = true, PlanFile unchanged.
//   - --from-plan <path>    → UseTerraformPlan = true, PlanFile = path.
//   - --from-plan=<path>    → UseTerraformPlan = true, PlanFile = path.
//   - --from-plan=          → UseTerraformPlan = true, PlanFile unchanged.
func parseFromPlanFlag(info *schema.ArgsAndFlagsInfo, arg string, args []string, index int) {
	if arg == cfg.FromPlanFlag {
		info.UseTerraformPlan = true
		// Check if next argument is the planfile path (not another flag).
		if len(args) > index+1 && !strings.HasPrefix(args[index+1], "-") {
			info.PlanFile = args[index+1]
		}
		return
	}
	if strings.HasPrefix(arg, cfg.FromPlanFlag+flagValueSeparator) {
		info.UseTerraformPlan = true
		planFilePath := strings.TrimPrefix(arg, cfg.FromPlanFlag+flagValueSeparator)
		if planFilePath != "" {
			info.PlanFile = planFilePath
		}
	}
}

// ParseArguments separates Atmos flags from the component command and native arguments.
func ParseArguments(componentType string, inputArgsAndFlags []string) (schema.ArgsAndFlagsInfo, error) {
	defer perf.Track(nil, "commandline.ParseArguments")()

	var info schema.ArgsAndFlagsInfo
	// For commands with only the subcommand (e.g., `atmos terraform plan`),
	// just set the subcommand and return - don't auto-show help.
	// Interactive prompts will handle missing args if available,
	// otherwise validation will show appropriate errors.
	// Exception: a quoted compound subcommand still needs further processing.
	if len(inputArgsAndFlags) == 1 && !strings.Contains(inputArgsAndFlags[0], " ") {
		info.SubCommand = inputArgsAndFlags[0]
		return info, nil
	}
	parsed, err := collectAtmosFlags(&info, inputArgsAndFlags)
	if err != nil {
		return info, err
	}
	info.GlobalOptions = parsed.globalOptions
	return finishCommandArguments(componentType, &info, parsed.additional)
}

// parsedArguments keeps collection results named rather than returning positional slices.
type parsedArguments struct {
	additional    []string
	globalOptions []string
}

func collectAtmosFlags(info *schema.ArgsAndFlagsInfo, args []string) (parsedArguments, error) {
	var result parsedArguments
	var indexesToRemove []int
	// https://github.com/roboll/helmfile#cli-reference
	var globalOptionsFlagIndex int
	for i, arg := range args {
		// Track the global-options value's position for second-pass collection.
		// Match the full flag or flag+"=" so flags sharing a prefix cannot match.
		switch {
		case arg == cfg.GlobalOptionsFlag:
			globalOptionsFlagIndex = i + 1
		case strings.HasPrefix(arg, cfg.GlobalOptionsFlag+flagValueSeparator):
			globalOptionsFlagIndex = i
		}
		if err := parseStringFlags(info, arg, args, i); err != nil {
			return result, err
		}
		parseBooleanFlags(info, args, i)
		// Identity and from-plan have special optional/empty-value semantics.
		parseIdentityFlag(info, arg, args, i)
		parseFromPlanFlag(info, arg, args, i)
		indexesToRemove = append(indexesToRemove, atmosFlagIndexes(args, i)...)
	}
	for i, arg := range args {
		if !u.SliceContainsInt(indexesToRemove, i) {
			result.additional = append(result.additional, arg)
		}
		if globalOptionsFlagIndex > 0 && i == globalOptionsFlagIndex {
			value := strings.TrimPrefix(arg, cfg.GlobalOptionsFlag+flagValueSeparator)
			result.globalOptions = strings.Split(value, " ")
		}
	}
	return result, nil
}

// parseStringFlags applies the table of ordinary string-valued flags.
func parseStringFlags(info *schema.ArgsAndFlagsInfo, arg string, args []string, index int) error {
	for _, def := range stringFlagDefs {
		value, found, err := parseFlagValue(def.flag, arg, args, index)
		if err != nil {
			return err
		}
		if found {
			def.setFunc(info, value)
			return nil
		}
	}
	return nil
}

func parseBooleanFlags(info *schema.ArgsAndFlagsInfo, args []string, index int) {
	arg := args[index]
	// --logs-color consumes the next positional argument only for an explicit boolean literal.
	if arg == cfg.LogsColorFlag {
		info.LogsColor = strconv.FormatBool(boolFlagArgValue(args, index))
	} else if value, found := strings.CutPrefix(arg, cfg.LogsColorFlag+flagValueSeparator); found {
		info.LogsColor = value
	}
	// ProcessTemplatesFlag and ProcessFunctionsFlag are consumed exclusively by
	// Cobra in cmd/terraform and have no ArgsAndFlagsInfo field. They remain in
	// commonFlags so they are stripped before invoking the underlying tool.
	switch arg {
	case cfg.DryRunFlag:
		info.DryRun = boolFlagArgValue(args, index)
	case cfg.SkipInitFlag:
		info.SkipInit = boolFlagArgValue(args, index)
	case cfg.HelpFlag1, cfg.HelpFlag2:
		info.NeedHelp = true
	case cfg.AffectedFlag:
		info.Affected = boolFlagArgValue(args, index)
	case cfg.AllFlag:
		info.All = boolFlagArgValue(args, index)
	}
}

func atmosFlagIndexes(args []string, index int) []int {
	var result []int
	for _, flag := range commonFlags {
		switch {
		case args[index] == flag:
			result = append(result, index)
			if commonFlagConsumesNext(flag, args, index) {
				result = append(result, index+1)
			}
		case strings.HasPrefix(args[index], flag+flagValueSeparator):
			result = append(result, index)
		}
	}
	return result
}

func commonFlagConsumesNext(flag string, args []string, index int) bool {
	if index+1 >= len(args) {
		return false
	}
	switch {
	case flag == cfg.FromPlanFlag || flag == cfg.IdentityFlag || flag == cfg.IdentityFlagShort:
		// Optional values consume the next argument only when it is not another
		// flag. Preserve native flags such as "-i -lock=false" without an identity.
		return !strings.HasPrefix(args[index+1], "-")
	case boolOnlyCommonFlags[flag]:
		// Boolean flags only consume an explicit true/false literal; other values
		// belong to the command or underlying tool.
		return hasBoolFlagLiteral(args, index)
	default:
		// Ordinary value-taking flags consumed the following value during parsing.
		return valueTakingCommonFlags[flag]
	}
}

func finishCommandArguments(componentType string, info *schema.ArgsAndFlagsInfo, args []string) (schema.ArgsAndFlagsInfo, error) {
	if info.NeedHelp {
		if len(args) > 0 {
			info.SubCommand = args[0]
		}
		return *info, nil
	}
	if len(args) == 1 && info.SubCommand == "" {
		info.SubCommand = args[0]
	}
	if len(args) == 0 {
		return *info, nil
	}
	// Terraform compound commands have their own subcommands (providers lock,
	// state list, workspace select). https://developer.hashicorp.com/terraform/cli/commands
	if componentType == "terraform" {
		processed, err := processTerraformCompoundSubcommand(info, args)
		if err != nil || processed {
			return *info, err
		}
	}
	// Not a compound subcommand: process a single command.
	err := processSingleCommand(info, args)
	return *info, err
}
