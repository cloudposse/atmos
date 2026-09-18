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

func TestWildcardRelPath(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		path    string
		want    string
	}{
		{
			name:    "empty pattern returns path unchanged",
			pattern: "",
			path:    "components/vpc/main.tf",
			want:    "components/vpc/main.tf",
		},
		{
			// Regression case: doublestar.SplitPattern always splits off the
			// final path segment regardless of whether it's a glob, so an
			// ungated call would incorrectly strip "components/" here.
			name:    "literal path with no glob metacharacter is unaffected",
			pattern: "components/main.tf",
			path:    "components/main.tf",
			want:    "components/main.tf",
		},
		{
			name:    "literal path with no directory is unaffected",
			pattern: "main.tf",
			path:    "main.tf",
			want:    "main.tf",
		},
		{
			name:    "recursive double-star glob strips its literal base",
			pattern: "components/**",
			path:    "components/vpc/main.tf",
			want:    "vpc/main.tf",
		},
		{
			name:    "single-level glob strips its literal base",
			pattern: "components/*/main.tf",
			path:    "components/vpc/main.tf",
			want:    "vpc/main.tf",
		},
		{
			name:    "glob at the root has no base to strip",
			pattern: "**",
			path:    "components/vpc/main.tf",
			want:    "components/vpc/main.tf",
		},
		{
			name:    "brace expansion glob strips its literal base",
			pattern: "components/{vpc,vpc2}/main.tf",
			path:    "components/vpc/main.tf",
			want:    "vpc/main.tf",
		},
		{
			// Regression case: a backslash-authored pattern must behave
			// identically to its forward-slash equivalent, regardless of the
			// OS running this test -- filepath.ToSlash alone is a no-op for
			// backslashes on macOS/Linux, since it only replaces the *host*
			// OS's own separator character.
			name:    "backslash-authored glob strips its literal base the same as forward-slash",
			pattern: `components\**`,
			path:    "components/vpc/main.tf",
			want:    "vpc/main.tf",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := WildcardRelPath(tt.pattern, tt.path)
			if got != tt.want {
				t.Errorf("WildcardRelPath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNormalizeGlobPattern(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		want    string
	}{
		{name: "no backslashes is unchanged", pattern: "components/**", want: "components/**"},
		{name: "backslash directory separators become forward slashes", pattern: `components\vpc\main.tf`, want: "components/vpc/main.tf"},
		{name: "mixed separators are all normalized", pattern: `components\vpc/main.tf`, want: "components/vpc/main.tf"},
		{name: "empty pattern stays empty", pattern: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeGlobPattern(tt.pattern)
			if got != tt.want {
				t.Errorf("NormalizeGlobPattern() = %q, want %q", got, tt.want)
			}
		})
	}
}
