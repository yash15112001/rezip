package repackage

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// helper to create a zip at path with entries name->content (in-memory buffer)
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
		if _, err := io.Copy(w, strings.NewReader(content)); err != nil {
			return err
		}
	}
	return nil
}

func TestFlattenSimple(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.zip")
	out := filepath.Join(dir, "out.zip")

	entries := map[string]string{
		"foo/bar/baz.txt": "a",
		"dir1/file1.txt":  "b",
	}
	if err := makeZip(in, entries); err != nil {
		t.Fatalf("makeZip: %v", err)
	}

	got, err := Run(in, out)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// Expect base names only
	for _, want := range []string{"baz.txt", "file1.txt"} {
		if _, ok := got[want]; !ok {
			t.Errorf("missing %q in result", want)
		}
	}
	if len(got) != 2 {
		t.Errorf("expected 2 entries, got %d", len(got))
	}
}

func TestDedupBySize(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.zip")
	out := filepath.Join(dir, "out.zip")

	entries := map[string]string{
		"a/x.txt": "small",
		"b/x.txt": "much larger content",
	}
	if err := makeZip(in, entries); err != nil {
		t.Fatalf("makeZip: %v", err)
	}

	got, err := Run(in, out)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(got))
	}
	info := got["x.txt"]
	if !strings.HasPrefix(info.OriginalPath, "b/") {
		t.Errorf("expected larger entry b/x.txt, got %q", info.OriginalPath)
	}
}

func TestDedupHashConflict(t *testing.T) {
	// two same-name same-size but different contents -> error
	dir := t.TempDir()
	inZip := filepath.Join(dir, "in.zip")
	// both 3 bytes
	entries := map[string]string{
		"a/z.bin": "foo",
		"b/z.bin": "bar",
	}
	if err := makeZip(inZip, entries); err != nil {
		t.Fatalf("makeZip: %v", err)
	}
	outZip := filepath.Join(dir, "out.zip")
	_, err := Run(inZip, outZip)
	if err == nil {
		t.Fatal("expected conflict error, got nil")
	}
	// Should report identical sizes but differing content
	if !strings.Contains(err.Error(), "identical sizes but differing content") {
		t.Errorf("unexpected error, got %v", err)
	}
}

func TestDedupHashSame(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.zip")
	out := filepath.Join(dir, "out.zip")

	entries := map[string]string{
		"a/y.dat": "abc",
		"b/y.dat": "abc",
	}
	if err := makeZip(in, entries); err != nil {
		t.Fatalf("makeZip: %v", err)
	}

	got, err := Run(in, out)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(got))
	}
}

func TestSkipMetadata(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.zip")
	out := filepath.Join(dir, "out.zip")

	entries := map[string]string{
		"__MACOSX/foo":  "m",
		"bar/.DS_Store": "n",
		"baz/good.txt":  "ok",
	}
	if err := makeZip(in, entries); err != nil {
		t.Fatalf("makeZip: %v", err)
	}

	got, err := Run(in, out)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(got))
	}
	if _, ok := got["good.txt"]; !ok {
		t.Error("expected good.txt in result")
	}
}

func TestOutputZipContents(t *testing.T) {
	// ensure output ZIP actually contains the flattened entries, uncompressed
	dir := t.TempDir()
	in := filepath.Join(dir, "in.zip")
	out := filepath.Join(dir, "out.zip")
	entries := map[string]string{"a/foo.txt": "hello"}
	if err := makeZip(in, entries); err != nil {
		t.Fatalf("makeZip: %v", err)
	}

	if _, err := Run(in, out); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// open and inspect
	zr, err := zip.OpenReader(out)
	if err != nil {
		t.Fatalf("open output zip: %v", err)
	}
	defer zr.Close()

	if len(zr.File) != 1 {
		t.Fatalf("expected 1 file in output, got %d", len(zr.File))
	}
	fh := zr.File[0]
	if fh.Name != "foo.txt" {
		t.Errorf("expected name foo.txt, got %s", fh.Name)
	}
	if fh.Method != zip.Store {
		t.Errorf("expected uncompressed STORE, got method %d", fh.Method)
	}
	rc, err := fh.Open()
	if err != nil {
		t.Fatalf("open entry: %v", err)
	}
	defer rc.Close()
	buf := new(bytes.Buffer)
	if _, err := io.Copy(buf, rc); err != nil {
		t.Fatalf("read entry: %v", err)
	}
	if got := buf.String(); got != "hello" {
		t.Errorf("expected content 'hello', got %q", got)
	}
}
