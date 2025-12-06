package errors

import (
	"fmt"
	"os"
	"testing"

	"github.com/cockroachdb/errors"
)

// TestExampleErrorFormatting demonstrates error formatting and can be run manually.
// Run with: go test -v ./errors -run TestExampleErrorFormatting.
func TestExampleErrorFormatting(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping example test in short mode")
	}

	fmt.Fprintf(os.Stderr, "\n=== Example 1: Simple Error ===\n")
	err1 := errors.New("component 'vpc' not found in stack 'prod/us-east-1'")
	formatted1 := Format(err1, FormatterConfig{
		Verbose:       false,
		MaxLineLength: 80,
	})
	fmt.Fprintf(os.Stderr, "%s\n\n", formatted1)

	fmt.Fprintf(os.Stderr, "=== Example 2: Error with Single Hint ===\n")
	err2 := errors.New("component 'vpc' not found")
	err2 = errors.WithHint(err2, "Use 'atmos list components --stack prod/us-east-1' to see available components")
	formatted2 := Format(err2, FormatterConfig{
		Verbose:       false,
		MaxLineLength: 80,
	})
	fmt.Fprintf(os.Stderr, "%s\n\n", formatted2)

	fmt.Fprintf(os.Stderr, "=== Example 3: Error with Multiple Hints ===\n")
	err3 := errors.New("failed to connect to database")
	err3 = errors.WithHint(err3, "Check database credentials in atmos.yaml")
	err3 = errors.WithHint(err3, "Verify network connectivity to database host")
	err3 = errors.WithHint(err3, "Ensure database is running and accessible")
	formatted3 := Format(err3, FormatterConfig{
		Verbose:       false,
		MaxLineLength: 80,
	})
	fmt.Fprintf(os.Stderr, "%s\n\n", formatted3)

	fmt.Fprintf(os.Stderr, "=== Example 4: Long Error Chain (Collapsed) ===\n")
	err4 := errors.New("dial tcp 10.0.1.5:5432: i/o timeout")
	err4 = errors.Wrap(err4, "connection refused")
	err4 = errors.Wrap(err4, "failed to connect to database")
	err4 = errors.Wrap(err4, "failed to initialize component vpc")
	err4 = errors.WithHint(err4, "Check database connectivity")
	err4 = errors.WithHint(err4, "Verify database credentials in atmos.yaml")
	formatted4 := Format(err4, FormatterConfig{
		Verbose:       false,
		MaxLineLength: 80,
	})
	fmt.Fprintf(os.Stderr, "%s\n\n", formatted4)

	fmt.Fprintf(os.Stderr, "=== Example 5: Same Error Chain (Verbose) ===\n")
	formatted5 := Format(err4, FormatterConfig{
		Verbose:       true,
		MaxLineLength: 80,
	})
	fmt.Fprintf(os.Stderr, "%s\n\n", formatted5)

	fmt.Fprintf(os.Stderr, "=== Example 6: Builder Pattern ===\n")
	err6 := Build(errors.New("authentication failed")).
		WithHint("Check your AWS credentials").
		WithHintf("Ensure the AWS profile '%s' is configured", "prod-admin").
		WithContext("aws_profile", "prod-admin").
		WithContext("region", "us-east-1").
		WithExitCode(2).
		Err()
	formatted6 := Format(err6, FormatterConfig{
		Verbose:       false,
		MaxLineLength: 80,
	})
	fmt.Fprintf(os.Stderr, "%s\n\n", formatted6)

	fmt.Fprintf(os.Stderr, "=== Example 7: Very Long Error Message (Wrapped) ===\n")
	err7 := errors.New("failed to initialize component vpc in stack prod/us-east-1 with terraform workspace prod-use1 due to configuration validation error in file stacks/prod/us-east-1.yaml")
	formatted7 := Format(err7, FormatterConfig{
		Verbose:       false,
		MaxLineLength: 80,
	})
	fmt.Fprintf(os.Stderr, "%s\n\n", formatted7)
}
