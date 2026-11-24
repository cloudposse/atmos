package planout

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	for _, test := range []struct {
		arg    string
		want   Arg
		isFlag bool
	}{
		{"-out", Arg{Name: Flag}, true},
		{"--out", Arg{Name: DoubleDashFlag}, true},
		{"-out=a.tfplan", Arg{Name: Flag, Value: "a.tfplan", Joined: true}, true},
		{"--out=a.tfplan", Arg{Name: DoubleDashFlag, Value: "a.tfplan", Joined: true}, true},
		{"-out=", Arg{Name: Flag, Joined: true}, true},
		{"--out=", Arg{Name: DoubleDashFlag, Joined: true}, true},
		{"-outline", Arg{}, false},
		{"---out=a", Arg{}, false},
		{"out=a", Arg{}, false},
		{"-state-out=a", Arg{}, false},
		{"", Arg{}, false},
	} {
		t.Run(test.arg, func(t *testing.T) {
			got, ok := Parse(test.arg)
			require.Equal(t, test.isFlag, ok)
			require.Equal(t, test.want, got)
			require.Equal(t, test.isFlag, IsFlag(test.arg))
		})
	}
}

func TestContains(t *testing.T) {
	require.True(t, Contains([]string{"plan", "--out=a"}))
	require.True(t, Contains([]string{"plan", "-out"}))
	require.False(t, Contains([]string{"plan", "-state-out=a", "-outline"}))
	require.False(t, Contains(nil))
}

func TestValue(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
		want string
	}{
		{"single-dash separate", []string{"plan", "-out", "a.tfplan"}, "a.tfplan"},
		{"single-dash joined", []string{"plan", "-out=a.tfplan"}, "a.tfplan"},
		{"double-dash separate", []string{"plan", "--out", "a.tfplan"}, "a.tfplan"},
		{"double-dash joined", []string{"plan", "--out=a.tfplan"}, "a.tfplan"},
		{"first flag with a value wins", []string{"plan", "-out=", "--out", "b.tfplan", "-out=c.tfplan"}, "b.tfplan"},
		{"separate form at the end has no value", []string{"plan", "--out"}, ""},
		{"empty separate value is skipped", []string{"plan", "-out", "", "-out=c.tfplan"}, "c.tfplan"},
		{"no output flag", []string{"plan", "-state-out=x"}, ""},
		{"no arguments", nil, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.want, Value(test.args))
		})
	}
}
