package renderer

import (
	"encoding/csv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDelimitedFieldsRoundTrip(t *testing.T) {
	headers := []string{"Name", "Description, quoted"}
	rows := [][]string{
		{"acme/auth", "Login, with \"quotes\"\nand\ttabs"},
		{"acme/command", "line\rbreak"},
		{"", ""},
	}
	for _, delimiter := range []string{",", "\t", ";"} {
		t.Run(delimiter, func(t *testing.T) {
			text, err := formatDelimited(headers, rows, delimiter)
			require.NoError(t, err)
			reader := csv.NewReader(strings.NewReader(text))
			reader.Comma = []rune(delimiter)[0]
			actual, err := reader.ReadAll()
			require.NoError(t, err)
			require.Equal(t, append([][]string{headers}, rows...), actual)
		})
	}
}

func TestDelimitedCustomSeparatorAndInputIsolation(t *testing.T) {
	headers := []string{"Plugin", "Description"}
	row := []string{"acme/auth", "one||two"}
	text, err := formatDelimited(headers, [][]string{row}, "||")
	require.NoError(t, err)
	require.Equal(t, "Plugin||Description\nacme/auth||\"one||two\"\n", text)
	require.Equal(t, []string{"Plugin", "Description"}, headers)
	require.Equal(t, []string{"acme/auth", "one||two"}, row)
}
