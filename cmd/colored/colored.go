// ColoredCobra allows you to colorize Cobra's text output,
// making it look better using simple settings to customize
// individual parts of console output.
//
// Usage example:
//
// 1. Insert in cmd/root.go file of your project :
//
//	import cc "github.com/ivanpirog/coloredcobra"
//
// 2. Put the following code to the beginning of the Execute() function:
//
//	cc.Init(&cc.Config{
//	    RootCmd:    rootCmd,
//	    Headings:   cc.Bold + cc.Underline,
//	    Commands:   cc.Yellow + cc.Bold,
//	    ExecName:   cc.Bold,
//	    Flags:      cc.Bold,
//	})
//
// 3. Build & execute your code.
//
// Copyright © 2022 Ivan Pirog <ivan.pirog@gmail.com>.
// Released under the MIT license.
// Project home: https://github.com/ivanpirog/coloredcobra
package colored

import (
	"regexp"
	"strings"

	"github.com/cloudposse/atmos/pkg/ui/theme"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

// Config is a settings structure which sets styles for individual parts of Cobra text output.
//
// Note that RootCmd is required.
//
// Example:
//
//	c := &cc.Config{
//	   RootCmd:       rootCmd,
//	}
type Config struct {
	RootCmd         *cobra.Command
	NoExtraNewlines bool
	NoBottomNewline bool
}

// Init patches Cobra's usage template with configuration provided.
func Init(cfg *Config) {
	if cfg.RootCmd == nil {
		panic("coloredcobra: Root command pointer is missing.")
	}

	// Get usage template
	tpl := cfg.RootCmd.UsageTemplate()

	//
	// Add extra line breaks for headings
	//
	if !cfg.NoExtraNewlines {
		tpl = strings.NewReplacer(
			"Use \"", "\nUse \"",
		).Replace(tpl)
	}

	//
	// Styling headers
	//
	if theme.Styles.Help.Headings != nil {
		ch := theme.Styles.Help.Headings
		// Add template function to style the headers
		cobra.AddTemplateFunc("HeadingStyle", ch.SprintFunc())
	}

	//
	// Styling commands
	//
	if theme.Styles.Help.Commands != nil {
		cc := theme.Styles.Help.Commands

		// Add template function to style commands
		cobra.AddTemplateFunc("CommandStyle", cc.SprintFunc())
		cobra.AddTemplateFunc("sum", func(a, b int) int {
			return a + b
		})

		// Patch usage template
		re := regexp.MustCompile(`(?i){{\s*rpad\s+.Name\s+.NamePadding\s*}}`)
		tpl = re.ReplaceAllLiteralString(tpl, "{{rpad (CommandStyle .Name) (sum .NamePadding 12)}}")

		re = regexp.MustCompile(`(?i){{\s*rpad\s+.CommandPath\s+.CommandPathPadding\s*}}`)
		tpl = re.ReplaceAllLiteralString(tpl, "{{rpad (CommandStyle .CommandPath) (sum .CommandPathPadding 12)}}")
	}

	//
	// Styling a short desription of commands
	//
	if theme.Styles.Help.CmdShortDescr != nil {
		csd := theme.Styles.Help.CmdShortDescr

		cobra.AddTemplateFunc("CmdShortStyle", csd.SprintFunc())

		re := regexp.MustCompile(`(?ism)({{\s*range\s+.Commands\s*}}.*?){{\s*.Short\s*}}`)
		tpl = re.ReplaceAllString(tpl, `$1{{CmdShortStyle .Short}}`)
	}

	//
	// Styling executable file name
	//
	if theme.Styles.Help.ExecName != nil {
		cen := theme.Styles.Help.ExecName

		// Add template functions
		cobra.AddTemplateFunc("ExecStyle", cen.SprintFunc())
		cobra.AddTemplateFunc("UseLineStyle", func(s string) string {
			spl := strings.Split(s, " ")
			spl[0] = cen.Sprint(spl[0])
			return strings.Join(spl, " ")
		})

		// Patch usage template
		re := regexp.MustCompile(`(?i){{\s*.CommandPath\s*}}`)
		tpl = re.ReplaceAllLiteralString(tpl, "{{ExecStyle .CommandPath}}")

		re = regexp.MustCompile(`(?i){{\s*.UseLine\s*}}`)
		tpl = re.ReplaceAllLiteralString(tpl, "{{UseLineStyle .UseLine}}")
	}

	//
	// Styling flags
	//
	var cf, cfd, cfdt *color.Color
	if theme.Styles.Help.Flags != nil {
		cf = theme.Styles.Help.Flags
	}
	if theme.Styles.Help.FlagsDescr != nil {
		cfd = theme.Styles.Help.FlagsDescr
	}
	if theme.Styles.Help.FlagsDataType != nil {
		cfdt = theme.Styles.Help.FlagsDataType
	}
	if cf != nil || cfd != nil || cfdt != nil {

		cobra.AddTemplateFunc("FlagStyle", func(s string) string {
			// Flags info section is multi-line.
			// Let's split these lines and iterate them.
			lines := strings.Split(s, "\n")
			for k := range lines {

				// Styling short and full flags (-f, --flag)
				if cf != nil {
					re := regexp.MustCompile(`(--?\S+)`)
					for _, flag := range re.FindAllString(lines[k], 2) {
						lines[k] = strings.Replace(lines[k], flag, cf.Sprint(flag), 1)
					}
				}

				// If no styles for flag data types and description - continue
				if cfd == nil && cfdt == nil {
					continue
				}

				// Split line into two parts: flag data type and description
				// Tip: Use debugger to understand the logic
				re := regexp.MustCompile(`\s{2,}`)
				spl := re.Split(lines[k], -1)
				if len(spl) != 3 {
					continue
				}

				// Styling the flag description
				if cfd != nil {
					lines[k] = strings.Replace(lines[k], spl[2], cfd.Sprint(spl[2]), 1)
				}

				// Styling flag data type
				// Tip: Use debugger to understand the logic
				if cfdt != nil {
					re = regexp.MustCompile(`\s+(\w+)$`) // the last word after spaces is the flag data type
					m := re.FindAllStringSubmatch(spl[1], -1)
					if len(m) == 1 && len(m[0]) == 2 {
						lines[k] = strings.Replace(lines[k], m[0][1], cfdt.Sprint(m[0][1]), 1)
					}
				}

			}
			s = strings.Join(lines, "\n")

			return s
		})

		// Patch usage template
		re := regexp.MustCompile(`(?i)(\.(InheritedFlags|LocalFlags)\.FlagUsages)`)
		tpl = re.ReplaceAllString(tpl, "FlagStyle $1")
	}

	//
	// Styling aliases
	//
	if theme.Styles.Help.Aliases != nil {
		ca := theme.Styles.Help.Aliases
		cobra.AddTemplateFunc("AliasStyle", ca.SprintFunc())

		re := regexp.MustCompile(`(?i){{\s*.NameAndAliases\s*}}`)
		tpl = re.ReplaceAllLiteralString(tpl, "{{AliasStyle .NameAndAliases}}")
	}

	//
	// Styling the example text
	//
	if theme.Styles.Help.Example != nil {
		ce := theme.Styles.Help.Example
		cobra.AddTemplateFunc("ExampleStyle", ce.SprintFunc())

		re := regexp.MustCompile(`(?i){{\s*.Example\s*}}`)
		tpl = re.ReplaceAllLiteralString(tpl, "{{ExampleStyle .Example}}")
	}

	// Adding a new line to the end
	if !cfg.NoBottomNewline {
		tpl += "\n"
	}
	// Apply patched template
	cfg.RootCmd.SetUsageTemplate(tpl)
	// Debug line, uncomment when needed
	// fmt.Println(tpl)
}
