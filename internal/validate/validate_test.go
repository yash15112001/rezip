package validate

import (
	"archive/zip"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yash15112001/rezip/internal/repackage"
)

// helper to create a zip file at path with given entries mapping name->content
func makeZip(path string, entries map[string]string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	defer zw.Close()

	for name, content := range entries {
		w, err := zw.Create(name)
		if err != nil {
			return err
		}
		if _, err := w.Write([]byte(content)); err != nil {
			return err
		}
	}
	return nil
}

func TestValidate_Success(t *testing.T) {
	// Setup
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "out.zip")
	entries := map[string]string{
		"a.txt": "hello",
		"b.txt": "world",
	}
	if err := makeZip(zipPath, entries); err != nil {
		t.Fatalf("failed to create test zip: %v", err)
	}

	// Build expectedFiles map using actual hashes
	expected := make(map[string]repackage.FileInfo)
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatalf("opening zip: %v", err)
	}
	defer r.Close()
	for _, f := range r.File {
		hash, err := repackage.HashOf(f)
		if err != nil {
			t.Fatalf("hashing entry: %v", err)
		}
		expected[f.Name] = repackage.FileInfo{OriginalPath: f.Name, Hash: hash}
	}

	// Execute
	ok, err := Run(zipPath, expected)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !ok {
		t.Fatal("expected ok==true")
	}

	// Check report file exists and is valid JSON
	reportPath := filepath.Join(dir, "out_validation.json")
	data, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("reading report: %v", err)
	}
	var results []validationResult
	if err := json.Unmarshal(data, &results); err != nil {
		t.Fatalf("invalid JSON in report: %v", err)
	}
	if len(results) != len(entries) {
		t.Fatalf("expected %d results, got %d", len(entries), len(results))
	}
}

func TestValidate_MissingFile(t *testing.T) {
	// Setup zip with single entry
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "out.zip")
	entries := map[string]string{"a.txt": "hello"}
	if err := makeZip(zipPath, entries); err != nil {
		t.Fatalf("create zip: %v", err)
	}

	// expectedFiles has extra key
	expected := map[string]repackage.FileInfo{
		"a.txt": {OriginalPath: "a.txt", Hash: [32]byte{}},
		"b.txt": {OriginalPath: "b.txt", Hash: [32]byte{}},
	}

	// Execute
	_, err := Run(zipPath, expected)
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
	if !contains(err.Error(), "missing file in output zip: b.txt") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidate_HashMismatch(t *testing.T) {
	// Setup zip with a.txt
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "out.zip")
	entries := map[string]string{"a.txt": "foo"}
	if err := makeZip(zipPath, entries); err != nil {
		t.Fatalf("create zip: %v", err)
	}

	// expectedFiles uses wrong hash for a.txt
	var wrongHash [32]byte
	for i := range wrongHash {
		wrongHash[i] = 0xFF
	}
	expected := map[string]repackage.FileInfo{
		"a.txt": {OriginalPath: "a.txt", Hash: wrongHash},
	}

	// Execute
	ok, err := Run(zipPath, expected)
	if err != nil {
		t.Fatalf("expected no error on mismatch, got %v", err)
	}
	if ok {
		t.Fatal("expected ok==false on hash mismatch")
	}

	// Report file should still be written
	reportPath := filepath.Join(dir, "out_validation.json")
	if _, err := os.Stat(reportPath); err != nil {
		t.Fatalf("expected report file, got error: %v", err)
	}
}

// contains is a helper for substring checks
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
