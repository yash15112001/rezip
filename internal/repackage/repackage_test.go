package repackage

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRun(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("Returns error when input ZIP doesn't exist", func(t *testing.T) {
		inputPath := filepath.Join(tempDir, "nonexistent.zip")
		outputPath := filepath.Join(tempDir, "output.zip")

		_, err := Run(inputPath, outputPath)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to open input zip")
	})

	t.Run("Returns error when flatten and deduplication fails", func(t *testing.T) {
		// Create test ZIP with files having same name, same size, but different content.
		// This will cause the flattenAndDeduplicate function to return an error.
		inputPath := filepath.Join(tempDir, "hash_conflict_input.zip")
		outputPath := filepath.Join(tempDir, "hash_conflict_output.zip")

		entries := map[string]string{
			"a/conflict.txt": "content1",
			"b/conflict.txt": "content2", // Same size, different content.
		}
		err := makeTestZip(inputPath, entries)
		require.NoError(t, err, "Failed to create test ZIP file")

		_, err = Run(inputPath, outputPath)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "identical sizes but differing content")
	})

	t.Run("Returns error when output ZIP cannot be created", func(t *testing.T) {
		inputPath := filepath.Join(tempDir, "valid_input.zip")
		entries := map[string]string{"test.txt": "content"}
		err := makeTestZip(inputPath, entries)
		require.NoError(t, err, "Failed to create test ZIP file")

		// Try to write to a non-existent directory.
		nonExistentDir := filepath.Join(tempDir, "nonexistent")
		outputPath := filepath.Join(nonExistentDir, "output.zip")

		_, err = Run(inputPath, outputPath)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create output file")
	})

	t.Run("Successfully repackages ZIP", func(t *testing.T) {
		inputPath := filepath.Join(tempDir, "success_input.zip")
		outputPath := filepath.Join(tempDir, "success_output.zip")

		entries := map[string]string{
			"foo/bar/file1.txt":    "content1",
			"dir/file2.txt":        "content2",
			"__MACOSX/ignored.txt": "should be ignored",
		}
		err := makeTestZip(inputPath, entries)
		require.NoError(t, err, "Failed to create test ZIP file")

		result, err := Run(inputPath, outputPath)

		assert.NoError(t, err)
		assert.Len(t, result, 2, "Expected 2 files in output")
		assert.Contains(t, result, "file1.txt")
		assert.Contains(t, result, "file2.txt")
		assert.Equal(t, "foo/bar/file1.txt", result["file1.txt"].OriginalPath)
		assert.Equal(t, "dir/file2.txt", result["file2.txt"].OriginalPath)

		// Verify output ZIP exists and is readable.
		zipReader, err := zip.OpenReader(outputPath)
		assert.NoError(t, err, "Output ZIP should be readable")
		defer zipReader.Close()

		// Verify file count in output ZIP.
		assert.Len(t, zipReader.File, 2, "Output ZIP should contain 2 files")

		// Verify content is preserved.
		assertZipHasExpectedContent(t, outputPath, "file1.txt", "content1")
		assertZipHasExpectedContent(t, outputPath, "file2.txt", "content2")
	})

	t.Run("End-to-end test with all features", func(t *testing.T) {
		inputPath := filepath.Join(tempDir, "endtoend_input.zip")
		outputPath := filepath.Join(tempDir, "endtoend_output.zip")

		entries := map[string]string{
			"a/foo.txt":           "small",
			"b/foo.txt":           "larger content",
			"deep/nested/bar.txt": "test content",
			"__MACOSX/ignore.txt": "metadata",
			".DS_Store":           "more metadata",
		}

		err := makeTestZip(inputPath, entries)
		require.NoError(t, err, "Failed to create test ZIP file")

		result, err := Run(inputPath, outputPath)

		assert.NoError(t, err)
		assert.Len(t, result, 2, "Should have 2 files after processing")
		assert.Contains(t, result, "foo.txt")
		assert.Contains(t, result, "bar.txt")
		assert.Equal(t, "b/foo.txt", result["foo.txt"].OriginalPath)
		assert.Equal(t, "deep/nested/bar.txt", result["bar.txt"].OriginalPath)

		assertZipHasExpectedContent(t, outputPath, "foo.txt", "larger content")
		assertZipHasExpectedContent(t, outputPath, "bar.txt", "test content")
	})
}

func TestFlattenAndDeduplicate(t *testing.T) {
	t.Run("Returns error when files with same name and size have different hash", func(t *testing.T) {
		// Create files with same name and size but different content.
		file1 := createTestZipFile("dir1/file.txt", "content1")
		file2 := createTestZipFile("dir2/file.txt", "content2")

		_, err := flattenAndDeduplicate([]*zip.File{file1, file2})

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "identical sizes but differing content")
	})

	t.Run("Successfully skips directory entries", func(t *testing.T) {
		// Create a mix of file and directory entries.
		fileEntry := createTestZipFile("dir/file.txt", "content")
		dirEntry := createTestZipDir("dir/")

		result, err := flattenAndDeduplicate([]*zip.File{fileEntry, dirEntry})

		assert.NoError(t, err)
		assert.Len(t, result, 1, "Expected only the file entry")
		assert.Equal(t, fileEntry, result["file.txt"])
		assert.NotContains(t, result, "dir", "Directory entry should be skipped")
	})

	t.Run("Successfully skips macOS-based symlinks", func(t *testing.T) {
		// Create a regular file and a symlink.
		regularFile := createTestZipFile("dir/file.txt", "content")
		symlinkFile := createTestZipSymlink("dir/symlink.txt", "target.txt")

		result, err := flattenAndDeduplicate([]*zip.File{regularFile, symlinkFile})

		assert.NoError(t, err)
		assert.Len(t, result, 1, "Expected only the regular file")
		assert.Equal(t, regularFile, result["file.txt"])
		assert.NotContains(t, result, "symlink.txt", "Symlink should be skipped")
	})

	t.Run("Successfully skips metadata files", func(t *testing.T) {
		// Create a regular file and various metadata files.
		regularFile := createTestZipFile("dir/file.txt", "content")
		macosxFile := createTestZipFile("__MACOSX/file.txt", "metadata")
		dsStoreFile := createTestZipFile(".DS_Store", "metadata")
		thumbsFile := createTestZipFile("Thumbs.db", "windows metadata")

		result, err := flattenAndDeduplicate(
			[]*zip.File{regularFile, macosxFile, dsStoreFile, thumbsFile},
		)

		assert.NoError(t, err)
		assert.Len(t, result, 1, "Expected only the regular file")
		assert.Equal(t, regularFile, result["file.txt"])
		assert.NotContains(t, result, ".DS_Store", "Metadata file should be skipped")
		assert.NotContains(t, result, "__MACOSX", "Metadata directory should be skipped")
		assert.NotContains(t, result, "Thumbs.db", "Windows metadata file should be skipped")
	})

	t.Run("Successfully keeps larger file when deduplicating", func(t *testing.T) {
		// Create two files with same name but different sizes.
		smallFile := createTestZipFile("dir1/file.txt", "small")
		largeFile := createTestZipFile("dir2/file.txt", "larger content")

		result, err := flattenAndDeduplicate([]*zip.File{smallFile, largeFile})

		assert.NoError(t, err)
		assert.Len(t, result, 1, "Expected 1 file after deduplication")
		assert.Equal(t, largeFile, result["file.txt"], "Larger file should be kept")
	})

	t.Run("Successfully handles combination of all cases", func(t *testing.T) {
		// Create a comprehensive test with all types of entries.
		entries := []*zip.File{
			// Regular files to keep.
			createTestZipFile("dir1/keep1.txt", "content1"),
			createTestZipFile("dir2/keep2.txt", "content2"),

			// Directory to skip.
			createTestZipDir("skipdir/"),

			// Symlinks to skip.
			createTestZipSymlink("symlink1.txt", "target.txt"),

			// Metadata files to skip.
			createTestZipFile("__MACOSX/skip.txt", "metadata"),
			createTestZipFile(".DS_Store", "metadata"),

			// Duplicate files with different sizes.
			createTestZipFile("dup/small.txt", "small"),
			createTestZipFile("another/small.txt", "larger content"),
		}

		result, err := flattenAndDeduplicate(entries)

		assert.NoError(t, err)
		assert.Len(t, result, 3, "Expected 3 files after processing")
		assert.Contains(t, result, "keep1.txt")
		assert.Contains(t, result, "keep2.txt")
		assert.Contains(t, result, "small.txt")

		contentSize := len([]byte("larger content"))
		assert.Equal(t, int64(contentSize), result["small.txt"].FileInfo().Size())

		assert.NotContains(t, result, "skipdir")
		assert.NotContains(t, result, "symlink1.txt")
		assert.NotContains(t, result, "skip.txt")
		assert.NotContains(t, result, ".DS_Store")
	})
}

func TestCreateOutputZip(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("Creates uncompressed output ZIP file", func(t *testing.T) {
		inputPath := filepath.Join(tempDir, "in.zip")
		outputPath := filepath.Join(tempDir, "out.zip")

		// Make a test ZIP file to get real zip.File entries.
		err := makeTestZip(inputPath, map[string]string{
			"file1.txt": "content1",
			"file2.txt": "content2",
		})
		require.NoError(t, err)

		reader, err := zip.OpenReader(inputPath)
		require.NoError(t, err)
		defer reader.Close()

		deduplicatedFiles := make(map[string]*zip.File)
		for _, file := range reader.File {
			deduplicatedFiles[file.Name] = file
		}

		fileRegistry, err := createOutputZip(deduplicatedFiles, outputPath)

		assert.NoError(t, err)
		assert.Len(t, fileRegistry, 2, "Should have metadata for 2 files")

		zipReader, err := zip.OpenReader(outputPath)
		require.NoError(t, err)
		defer zipReader.Close()

		assert.Len(t, zipReader.File, 2, "Output ZIP should contain 2 files")

		// Check compression method.
		for _, file := range zipReader.File {
			assert.Equal(t, zip.Store, file.Method, "Files should be stored uncompressed")
		}

		assertZipHasExpectedContent(t, outputPath, "file1.txt", "content1")
		assertZipHasExpectedContent(t, outputPath, "file2.txt", "content2")
	})

	t.Run("Returns error when output file cannot be created", func(t *testing.T) {
		// Try to create output in a non-existent directory.
		nonExistentPath := filepath.Join(tempDir, "nonexistent", "output.zip")

		_, err := createOutputZip(map[string]*zip.File{}, nonExistentPath)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create output file")
	})
}

func TestReadZipFileContent(t *testing.T) {
	t.Run("Returns error when output zip file cannot be created", func(t *testing.T) {
		tempDir := t.TempDir()

		// Attempt to create a ZIP file in a non-existent directory.
		nonExistentDir := filepath.Join(tempDir, "non-existent")
		outputPath := filepath.Join(nonExistentDir, "output.zip")

		_, err := createOutputZip(map[string]*zip.File{}, outputPath)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create output file")
	})

	t.Run("Returns error when certain zipEntry cannot be written to output zip", func(t *testing.T) {
		tempDir := t.TempDir()
		outputPath := filepath.Join(tempDir, "output.zip")

		// Create a test file that will cause writing to fail.
		// Using a corrupted zip file with a manipulated header.
		file := createTestZipFile("test.txt", "content")
		file.Method = 999 // Invalid method - will cause error when writing.

		deduplicatedFiles := map[string]*zip.File{
			"test.txt": file,
		}

		_, err := createOutputZip(deduplicatedFiles, outputPath)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to write and hash file")
	})

	t.Run("Successfully creates output zip with all deduplicated files", func(t *testing.T) {
		tempDir := t.TempDir()
		outputPath := filepath.Join(tempDir, "output.zip")
		sourcePath := filepath.Join(tempDir, "source.zip")

		entries := map[string]string{
			"dir1/file1.txt": "content1",
			"dir2/file2.txt": "content2",
		}
		err := makeTestZip(sourcePath, entries)
		require.NoError(t, err)

		reader, err := zip.OpenReader(sourcePath)
		require.NoError(t, err)
		defer reader.Close()

		deduplicatedFiles := make(map[string]*zip.File)
		for _, file := range reader.File {
			deduplicatedFiles[filepath.Base(file.Name)] = file
		}

		registry, err := createOutputZip(deduplicatedFiles, outputPath)

		assert.NoError(t, err)
		assert.Len(t, registry, 2)
		assert.Contains(t, registry, "file1.txt")
		assert.Contains(t, registry, "file2.txt")
		assert.Equal(t, "dir1/file1.txt", registry["file1.txt"].OriginalPath)
		assert.Equal(t, "dir2/file2.txt", registry["file2.txt"].OriginalPath)

		// Verify the output zip file was created correctly.
		zipReader, err := zip.OpenReader(outputPath)
		assert.NoError(t, err)
		defer zipReader.Close()

		assert.Len(t, zipReader.File, 2)
		assertZipHasExpectedContent(t, outputPath, "file1.txt", "content1")
		assertZipHasExpectedContent(t, outputPath, "file2.txt", "content2")
	})
}

func makeTestZip(path string, entries map[string]string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	zipWriter := zip.NewWriter(file)
	defer zipWriter.Close()

	for name, content := range entries {
		fileWriter, err := zipWriter.Create(name)
		if err != nil {
			return err
		}
		_, err = io.Copy(fileWriter, strings.NewReader(content))
		if err != nil {
			return err
		}
	}
	return nil
}

func createTestZipFile(name, content string) *zip.File {
	// Create a temporary buffer to hold our zip file.
	buf := new(bytes.Buffer)
	zipWriter := zip.NewWriter(buf)

	// Create the file entry.
	writer, _ := zipWriter.Create(name)
	writer.Write([]byte(content))

	zipWriter.Close()

	reader, _ := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))

	// Return the first (and only) file.
	return reader.File[0]
}

func createTestZipDir(name string) *zip.File {
	buf := new(bytes.Buffer)
	zipWriter := zip.NewWriter(buf)

	zipWriter.CreateHeader(&zip.FileHeader{
		Name:   name,
		Method: zip.Store,
	})

	zipWriter.Close()

	reader, _ := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))

	// Return the first (and only) file which is the directory.
	return reader.File[0]
}

func createTestZipSymlink(name, target string) *zip.File {
	// Create a temporary buffer to hold our zip file.
	buf := new(bytes.Buffer)
	zipWriter := zip.NewWriter(buf)

	header := &zip.FileHeader{
		Name:   name,
		Method: zip.Store,
	}

	// Set symlink bit in external attributes (Unix format).
	const symlink = ioReparseSymlink
	header.SetMode(symlink)

	writer, _ := zipWriter.CreateHeader(header)
	writer.Write([]byte(target))

	zipWriter.Close()

	reader, _ := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))

	// Return the first (and only) file which is the symlink.
	return reader.File[0]
}

func readZipFileContent(file *zip.File) (string, error) {
	reader, err := file.Open()
	if err != nil {
		return "", err
	}
	defer reader.Close()

	buffer := new(bytes.Buffer)
	if _, err = io.Copy(buffer, reader); err != nil {
		return "", err
	}

	return buffer.String(), nil
}

// assertZipHasExpectedContent verifies that the ZIP file contains a file with the expected content.
func assertZipHasExpectedContent(t *testing.T, zipPath, fileName, expectedContent string) {
	zipReader, err := zip.OpenReader(zipPath)
	require.NoError(t, err, "Failed to open ZIP file")
	defer zipReader.Close()

	var found bool
	for _, file := range zipReader.File {
		if file.Name == fileName {
			found = true
			content, err := readZipFileContent(file)
			assert.NoError(t, err)
			assert.Equal(t, expectedContent, content)
			break
		}
	}

	assert.True(t, found, "File %s not found in ZIP", fileName)
}
