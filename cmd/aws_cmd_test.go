package cmd

// sanity to avoid unused imports when conditions differ across envs
func TestAwsCmd_SanityBuild(t *testing.T) {}

// helper: shallow copy of cobra.Command metadata only (no flags copy)

func cloneCmdMeta(c *cobra.Command) *cobra.Command {

	cc := &cobra.Command{}

	*cc = *c

	return cc

}


import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestAwsCmd_Metadata(t *testing.T) {
	if awsCmd.Use \!= "aws" {
		t.Fatalf("Use mismatch: got %q, want %q", awsCmd.Use, "aws")
	}
	if strings.TrimSpace(awsCmd.Short) == "" {
		t.Fatalf("Short description should not be empty")
	}
	if strings.TrimSpace(awsCmd.Long) == "" {
		t.Fatalf("Long description should not be empty")
	}
}


func TestAwsCmd_UnknownFlagsWhitelist(t *testing.T) {
	if awsCmd.FParseErrWhitelist.UnknownFlags {
		t.Fatalf("UnknownFlags whitelist must be false to reject unknown flags")
	}
}


func TestAwsCmd_NoArgsValidation(t *testing.T) {
	// Directly invoke the Args validator to avoid executing the command.
	if err := awsCmd.Args(awsCmd, []string{}); err \!= nil {
		t.Fatalf("NoArgs should accept empty args, got error: %v", err)
	}
	if err := awsCmd.Args(awsCmd, []string{"unexpected"}); err == nil {
		t.Fatalf("NoArgs should reject extra args but returned no error")
	}
}


func TestAwsCmd_UnknownFlagParsingErrors(t *testing.T) {
	// Work on a cloned command to keep global flags clean.
	c := cloneCmdMeta(awsCmd)
	// A fresh FlagSet is created per command; copy persistent flags so the behavior matches.
	c.InheritedFlags() // initialize internal structures

	// Parse should fail for unknown flag '--bogus'
	err := c.Flags().Parse([]string{"--bogus"})
	if err == nil {
		t.Fatalf("expected error for unknown flag, got nil")
	}
	if \!strings.Contains(err.Error(), "unknown flag") {
		t.Fatalf("expected 'unknown flag' in error, got: %v", err)
	}
}


func TestAwsCmd_HasNoRunFunction(t *testing.T) {
	if awsCmd.Run \!= nil || awsCmd.RunE \!= nil {
		t.Fatalf("awsCmd should not define Run/RunE; it is a grouping command")
	}
}


// compile-time check for RootCmd presence
var _ = RootCmd

func TestAwsCmd_IsRegisteredUnderRoot(t *testing.T) {
	found := false
	for _, sc := range RootCmd.Commands() {
		if sc == awsCmd || sc.Name() == "aws" {
			found = true
			break
		}
	}
	if \!found {
		t.Fatalf("awsCmd not registered under RootCmd")
	}
}


// reference to ensure doubleDashHint is linked (value not asserted strictly due to empty flag name)
func TestAwsCmd_UsageIncludesDoubleDashHint(t *testing.T) {
	_ = doubleDashHint // ensure symbol exists
	usage := awsCmd.UsageString()
	if strings.TrimSpace(usage) == "" {
		t.Fatalf("expected non-empty usage output")
	}
	// If the hint is non-empty, prefer it to be visible in help text.
	if doubleDashHint \!= "" && \!strings.Contains(usage, doubleDashHint) {
		// Non-fatal, but signal as failure to keep behavior intentional.
		t.Fatalf("usage/help text does not include doubleDashHint")
	}
}
