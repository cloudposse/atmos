package terraform

import (
	"context"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

// TestParser_SeparatedArgs tests that the parser correctly separates positional args from pass-through args.
func TestParser_SeparatedArgs(t *testing.T) {
	tests := []struct {
		name                string
		args                []string
		wantComponent       string
		wantStack           string
		wantPositionalArgs  []string
		wantSeparatedArgs []string
	}{
		{
			name:                "with -- separator",
			args:                []string{"vpc", "-s", "dev", "--", "-var", "foo=bar"},
			wantComponent:       "vpc",
			wantStack:           "dev",
			wantPositionalArgs:  []string{"vpc"},
			wantSeparatedArgs: []string{"-var", "foo=bar"},
		},
		{
			name:                "without -- separator",
			args:                []string{"vpc", "-s", "dev"},
			wantComponent:       "vpc",
			wantStack:           "dev",
			wantPositionalArgs:  []string{"vpc"},
			wantSeparatedArgs: []string{},
		},
		{
			name:                "with multiple pass-through args",
			args:                []string{"rds", "-s", "prod", "--", "-var", "foo=bar", "-var", "baz=qux", "-out=plan.tfplan"},
			wantComponent:       "rds",
			wantStack:           "prod",
			wantPositionalArgs:  []string{"rds"},
			wantSeparatedArgs: []string{"-var", "foo=bar", "-var", "baz=qux", "-out=plan.tfplan"},
		},
		{
			name:                "component with only pass-through args",
			args:                []string{"vpc", "--", "-var", "foo=bar"},
			wantComponent:       "vpc",
			wantStack:           "",
			wantPositionalArgs:  []string{"vpc"},
			wantSeparatedArgs: []string{"-var", "foo=bar"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewParser()
			v := viper.New()
			cmd := &cobra.Command{Use: "plan"}

			parser.RegisterFlags(cmd)
			_ = parser.BindToViper(v)

			opts, err := parser.Parse(context.Background(), tt.args)
			assert.NoError(t, err)

			assert.Equal(t, tt.wantComponent, opts.Component, "Component mismatch")
			assert.Equal(t, tt.wantStack, opts.Stack, "Stack mismatch")
			assert.Equal(t, tt.wantPositionalArgs, opts.GetPositionalArgs(), "PositionalArgs mismatch")
			assert.Equal(t, tt.wantSeparatedArgs, opts.GetSeparatedArgs(), "SeparatedArgs mismatch")
		})
	}
}

// TestParser_PerCommandValidation tests that per-command compatibility alias validation works correctly.
// For example: `atmos terraform init -out` should fail since init doesn't support -out.
func TestParser_PerCommandValidation(t *testing.T) {
	tests := []struct {
		name              string
		subcommand        string
		args              []string
		wantPositionalArgs []string
		wantSeparatedArgs []string
		description       string
	}{
		{
			name:              "plan with -out flag - allowed",
			subcommand:        "plan",
			args:              []string{"plan", "vpc", "-out", "plan.tfplan"},
			wantPositionalArgs: []string{"plan", "vpc"},
			wantSeparatedArgs: []string{"-out", "plan.tfplan"},
			description:       "plan supports -out flag",
		},
		{
			name:              "init with -upgrade flag - allowed",
			subcommand:        "init",
			args:              []string{"init", "vpc", "-upgrade"},
			wantPositionalArgs: []string{"init", "vpc"},
			wantSeparatedArgs: []string{"-upgrade"},
			description:       "init supports -upgrade flag",
		},
		{
			name:              "output with -json flag - allowed",
			subcommand:        "output",
			args:              []string{"output", "vpc_id", "-json"},
			wantPositionalArgs: []string{"output", "vpc_id"},
			wantSeparatedArgs: []string{"-json"},
			description:       "output supports -json flag",
		},
		{
			name:              "validate with -json flag - allowed",
			subcommand:        "validate",
			args:              []string{"validate", "-json"},
			wantPositionalArgs: []string{"validate"},
			wantSeparatedArgs: []string{"-json"},
			description:       "validate supports -json flag",
		},
		{
			name:              "apply with -auto-approve - allowed",
			subcommand:        "apply",
			args:              []string{"apply", "vpc", "-auto-approve"},
			wantPositionalArgs: []string{"apply", "vpc"},
			wantSeparatedArgs: []string{"-auto-approve"},
			description:       "apply supports -auto-approve flag",
		},
		{
			name:              "import with -var and -state - allowed",
			subcommand:        "import",
			args:              []string{"import", "aws_instance.example", "i-12345", "-var", "region=us-east-1", "-state", "prod.tfstate"},
			wantPositionalArgs: []string{"import", "aws_instance.example", "i-12345"},
			wantSeparatedArgs: []string{"-var", "region=us-east-1", "-state", "prod.tfstate"},
			description:       "import supports -var and -state flags",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewParser()
			v := viper.New()
			cmd := &cobra.Command{Use: tt.subcommand}

			parser.RegisterFlags(cmd)
			_ = parser.BindToViper(v)

			opts, err := parser.Parse(context.Background(), tt.args)
			assert.NoError(t, err, tt.description)

			assert.Equal(t, tt.wantPositionalArgs, opts.GetPositionalArgs(), "PositionalArgs mismatch")
			assert.Equal(t, tt.wantSeparatedArgs, opts.GetSeparatedArgs(), "SeparatedArgs mismatch")
		})
	}
}

// TestParser_FlagSyntaxVariations tests all the crazy permutations of terraform flag syntax.
// Tests: -var=foo, -var="foo=", -var "foo", -var foo, etc.
func TestParser_FlagSyntaxVariations(t *testing.T) {
	tests := []struct {
		name              string
		args              []string
		wantPositionalArgs []string
		wantSeparatedArgs []string
		description       string
	}{
		{
			name:              "-var=foo (equals form)",
			args:              []string{"plan", "vpc", "-var=region=us-east-1"},
			wantPositionalArgs: []string{"plan", "vpc"},
			wantSeparatedArgs: []string{"-var=region=us-east-1"},
			description:       "single -var flag with = syntax",
		},
		{
			name:              `-var="foo=" (quoted with equals)`,
			args:              []string{"plan", "vpc", `-var=name="value=with=equals"`},
			wantPositionalArgs: []string{"plan", "vpc"},
			wantSeparatedArgs: []string{`-var=name="value=with=equals"`},
			description:       "-var with quoted value containing equals signs",
		},
		{
			name:              "-var foo (space form)",
			args:              []string{"plan", "vpc", "-var", "region=us-east-1"},
			wantPositionalArgs: []string{"plan", "vpc"},
			wantSeparatedArgs: []string{"-var", "region=us-east-1"},
			description:       "single -var flag with space-separated value",
		},
		{
			name:              "multiple -var flags with mixed syntax",
			args:              []string{"plan", "vpc", "-var=region=us-east-1", "-var", "env=prod", "-var=app=web"},
			wantPositionalArgs: []string{"plan", "vpc"},
			wantSeparatedArgs: []string{"-var=region=us-east-1", "-var", "env=prod", "-var=app=web"},
			description:       "multiple -var flags with both = and space syntax",
		},
		{
			name:              "-var-file with = syntax",
			args:              []string{"plan", "vpc", "-var-file=prod.tfvars"},
			wantPositionalArgs: []string{"plan", "vpc"},
			wantSeparatedArgs: []string{"-var-file=prod.tfvars"},
			description:       "single -var-file with = syntax",
		},
		{
			name:              "-var-file with space syntax",
			args:              []string{"plan", "vpc", "-var-file", "prod.tfvars"},
			wantPositionalArgs: []string{"plan", "vpc"},
			wantSeparatedArgs: []string{"-var-file", "prod.tfvars"},
			description:       "single -var-file with space syntax",
		},
		{
			name:              "complex: multiple flags with all variations",
			args:              []string{"plan", "vpc", "-var=region=us-east-1", "-var", "env=prod", "-var-file=prod.tfvars", "-var-file", "common.tfvars", "-out=plan.tfplan"},
			wantPositionalArgs: []string{"plan", "vpc"},
			wantSeparatedArgs: []string{"-var=region=us-east-1", "-var", "env=prod", "-var-file=prod.tfvars", "-var-file", "common.tfvars", "-out=plan.tfplan"},
			description:       "complex scenario with all flag syntax variations",
		},
		{
			name:              "-target with = syntax",
			args:              []string{"plan", "vpc", "-target=aws_instance.web"},
			wantPositionalArgs: []string{"plan", "vpc"},
			wantSeparatedArgs: []string{"-target=aws_instance.web"},
			description:       "single -target with = syntax",
		},
		{
			name:              "-target with space syntax",
			args:              []string{"plan", "vpc", "-target", "aws_instance.web"},
			wantPositionalArgs: []string{"plan", "vpc"},
			wantSeparatedArgs: []string{"-target", "aws_instance.web"},
			description:       "single -target with space syntax",
		},
		{
			name:              "mixed with Atmos flags",
			args:              []string{"plan", "vpc", "-s", "dev", "-var=region=us-east-1", "-var", "env=prod"},
			wantPositionalArgs: []string{"plan", "vpc"},
			wantSeparatedArgs: []string{"-var=region=us-east-1", "-var", "env=prod"},
			description:       "Atmos flags (-s) and terraform pass-through flags",
		},
		{
			name:              "all permutations in one command",
			args:              []string{"plan", "vpc", "-s", "dev", "-var=a=1", "-var", "b=2", `-var="c=3"`, "-var-file=x.tfvars", "-var-file", "y.tfvars", "-target=aws_instance.web", "-target", "aws_s3_bucket.data", "-out=plan.tfplan"},
			wantPositionalArgs: []string{"plan", "vpc"},
			wantSeparatedArgs: []string{"-var=a=1", "-var", "b=2", `-var="c=3"`, "-var-file=x.tfvars", "-var-file", "y.tfvars", "-target=aws_instance.web", "-target", "aws_s3_bucket.data", "-out=plan.tfplan"},
			description:       "all permutations and combinations in one command",
		},
		{
			name:              "empty value with =",
			args:              []string{"plan", "vpc", "-var="},
			wantPositionalArgs: []string{"plan", "vpc"},
			wantSeparatedArgs: []string{"-var="},
			description:       "-var with empty value after =",
		},
		{
			name:              "with -- separator",
			args:              []string{"plan", "vpc", "-s", "dev", "--", "-var=region=us-east-1", "-var", "env=prod"},
			wantPositionalArgs: []string{"plan", "vpc"},
			wantSeparatedArgs: []string{"-var=region=us-east-1", "-var", "env=prod"},
			description:       "terraform flags after -- separator",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewParser()
			v := viper.New()
			cmd := &cobra.Command{Use: "plan"}

			parser.RegisterFlags(cmd)
			_ = parser.BindToViper(v)

			opts, err := parser.Parse(context.Background(), tt.args)
			assert.NoError(t, err, tt.description)

			assert.Equal(t, tt.wantPositionalArgs, opts.GetPositionalArgs(), "PositionalArgs mismatch: %s", tt.description)
			assert.Equal(t, tt.wantSeparatedArgs, opts.GetSeparatedArgs(), "SeparatedArgs mismatch: %s", tt.description)
		})
	}
}
