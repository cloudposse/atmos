package cmd

import (
	"testing"
)

func TestCastFlagDoesNotConsumeNextArgument(t *testing.T) {
	flag := RootCmd.PersistentFlags().Lookup("cast")
	if flag == nil {
		t.Fatal("cast flag is not registered")
	}
	if flag.NoOptDefVal == "" {
		t.Fatal("cast flag must support bare --cast")
	}

	args := []string{"--cast", "terraform", "plan"}
	parsed := preprocessNoOptDefValFlags(args)
	if len(parsed) != len(args) {
		t.Fatalf("cast flag preprocessing consumed an argument: got %#v", parsed)
	}
	for i := range args {
		if parsed[i] != args[i] {
			t.Fatalf("unexpected preprocessing result: got %#v want %#v", parsed, args)
		}
	}
}
