package args

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseArgs(t *testing.T) {
	// capture original os.Args
	savedArgs := os.Args
	defer func() { os.Args = savedArgs }()

	tmp := t.TempDir()

	// helper to create a valid zip file
	makeZip := func(path string) {
		f, err := os.Create(path)
		testErr(t, err)
		w := zip.NewWriter(f)
		testErr(t, w.Close())
		testErr(t, f.Close())
	}

	// prepare existing valid input
	validZip := filepath.Join(tmp, "in.zip")
	makeZip(validZip)

	tests := []struct {
		name        string
		argv        []string
		prepare     func()
		wantErr     bool
		errContains string
	}{
		{
			name:        "too few args",
			argv:        []string{"rezip"},
			wantErr:     true,
			errContains: "invalid number of arguments",
		},
		{
			name:        "unknown flag",
			argv:        []string{"rezip", validZip, filepath.Join(tmp, "out.zip"), "-foo"},
			wantErr:     true,
			errContains: "unknown option",
		},
		{
			name:        "input does not exist",
			argv:        []string{"rezip", filepath.Join(tmp, "no.zip"), filepath.Join(tmp, "out.zip")},
			wantErr:     true,
			errContains: "does not exist",
		},
		{
			name:        "input is directory",
			argv:        []string{"rezip", tmp, filepath.Join(tmp, "out.zip")},
			wantErr:     true,
			errContains: "is a directory",
		},
		{
			name: "input not readable",
			argv: []string{"rezip", filepath.Join(tmp, "bad.zip"), filepath.Join(tmp, "out.zip")},
			prepare: func() {
				f := filepath.Join(tmp, "bad.zip")
				os.WriteFile(f, []byte("notzip"), 0o400)
			},
			wantErr:     true,
			errContains: "not a valid zip",
		},
		{
			name:        "output dir does not exist",
			argv:        []string{"rezip", validZip, filepath.Join(tmp, "no", "out.zip")},
			wantErr:     true,
			errContains: "output directory does not exist",
		},
		{
			name: "output dir not writable",
			argv: []string{"rezip", validZip, filepath.Join(tmp, "out", "out.zip")},
			prepare: func() {
				d := filepath.Join(tmp, "out")

				os.Mkdir(d, 0o500)
			},
			wantErr:     true,
			errContains: "is not writable",
		},
		{
			name:        "input equals output",
			argv:        []string{"rezip", validZip, validZip},
			wantErr:     true,
			errContains: "cannot be the same file",
		},
		{
			name:    "valid no validate",
			argv:    []string{"rezip", validZip, filepath.Join(tmp, "out.zip")},
			wantErr: false,
		},
		{
			name:    "valid with validate",
			argv:    []string{"rezip", validZip, filepath.Join(tmp, "out.zip"), "--validate"},
			wantErr: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.prepare != nil {
				tc.prepare()
			}
			os.Args = append([]string{"rezip"}, tc.argv[1:]...)
			cfg, err := Parse()
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.errContains)
				}
				if !strings.Contains(err.Error(), tc.errContains) {
					t.Fatalf("expected error containing %q, got %q", tc.errContains, err.Error())
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				// basic sanity
				if cfg.InputZipPath == "" || cfg.OutputZipPath == "" {
					t.Fatalf("parsed empty paths: %+v", cfg)
				}
			}
		})
	}
}

// testErr fails the test on error.
func testErr(t *testing.T, err error) {
	if err != nil {
		t.Fatalf("setup error: %v", err)
	}
}
