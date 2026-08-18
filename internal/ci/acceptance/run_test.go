package acceptance

import (
	"reflect"
	"testing"
)

func TestSplitCommandLinePreservesQuoting(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		value string
		want  []string
	}{
		{name: "empty", value: "", want: []string{}},
		{name: "simple flags", value: "-v -short", want: []string{"-v", "-short"}},
		{
			name:  "quoted run pattern",
			value: `-run '^TestFoo$' -v`,
			want:  []string{"-run", "^TestFoo$", "-v"},
		},
		{
			name:  "double quoted value with spaces",
			value: `-run "^TestFoo bar$"`,
			want:  []string{"-run", "^TestFoo bar$"},
		},
		{
			name:  "newline separated",
			value: "-run\n^TestFoo$",
			want:  []string{"-run", "^TestFoo$"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := SplitCommandLine(testCase.value)
			if err != nil {
				t.Fatalf("split command line %q: %v", testCase.value, err)
			}
			if !reflect.DeepEqual(got, testCase.want) {
				t.Fatalf("split command line %q = %#v, want %#v", testCase.value, got, testCase.want)
			}
		})
	}
}

func TestSplitCommandLineRejectsUnterminatedQuotes(t *testing.T) {
	t.Parallel()

	if _, err := SplitCommandLine(`-run "^TestFoo$`); err == nil {
		t.Fatal("expected an error for an unterminated quote")
	}
}
