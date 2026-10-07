package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	// Annotation key for the JSON-encoded positional arguments of a custom command. The help
	// renderer reads it to show an ARGUMENTS section.
	annotationCustomCommandArguments = "customCommandArguments"
	// Flag annotation key that marks a custom command string flag whose default is shown quoted
	// in help. Quoting stops a string default such as "2" from reading as an integer.
	annotationQuoteDefault = "atmos-quote-default"
)

// customCommandUse returns the cobra Use string of a custom command: its name followed by its
// positional arguments, `<name>` for a required one and `[name]` for an optional one.
func customCommandUse(commandConfig *schema.Command) string {
	parts := []string{commandConfig.Name}
	for _, arg := range commandConfig.Arguments {
		if arg.Required && arg.Default == "" {
			parts = append(parts, "<"+arg.Name+">")
		} else {
			parts = append(parts, "["+arg.Name+"]")
		}
	}
	return strings.Join(parts, " ")
}

// setCustomCommandArgumentsAnnotation records the command's arguments for the help renderer.
func setCustomCommandArgumentsAnnotation(cmd *cobra.Command, commandConfig *schema.Command) {
	if len(commandConfig.Arguments) == 0 {
		return
	}
	encoded, err := json.Marshal(commandConfig.Arguments)
	if err != nil {
		return
	}
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations[annotationCustomCommandArguments] = string(encoded)
}

// customCommandArguments returns the positional arguments recorded on a custom command.
func customCommandArguments(cmd *cobra.Command) []schema.CommandArgument {
	raw, ok := cmd.Annotations[annotationCustomCommandArguments]
	if !ok {
		return nil
	}
	var args []schema.CommandArgument
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return nil
	}
	return args
}

// printCustomCommandArguments prints the ARGUMENTS section of a custom command's help: each
// positional argument with its description, whether it is required, its default, and its allowed
// values.
func printCustomCommandArguments(w io.Writer, cmd *cobra.Command, styles *helpStyles) {
	defer perf.Track(nil, "cmd.printCustomCommandArguments")()

	args := customCommandArguments(cmd)
	if len(args) == 0 {
		return
	}
	fmt.Fprintln(w, styles.heading.Render("ARGUMENTS"))
	fmt.Fprintln(w)
	maxWidth := 0
	for _, arg := range args {
		maxWidth = max(maxWidth, len(arg.Name))
	}
	for _, arg := range args {
		name := styles.commandName.Render(fmt.Sprintf("%-*s", maxWidth, arg.Name))
		fmt.Fprintf(w, "  %s  %s\n", name, styles.flagDesc.Render(customCommandArgumentDescription(&arg)))
	}
	fmt.Fprintln(w)
}

// customCommandArgumentDescription builds the help text of one positional argument.
func customCommandArgumentDescription(arg *schema.CommandArgument) string {
	desc := arg.Description
	var notes []string
	if arg.Required && arg.Default == "" {
		notes = append(notes, "required")
	}
	if arg.Default != "" {
		notes = append(notes, fmt.Sprintf("default %q", arg.Default))
	}
	if len(arg.Values) > 0 {
		notes = append(notes, "one of: "+strings.Join(arg.Values, ", "))
	}
	if len(notes) == 0 {
		return desc
	}
	if desc == "" {
		return "(" + strings.Join(notes, "; ") + ")"
	}
	return desc + " (" + strings.Join(notes, "; ") + ")"
}

// markQuotedDefault flags a custom command string flag so help shows its default in quotes.
func markQuotedDefault(cmd *cobra.Command, flagName string) {
	_ = cmd.PersistentFlags().SetAnnotation(flagName, annotationQuoteDefault, []string{annotationValueTrue})
}

// flagDefaultText returns the "(default ...)" suffix of a flag's help, or "" when the flag has no
// default worth showing. A string flag marked with annotationQuoteDefault shows it quoted.
func flagDefaultText(f *pflag.Flag) string {
	if f.DefValue == "" || f.DefValue == "false" || f.DefValue == "0" || f.DefValue == "[]" || f.Name == "" || f.Name == "help" {
		return ""
	}
	if len(f.Annotations[annotationQuoteDefault]) > 0 {
		return fmt.Sprintf(" (default %q)", f.DefValue)
	}
	return fmt.Sprintf(" (default `%s`)", f.DefValue)
}
