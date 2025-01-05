package cmd

import (
	"testing"
)

func TestHasPositionalArgs(t *testing.T) {
	testCases := []struct {
		name string
		args []string
		want bool
	}{
		{
			name: "NoArgs",
			args: []string{},
			want: false,
		},
		{
			name: "OneArg",
			args: []string{"arg1"},
			want: true,
		},
		{
			name: "TwoArgs",
			args: []string{"arg1", "arg2"},
			want: true,
		},
		{
			name: "ArgAndFlagWithParam",
			args: []string{"arg1", "--flag", "param"},
			want: true,
		},
		{
			name: "Flag",
			args: []string{"--flag"},
			want: false,
		},
		{
			name: "FlagAndArg",
			args: []string{"--flag", "arg1"},
			want: false,
		},
		{
			name: "FlagAndArgAndFlag",
			args: []string{"--flag", "arg1", "--flag"},
			want: false,
		},
		{
			name: "FlagsWithArgsWithPositionalArg",
			args: []string{"--flag=something", "ted"},
			want: true,
		},
		{
			name: "FlagsWithoutArgsWithEqual",
			args: []string{"--flag=something"},
			want: false,
		},
		{
			name: "terrraformLikeArgs",
			args: []string{"-flag1=hello", "-flag2"},
			want: false,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := hasPositionalArgs(tc.args)
			if got != tc.want {
				t.Errorf("HasPositionalArgs() = %v, want %v", got, tc.want)
			}
		})
	}
}
