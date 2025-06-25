package cmd

import (
	"testing"
)

func Test_checkErrorAndExit(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		wantErr bool
	}{
		{
			name:    "nil error should not exit",
			err:     nil,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantErr {
				defer func() {
					if r := recover(); r == nil {
						t.Errorf("checkErrorAndExit() should have panicked with error")
					}
				}()
			}
			checkErrorAndExit(tt.err)
		})
	}
}
