package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func writeGoMod(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "go.mod")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	return path
}

func TestRun(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr error
	}{
		{
			name: "clean go.mod passes",
			content: `module example.com/app

go 1.26

require (
	github.com/spf13/cobra v1.8.0
	github.com/aws/aws-sdk-go-v2 v1.36.3 // indirect
)
`,
			wantErr: nil,
		},
		{
			name: "block-form forbidden require fails",
			content: `module example.com/app

go 1.26

require (
	github.com/spf13/cobra v1.8.0
	github.com/aws/aws-sdk-go v1.55.8 // indirect
)
`,
			wantErr: errForbiddenModuleUsed,
		},
		{
			name: "inline-form forbidden require fails",
			content: `module example.com/app

go 1.26

require github.com/aws/aws-sdk-go v1.55.8
`,
			wantErr: errForbiddenModuleUsed,
		},
		{
			name: "aws-sdk-go-v2 in block form passes",
			content: `module example.com/app

go 1.26

require (
	github.com/aws/aws-sdk-go-v2 v1.36.3 // indirect
	github.com/aws/aws-sdk-go-v2/config v1.29.0
	github.com/hashicorp/aws-sdk-go-base/v2 v2.0.0
)
`,
			wantErr: nil,
		},
		{
			name: "commented-out forbidden module passes",
			content: `module example.com/app

go 1.26

require (
	github.com/spf13/cobra v1.8.0
	// github.com/aws/aws-sdk-go v1.55.8
)
`,
			wantErr: nil,
		},
		{
			name: "replace directive still fails",
			content: `module example.com/app

go 1.26

require github.com/spf13/cobra v1.8.0

replace github.com/spf13/cobra => ../cobra
`,
			wantErr: errReplaceNotAllowed,
		},
		{
			name: "exclude directive still fails",
			content: `module example.com/app

go 1.26

exclude github.com/spf13/cobra v1.7.0
`,
			wantErr: errExcludeNotAllowed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := run(writeGoMod(t, tt.content))
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("run() error = %v, want nil", err)
				}
				return
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("run() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestRunMissingFile(t *testing.T) {
	err := run(filepath.Join(t.TempDir(), "does-not-exist", "go.mod"))
	if err == nil {
		t.Fatal("run() error = nil, want open error")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("run() error = %v, want os.ErrNotExist", err)
	}
}

func TestRequireModulePath(t *testing.T) {
	tests := []struct {
		name    string
		trimmed string
		inBlock bool
		want    string
	}{
		{name: "block line", trimmed: "github.com/aws/aws-sdk-go v1.55.8 // indirect", inBlock: true, want: "github.com/aws/aws-sdk-go"},
		{name: "block line v2", trimmed: "github.com/aws/aws-sdk-go-v2 v1.36.3 // indirect", inBlock: true, want: "github.com/aws/aws-sdk-go-v2"},
		{name: "inline require", trimmed: "require github.com/aws/aws-sdk-go v1.55.8", inBlock: false, want: "github.com/aws/aws-sdk-go"},
		{name: "inline require block opener", trimmed: "require (", inBlock: false, want: ""},
		{name: "comment in block", trimmed: "// github.com/aws/aws-sdk-go v1.55.8", inBlock: true, want: ""},
		{name: "comment outside block", trimmed: "// require github.com/aws/aws-sdk-go v1.55.8", inBlock: false, want: ""},
		{name: "blank in block", trimmed: "", inBlock: true, want: ""},
		{name: "module directive outside block", trimmed: "module example.com/app", inBlock: false, want: ""},
		{name: "go directive outside block", trimmed: "go 1.26", inBlock: false, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := requireModulePath(tt.trimmed, tt.inBlock); got != tt.want {
				t.Fatalf("requireModulePath(%q, %v) = %q, want %q", tt.trimmed, tt.inBlock, got, tt.want)
			}
		})
	}
}
