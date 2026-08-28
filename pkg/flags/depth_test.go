package flags

import (
	"errors"
	"testing"

	errUtils "github.com/cloudposse/atmos/errors"
)

// TestParseClosureDepth covers the depth-carrying closure flag encoding:
// absent → off, bare/all/true → unlimited, N → depth, false/0 → off,
// anything else → ErrInvalidFlagValue.
func TestParseClosureDepth(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		value   string
		want    int
		wantErr bool
	}{
		{name: "absent", value: "", want: 0},
		{name: "bare_flag_no_opt_def_val", value: ClosureDepthUnlimited, want: -1},
		{name: "true_compat", value: "true", want: -1},
		{name: "true_uppercase_compat", value: "TRUE", want: -1},
		{name: "false_compat", value: "false", want: 0},
		{name: "zero_off", value: "0", want: 0},
		{name: "depth_one", value: "1", want: 1},
		{name: "depth_three", value: "3", want: 3},
		{name: "whitespace_trimmed", value: " 2 ", want: 2},
		{name: "negative_rejected", value: "-2", wantErr: true},
		{name: "garbage_rejected", value: "banana", wantErr: true},
		{name: "float_rejected", value: "1.5", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseClosureDepth("include-dependencies", tc.value)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ParseClosureDepth(%q) = %d, want error", tc.value, got)
				}
				if !errors.Is(err, errUtils.ErrInvalidFlagValue) {
					t.Fatalf("ParseClosureDepth(%q) error = %v, want ErrInvalidFlagValue", tc.value, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseClosureDepth(%q) unexpected error: %v", tc.value, err)
			}
			if got != tc.want {
				t.Fatalf("ParseClosureDepth(%q) = %d, want %d", tc.value, got, tc.want)
			}
		})
	}
}
