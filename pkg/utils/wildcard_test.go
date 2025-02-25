package utils

import (
	"testing"
)

func TestMatchWildcard(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		str     string
		want    bool
		wantErr bool
	}{
		{
			name:    "empty pattern",
			pattern: "",
			str:     "anything",
			want:    true,
			wantErr: false,
		},
		{
			name:    "exact match",
			pattern: "file.txt",
			str:     "file.txt",
			want:    true,
			wantErr: false,
		},
		{
			name:    "single star",
			pattern: "*.txt",
			str:     "file.txt",
			want:    true,
			wantErr: false,
		},
		{
			name:    "single star no match",
			pattern: "*.txt",
			str:     "file.log",
			want:    false,
			wantErr: false,
		},
		{
			name:    "question mark",
			pattern: "file.???",
			str:     "file.txt",
			want:    true,
			wantErr: false,
		},
		{
			name:    "character class",
			pattern: "file.[tl]og",
			str:     "file.log",
			want:    true,
			wantErr: false,
		},
		{
			name:    "character range",
			pattern: "file[a-z].txt",
			str:     "filea.txt",
			want:    true,
			wantErr: false,
		},
		{
			name:    "double star directory matching",
			pattern: "dir/**/*.txt",
			str:     "dir/subdir/file.txt",
			want:    true,
			wantErr: false,
		},
		{
			name:    "double star deep directory matching",
			pattern: "dir/**/*.txt",
			str:     "dir/subdir/another/deep/file.txt",
			want:    true,
			wantErr: false,
		},
		{
			name:    "double star with single file match",
			pattern: "dir/**",
			str:     "dir/file.txt",
			want:    true,
			wantErr: false,
		},
		{
			name:    "double star no match",
			pattern: "dir/**/*.txt",
			str:     "other/subdir/file.txt",
			want:    false,
			wantErr: false,
		},
		// Stack name pattern tests
		{
			name:    "stack environment pattern match",
			pattern: "*-dev-*",
			str:     "tenant1-dev-us-east-1",
			want:    true,
			wantErr: false,
		},
		{
			name:    "stack environment pattern no match",
			pattern: "*-dev-*",
			str:     "tenant1-prod-us-east-1",
			want:    false,
			wantErr: false,
		},
		{
			name:    "stack environment brace expansion match dev",
			pattern: "*-{dev,staging}-*",
			str:     "tenant1-dev-us-east-1",
			want:    true,
			wantErr: false,
		},
		{
			name:    "stack environment brace expansion match staging",
			pattern: "*-{dev,staging}-*",
			str:     "tenant1-staging-us-east-1",
			want:    true,
			wantErr: false,
		},
		{
			name:    "stack environment brace expansion no match",
			pattern: "*-{dev,staging}-*",
			str:     "tenant1-prod-us-east-1",
			want:    false,
			wantErr: false,
		},
		{
			name:    "stack with region pattern match",
			pattern: "*-us-east-*",
			str:     "tenant1-prod-us-east-1",
			want:    true,
			wantErr: false,
		},
		{
			name:    "stack with region and environment pattern match",
			pattern: "*-dev-*-east-*",
			str:     "tenant1-dev-us-east-1",
			want:    true,
			wantErr: false,
		},
		{
			name:    "stack with tenant pattern match",
			pattern: "tenant1-*",
			str:     "tenant1-dev-us-east-1",
			want:    true,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := MatchWildcard(tt.pattern, tt.str)

			// Check error
			if (err != nil) != tt.wantErr {
				t.Errorf("MatchWildcard() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Check result
			if got != tt.want {
				t.Errorf("MatchWildcard() = %v, want %v", got, tt.want)
			}
		})
	}
}
