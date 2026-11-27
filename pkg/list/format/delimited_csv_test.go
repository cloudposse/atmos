package format

import (
	"encoding/csv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A lone carriage return inside a field must survive the CRLF (Windows) writer path.
func TestWriteCSV_PreservesLoneCarriageReturnWithCRLF(t *testing.T) {
	records := [][]string{{"Key", "dev"}, {"note", "a\rb"}, {"multi", "x\ny"}}

	out, err := writeCSV(records, ',', true)
	require.NoError(t, err)

	got, err := csv.NewReader(strings.NewReader(out)).ReadAll()
	require.NoError(t, err)
	assert.Equal(t, "a\rb", got[1][1])
}

// Without carriage returns in the data, CRLF record terminators are still used.
func TestWriteCSV_UsesCRLFWhenSafe(t *testing.T) {
	out, err := writeCSV([][]string{{"a", "b"}, {"1", "2"}}, ',', true)
	require.NoError(t, err)
	assert.Equal(t, "a,b\r\n1,2\r\n", out)
}
