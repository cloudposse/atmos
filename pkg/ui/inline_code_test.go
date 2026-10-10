package ui

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInlineCode(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{name: "plain command", text: "atmos list stacks", want: "`atmos list stacks`"},
		{name: "a backtick inside cannot end the span early", text: "echo `date`", want: "`echo 'date'`"},
		{name: "markdown characters are left alone, a code span shows them literally", text: "a_b *c* x>y", want: "`a_b *c* x>y`"},
		{name: "empty", text: "", want: "``"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, InlineCode(tt.text))
		})
	}
}
