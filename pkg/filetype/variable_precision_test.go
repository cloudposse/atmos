package filetype

import (
	"bytes"
	"encoding/json"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVariableFilePrecision(t *testing.T) {
	const large = "123456789012345678901234567890123456789"
	const fraction = "0.12345678901234567890123456789"
	for _, tc := range []struct{ path, source string }{
		{"dev.tfvars", "values = [" + large + ", null, " + fraction + ", 1.75]"},
		{"prod.pkrvars.hcl", "values = [" + large + ", null, " + fraction + ", 1.75]"},
		{"dev.tfvars.json", `{"values":[` + large + `,null,` + fraction + `,1.75]}`},
		{"prod.pkrvars.json", `{"values":[` + large + `,null,` + fraction + `,1.75]}`},
	} {
		t.Run(tc.path, func(t *testing.T) {
			got, err := ParseFileByExtension(func(string) ([]byte, error) { return []byte(tc.source), nil }, tc.path)
			require.NoError(t, err)
			data, err := json.Marshal(got)
			require.NoError(t, err)
			// Compare exact decimals without rounding either side through float64.
			var actual struct{ Values []any }
			d := json.NewDecoder(bytes.NewReader(data))
			d.UseNumber()
			require.NoError(t, d.Decode(&actual))
			require.Len(t, actual.Values, 4)
			require.Nil(t, actual.Values[1])
			for i, want := range map[int]string{0: large, 2: fraction, 3: "1.75"} {
				token, ok := actual.Values[i].(json.Number)
				require.True(t, ok)
				number, ok := new(big.Rat).SetString(token.String())
				require.True(t, ok)
				expected, ok := new(big.Rat).SetString(want)
				require.True(t, ok)
				require.Zero(t, number.Cmp(expected))
			}
		})
	}
	_, err := ParseByExtension([]byte("{} {}"), ".json", "prod.tfvars.json")
	require.Error(t, err)
}
