package validate

import (
	"archive/zip"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yash15112001/rezip/internal/repackage"
)

func TestRun(t *testing.T) {
	t.Run("Returns error when can't read output zip", func(t *testing.T) {
		tempDir := t.TempDir()
		nonexistentPath := filepath.Join(tempDir, "nonexistent.zip")

		allMatch, err := Run(nonexistentPath, map[string]repackage.FileInfo{})

		assert.Error(t, err)
		assert.False(t, allMatch)
		assert.Contains(t, err.Error(), "failed to open output zip")
	})

	t.Run("Returns error when hash validation fails", func(t *testing.T) {
		tempDir := t.TempDir()
		zipPath := filepath.Join(tempDir, "output.zip")

		entries := map[string]string{
			"file1.txt": "content1",
		}
		makeTestZip(t, zipPath, entries)

		// Create expected files map with a file that doesn't exist in the zip to simulate validation failure.
		expected := map[string]repackage.FileInfo{
			"missing-file.txt": {
				OriginalPath: "original/missing-file.txt",
				Hash:         [32]byte{},
			},
		}

		allMatch, err := Run(zipPath, expected)

		assert.Error(t, err)
		assert.False(t, allMatch)
		assert.Contains(t, err.Error(), "missing file in output zip: missing-file.txt")

		reportPath := filepath.Join(tempDir, "output_validation.json")
		_, err = os.Stat(reportPath)
		assert.True(t, os.IsNotExist(err), "Report file should not exist when validation errors occur")
	})

	t.Run("Returns error when can't write the validation report", func(t *testing.T) {
		tempDir := t.TempDir()

		// Create a read-only subdirectory.
		readOnlyDir := filepath.Join(tempDir, "readonly")
		err := os.Mkdir(readOnlyDir, 0755)
		require.NoError(t, err)

		// Create a test ZIP file
		zipPath := filepath.Join(readOnlyDir, "output.zip")
		entries := map[string]string{
			"file1.txt": "content1",
		}
		makeTestZip(t, zipPath, entries)

		// Build expected files map.
		expected := buildExpectedFilesMap(t, zipPath)

		// Make the directory read-only to cause report writing to fail.
		err = os.Chmod(readOnlyDir, 0555)
		require.NoError(t, err)

		allMatch, err := Run(zipPath, expected)

		assert.Error(t, err)
		assert.False(t, allMatch)
		assert.Contains(t, err.Error(), "failed to create validation report file")

		// Restore permissions for cleanup.
		os.Chmod(readOnlyDir, 0755)
	})

	t.Run("Successfully completes the validation process and writes the validation report", func(t *testing.T) {
		tempDir := t.TempDir()
		zipPath := filepath.Join(tempDir, "output.zip")

		entries := map[string]string{
			"file1.txt": "content1",
			"file2.txt": "content2",
			"file3.txt": "content3",
		}
		makeTestZip(t, zipPath, entries)

		// Build expected files map with correct hashes.
		expected := buildExpectedFilesMap(t, zipPath)

		_, err := Run(zipPath, expected)

		assert.NoError(t, err, "Validation process should complete without errors")

		reportPath := filepath.Join(tempDir, "output_validation.json")
		assert.FileExists(t, reportPath)

		// Verify report contains valid JSON.
		reportData, err := os.ReadFile(reportPath)
		assert.NoError(t, err)

		var results []validationResult
		err = json.Unmarshal(reportData, &results)
		assert.NoError(t, err, "Report should contain valid JSON")
		assert.NotEmpty(t, results, "Report should contain validation results")
	})
}

func TestReadOutputZip(t *testing.T) {
	t.Run("Returns error when can't open output zip", func(t *testing.T) {
		tempDir := t.TempDir()
		nonexistentPath := filepath.Join(tempDir, "nonexistent.zip")

		zipReader, actualFiles, err := readOutputZip(nonexistentPath)

		assert.Error(t, err)
		assert.Nil(t, zipReader)
		assert.Nil(t, actualFiles)
		assert.Contains(t, err.Error(), "failed to open output zip")
	})

	t.Run("Successfully reads output ZIP", func(t *testing.T) {
		tempDir := t.TempDir()
		zipPath := filepath.Join(tempDir, "output.zip")

		entries := map[string]string{
			"file1.txt": "content1",
			"file2.txt": "content2",
		}
		makeTestZip(t, zipPath, entries)

		zipReader, actualFiles, err := readOutputZip(zipPath)

		assert.NoError(t, err)
		assert.NotNil(t, zipReader)
		assert.Len(t, actualFiles, 2, "Should have 2 files")
		assert.Contains(t, actualFiles, "file1.txt")
		assert.Contains(t, actualFiles, "file2.txt")

		defer zipReader.Close()
	})
}

func TestValidateFileHashes(t *testing.T) {
	t.Run("Returns error when certain file is missing in output zip", func(t *testing.T) {
		tempDir := t.TempDir()
		zipPath := filepath.Join(tempDir, "output.zip")

		entries := map[string]string{
			"file1.txt": "content1",
		}
		makeTestZip(t, zipPath, entries)

		zipReader, err := zip.OpenReader(zipPath)
		require.NoError(t, err)
		defer zipReader.Close()

		actualFiles := make(map[string]*zip.File)
		for _, file := range zipReader.File {
			actualFiles[file.Name] = file
		}

		expected := buildExpectedFilesMap(t, zipPath)
		expected["missing.txt"] = repackage.FileInfo{
			OriginalPath: "missing.txt",
			Hash:         [32]byte{},
		}

		results, allMatch, err := validateFileHashes(actualFiles, expected)

		assert.Error(t, err)
		assert.False(t, allMatch)
		assert.Nil(t, results)
		assert.Contains(t, err.Error(), "missing file in output zip: missing.txt")
	})

	t.Run("Successfully validates matching hashes", func(t *testing.T) {
		tempDir := t.TempDir()
		zipPath := filepath.Join(tempDir, "output.zip")

		entries := map[string]string{
			"file1.txt": "content1",
			"file2.txt": "content2",
		}
		makeTestZip(t, zipPath, entries)

		zipReader, err := zip.OpenReader(zipPath)
		require.NoError(t, err)
		defer zipReader.Close()

		actualFiles := make(map[string]*zip.File)
		for _, file := range zipReader.File {
			actualFiles[file.Name] = file
		}
		expected := buildExpectedFilesMap(t, zipPath)

		results, allMatch, err := validateFileHashes(actualFiles, expected)

		assert.NoError(t, err)
		assert.True(t, allMatch)
		assert.Len(t, results, 2)

		for _, result := range results {
			assert.True(t, result.Match)
		}
	})

	t.Run("Successfully returns false with mismatched hashes", func(t *testing.T) {
		tempDir := t.TempDir()
		zipPath := filepath.Join(tempDir, "output.zip")

		entries := map[string]string{
			"file1.txt": "content1",
			"file2.txt": "content2",
		}
		makeTestZip(t, zipPath, entries)

		zipReader, err := zip.OpenReader(zipPath)
		require.NoError(t, err)
		defer zipReader.Close()

		actualFiles := make(map[string]*zip.File)
		for _, file := range zipReader.File {
			actualFiles[file.Name] = file
		}

		// Create expected files map and corrupt one hash.
		expected := buildExpectedFilesMap(t, zipPath)
		expected["file1.txt"] = repackage.FileInfo{
			OriginalPath: expected["file1.txt"].OriginalPath,
			Hash:         corruptHash(expected["file1.txt"].Hash),
		}

		results, allMatch, err := validateFileHashes(actualFiles, expected)

		assert.NoError(t, err)
		assert.False(t, allMatch)
		assert.Len(t, results, 2)
	})
}

func TestWriteValidationReport(t *testing.T) {
	t.Run("Returns error when report cannot be created", func(t *testing.T) {
		tempDir := t.TempDir()

		reportDir := filepath.Join(tempDir, "readonly")
		err := os.Mkdir(reportDir, 0755)
		require.NoError(t, err)

		zipPath := filepath.Join(reportDir, "output.zip")

		// Make the directory read-only to cause file creation to fail.
		err = os.Chmod(reportDir, 0555)
		require.NoError(t, err)

		err = writeValidationReport(zipPath, []validationResult{})

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create validation report file")
	})

	t.Run("Successfully writes validation report", func(t *testing.T) {
		tempDir := t.TempDir()
		zipPath := filepath.Join(tempDir, "output.zip")

		results := []validationResult{
			{
				FileName:     "file1.txt",
				OriginalPath: "original/file1.txt",
				OriginalSHA:  "aabbcc",
				NewSHA:       "aabbcc",
				Match:        true,
			},
			{
				FileName:     "file2.txt",
				OriginalPath: "original/file2.txt",
				OriginalSHA:  "ddeeff",
				NewSHA:       "112233",
				Match:        false,
			},
		}

		err := writeValidationReport(zipPath, results)

		assert.NoError(t, err)

		reportPath := filepath.Join(tempDir, "output_validation.json")
		assert.FileExists(t, reportPath)
	})
}

// Helper to create a zip file at path with given entries mapping name->content.
func makeTestZip(t *testing.T, path string, entries map[string]string) {
	zipFile, err := os.Create(path)
	require.NoError(t, err, "Failed to create test ZIP file")
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	for name, content := range entries {
		zipEntry, err := zipWriter.Create(name)
		require.NoError(t, err, "Failed to create ZIP entry")
		_, err = zipEntry.Write([]byte(content))
		require.NoError(t, err, "Failed to write ZIP entry content")
	}
}

func buildExpectedFilesMap(t *testing.T, zipPath string) map[string]repackage.FileInfo {
	expected := make(map[string]repackage.FileInfo)

	zipReader, err := zip.OpenReader(zipPath)
	require.NoError(t, err, "Failed to open ZIP reader")
	defer zipReader.Close()

	for _, file := range zipReader.File {
		hash, err := repackage.HashOf(file)
		require.NoError(t, err, "Failed to hash ZIP entry")
		expected[file.Name] = repackage.FileInfo{
			OriginalPath: file.Name,
			Hash:         hash,
		}
	}

	return expected
}

func corruptHash(original [32]byte) [32]byte {
	corrupted := original

	// Change a few bytes to create a different hash.
	for i := 0; i < 5; i++ {
		corrupted[i] = ^corrupted[i]
	}
	return corrupted
}
