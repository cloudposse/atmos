package terraform

import (
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
)

// RefreshCompatibilityAliases returns compatibility aliases for terraform refresh.
func RefreshCompatibilityAliases() map[string]flags.CompatibilityAlias {
	defer perf.Track(nil, "terraform.RefreshCompatibilityAliases")()

	return mergeMaps(commonCompatibilityFlags(), map[string]flags.CompatibilityAlias{
		"-compact-warnings": {Behavior: flags.AppendToSeparated, Target: ""},
		"-input":            {Behavior: flags.AppendToSeparated, Target: ""},
		"-json":             {Behavior: flags.AppendToSeparated, Target: ""},
		"-parallelism":      {Behavior: flags.AppendToSeparated, Target: ""},
		"-replace":          {Behavior: flags.AppendToSeparated, Target: ""},
		"-target":           {Behavior: flags.AppendToSeparated, Target: ""},
	})
}

// ImportCompatibilityAliases returns compatibility aliases for terraform import.
func ImportCompatibilityAliases() map[string]flags.CompatibilityAlias {
	defer perf.Track(nil, "terraform.ImportCompatibilityAliases")()

	return mergeMaps(commonCompatibilityFlags(), map[string]flags.CompatibilityAlias{
		"-config":      {Behavior: flags.AppendToSeparated, Target: ""},
		"-input":       {Behavior: flags.AppendToSeparated, Target: ""},
		"-parallelism": {Behavior: flags.AppendToSeparated, Target: ""},
		"-provider":    {Behavior: flags.AppendToSeparated, Target: ""},
		"-state":       {Behavior: flags.AppendToSeparated, Target: ""},
		"-state-out":   {Behavior: flags.AppendToSeparated, Target: ""},
		"-backup":      {Behavior: flags.AppendToSeparated, Target: ""},
	})
}

// ShowCompatibilityAliases returns compatibility aliases for terraform show.
func ShowCompatibilityAliases() map[string]flags.CompatibilityAlias {
	defer perf.Track(nil, "terraform.ShowCompatibilityAliases")()

	return map[string]flags.CompatibilityAlias{
		"-json":     {Behavior: flags.AppendToSeparated, Target: ""},
		"-no-color": {Behavior: flags.AppendToSeparated, Target: ""},
	}
}

// StateCompatibilityAliases returns compatibility aliases for terraform state.
func StateCompatibilityAliases() map[string]flags.CompatibilityAlias {
	defer perf.Track(nil, "terraform.StateCompatibilityAliases")()

	return map[string]flags.CompatibilityAlias{
		"-lock":         {Behavior: flags.AppendToSeparated, Target: ""},
		"-lock-timeout": {Behavior: flags.AppendToSeparated, Target: ""},
		"-state":        {Behavior: flags.AppendToSeparated, Target: ""},
		"-no-color":     {Behavior: flags.AppendToSeparated, Target: ""},
	}
}

// FmtCompatibilityAliases returns compatibility aliases for terraform fmt.
func FmtCompatibilityAliases() map[string]flags.CompatibilityAlias {
	defer perf.Track(nil, "terraform.FmtCompatibilityAliases")()

	return map[string]flags.CompatibilityAlias{
		"-list":      {Behavior: flags.AppendToSeparated, Target: ""},
		"-write":     {Behavior: flags.AppendToSeparated, Target: ""},
		"-diff":      {Behavior: flags.AppendToSeparated, Target: ""},
		"-check":     {Behavior: flags.AppendToSeparated, Target: ""},
		"-recursive": {Behavior: flags.AppendToSeparated, Target: ""},
		"-no-color":  {Behavior: flags.AppendToSeparated, Target: ""},
	}
}

// GraphCompatibilityAliases returns compatibility aliases for terraform graph.
func GraphCompatibilityAliases() map[string]flags.CompatibilityAlias {
	defer perf.Track(nil, "terraform.GraphCompatibilityAliases")()

	return map[string]flags.CompatibilityAlias{
		"-type":         {Behavior: flags.AppendToSeparated, Target: ""},
		"-draw-cycles":  {Behavior: flags.AppendToSeparated, Target: ""},
		"-module-depth": {Behavior: flags.AppendToSeparated, Target: ""},
		"-no-color":     {Behavior: flags.AppendToSeparated, Target: ""},
	}
}

// TaintCompatibilityAliases returns compatibility aliases for terraform taint.
func TaintCompatibilityAliases() map[string]flags.CompatibilityAlias {
	defer perf.Track(nil, "terraform.TaintCompatibilityAliases")()

	return map[string]flags.CompatibilityAlias{
		"-lock":         {Behavior: flags.AppendToSeparated, Target: ""},
		"-lock-timeout": {Behavior: flags.AppendToSeparated, Target: ""},
		"-state":        {Behavior: flags.AppendToSeparated, Target: ""},
		"-state-out":    {Behavior: flags.AppendToSeparated, Target: ""},
		"-no-color":     {Behavior: flags.AppendToSeparated, Target: ""},
	}
}

// UntaintCompatibilityAliases returns compatibility aliases for terraform untaint.
func UntaintCompatibilityAliases() map[string]flags.CompatibilityAlias {
	defer perf.Track(nil, "terraform.UntaintCompatibilityAliases")()

	return TaintCompatibilityAliases() // Same flags as taint
}

// ForceUnlockCompatibilityAliases returns compatibility aliases for terraform force-unlock.
func ForceUnlockCompatibilityAliases() map[string]flags.CompatibilityAlias {
	defer perf.Track(nil, "terraform.ForceUnlockCompatibilityAliases")()

	return map[string]flags.CompatibilityAlias{
		"-force":    {Behavior: flags.AppendToSeparated, Target: ""},
		"-no-color": {Behavior: flags.AppendToSeparated, Target: ""},
	}
}

// ConsoleCompatibilityAliases returns compatibility aliases for terraform console.
func ConsoleCompatibilityAliases() map[string]flags.CompatibilityAlias {
	defer perf.Track(nil, "terraform.ConsoleCompatibilityAliases")()

	return map[string]flags.CompatibilityAlias{
		"-state":    {Behavior: flags.AppendToSeparated, Target: ""},
		"-var":      {Behavior: flags.AppendToSeparated, Target: ""},
		"-var-file": {Behavior: flags.AppendToSeparated, Target: ""},
		"-no-color": {Behavior: flags.AppendToSeparated, Target: ""},
	}
}

// ProvidersCompatibilityAliases returns compatibility aliases for terraform providers.
func ProvidersCompatibilityAliases() map[string]flags.CompatibilityAlias {
	defer perf.Track(nil, "terraform.ProvidersCompatibilityAliases")()

	return map[string]flags.CompatibilityAlias{
		"-json":     {Behavior: flags.AppendToSeparated, Target: ""},
		"-no-color": {Behavior: flags.AppendToSeparated, Target: ""},
	}
}

// GetCompatibilityAliases returns compatibility aliases for terraform get.
func GetCompatibilityAliases() map[string]flags.CompatibilityAlias {
	defer perf.Track(nil, "terraform.GetCompatibilityAliases")()

	return map[string]flags.CompatibilityAlias{
		"-update":   {Behavior: flags.AppendToSeparated, Target: ""},
		"-no-color": {Behavior: flags.AppendToSeparated, Target: ""},
	}
}

// TestCompatibilityAliases returns compatibility aliases for terraform test.
func TestCompatibilityAliases() map[string]flags.CompatibilityAlias {
	defer perf.Track(nil, "terraform.TestCompatibilityAliases")()

	return map[string]flags.CompatibilityAlias{
		"-filter":   {Behavior: flags.AppendToSeparated, Target: ""},
		"-json":     {Behavior: flags.AppendToSeparated, Target: ""},
		"-no-color": {Behavior: flags.AppendToSeparated, Target: ""},
		"-var":      {Behavior: flags.AppendToSeparated, Target: ""},
		"-var-file": {Behavior: flags.AppendToSeparated, Target: ""},
		"-verbose":  {Behavior: flags.AppendToSeparated, Target: ""},
	}
}
