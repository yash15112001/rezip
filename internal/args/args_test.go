package args

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParse(t *testing.T) {
	// Preserve and restore original os.Args.
	savedArgs := os.Args
	defer func() { os.Args = savedArgs }()

	// Create temp directory to store and maintain test zip files.
	tmpDir := t.TempDir()

	// Create a valid zip file for testing.
	validZipPath := filepath.Join(tmpDir, "valid.zip")
	createValidZip(t, validZipPath)

	t.Run("Returns error with too few arguments", func(t *testing.T) {
		os.Args = []string{"rezip"}

		config, err := Parse()

		assert.Error(t, err)
		assert.Nil(t, config)
		assert.Contains(t, err.Error(), "invalid number of arguments")
	})

	t.Run("Returns error with too many arguments", func(t *testing.T) {
		os.Args = []string{"rezip", validZipPath, filepath.Join(tmpDir, "out.zip"), "--validate", "extra"}

		config, err := Parse()

		assert.Error(t, err)
		assert.Nil(t, config)
		assert.Contains(t, err.Error(), "invalid number of arguments")
	})

	t.Run("Returns error with unknown option", func(t *testing.T) {
		os.Args = []string{"rezip", validZipPath, filepath.Join(tmpDir, "out.zip"), "--unknown"}

		config, err := Parse()

		assert.Error(t, err)
		assert.Nil(t, config)
		assert.Contains(t, err.Error(), "unknown option")
	})

	t.Run("Returns error when input file validation fails", func(t *testing.T) {
		nonExistentFile := filepath.Join(tmpDir, "nonexistent.zip")
		os.Args = []string{"rezip", nonExistentFile, filepath.Join(tmpDir, "out.zip")}

		config, err := Parse()

		assert.Error(t, err)
		assert.Nil(t, config)
		assert.Contains(t, err.Error(), "input zip file does not exist")
	})

	t.Run("Returns error when output directory validation fails", func(t *testing.T) {
		nonExistentDir := filepath.Join(tmpDir, "nonexistent-dir")
		outputPath := filepath.Join(nonExistentDir, "out.zip")
		os.Args = []string{"rezip", validZipPath, outputPath}

		config, err := Parse()

		assert.Error(t, err)
		assert.Nil(t, config)
		assert.Contains(t, err.Error(), "output directory does not exist")
	})

	t.Run("Returns error when distinct paths validation fails", func(t *testing.T) {
		os.Args = []string{"rezip", validZipPath, validZipPath}

		config, err := Parse()

		assert.Error(t, err)
		assert.Nil(t, config)
		assert.Contains(t, err.Error(), "cannot be the same file")
	})

	t.Run("Successfully parses without validate flag", func(t *testing.T) {
		outputPath := filepath.Join(tmpDir, "output.zip")
		os.Args = []string{"rezip", validZipPath, outputPath}

		config, err := Parse()

		assert.NoError(t, err)
		assert.NotNil(t, config)
		assert.Equal(t, validZipPath, config.InputZipPath)
		assert.Equal(t, outputPath, config.OutputZipPath)
		assert.False(t, config.Validate)
	})

	t.Run("Successfully parses with validate flag", func(t *testing.T) {
		outputPath := filepath.Join(tmpDir, "output.zip")
		os.Args = []string{"rezip", validZipPath, outputPath, "--validate"}

		config, err := Parse()

		assert.NoError(t, err)
		assert.NotNil(t, config)
		assert.Equal(t, validZipPath, config.InputZipPath)
		assert.Equal(t, outputPath, config.OutputZipPath)
		assert.True(t, config.Validate)
	})
}

func TestValidateInputFile(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("Returns error when input file does not exist", func(t *testing.T) {
		nonExistentPath := filepath.Join(tmpDir, "nonexistent.zip")

		err := validateInputFile(nonExistentPath)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "input zip file does not exist")
	})

	t.Run("Returns error when input is a directory", func(t *testing.T) {
		err := validateInputFile(tmpDir)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "input is a directory")
	})

	t.Run("Returns error when input file has no read permissions", func(t *testing.T) {
		noReadPath := filepath.Join(tmpDir, "noread.zip")
		file, err := os.Create(noReadPath)
		assert.NoError(t, err, "setup failed")
		file.Close()

		err = createValidZipFile(noReadPath)
		assert.NoError(t, err, "setup failed")

		// Remove read permissions.
		err = os.Chmod(noReadPath, 0200)
		assert.NoError(t, err, "setup failed")

		err = validateInputFile(noReadPath)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not readable")
	})

	t.Run("Returns error when input is not a valid zip", func(t *testing.T) {
		// Create a file that is not a valid zip.
		invalidZipPath := filepath.Join(tmpDir, "invalid.zip")
		err := os.WriteFile(invalidZipPath, []byte("not a zip file"), 0o644)
		assert.NoError(t, err, "setup failed")

		err = validateInputFile(invalidZipPath)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not a valid zip")
	})

	t.Run("Returns no error with valid zip file", func(t *testing.T) {
		validZipPath := filepath.Join(tmpDir, "valid.zip")
		err := createValidZipFile(validZipPath)
		assert.NoError(t, err, "setup failed")

		err = validateInputFile(validZipPath)

		assert.NoError(t, err)
	})
}

func TestValidateOutputDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("Returns error when output directory does not exist", func(t *testing.T) {
		nonExistentDir := filepath.Join(tmpDir, "nonexistent")
		outputPath := filepath.Join(nonExistentDir, "out.zip")

		err := validateOutputDirectory(outputPath)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "output directory does not exist")
	})

	t.Run("Returns error when output directory is not a directory", func(t *testing.T) {
		// Create a file instead of a directory to simulate a non-directory.
		filePath := filepath.Join(tmpDir, "file")
		err := os.WriteFile(filePath, []byte("test"), 0644)
		assert.NoError(t, err, "setup failed")

		outputPath := filepath.Join(filePath, "out.zip")

		err = validateOutputDirectory(outputPath)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not a directory")
	})

	t.Run("Returns error when output directory is not writable", func(t *testing.T) {
		// Create directory with no write permissions.
		noWriteDir := filepath.Join(tmpDir, "nowrite")
		err := os.Mkdir(noWriteDir, 0500)
		assert.NoError(t, err, "setup failed")

		outputPath := filepath.Join(noWriteDir, "out.zip")

		err = validateOutputDirectory(outputPath)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not writable")
	})

	t.Run("Returns no error with valid output directory", func(t *testing.T) {
		outputPath := filepath.Join(tmpDir, "out.zip")

		err := validateOutputDirectory(outputPath)

		assert.NoError(t, err)
	})
}

func TestValidateDistinctPaths(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("Returns error when input and output paths are the same", func(t *testing.T) {
		path := filepath.Join(tmpDir, "same.zip")

		err := validateDistinctPaths(path, path)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be the same file")
	})

	t.Run("Returns error when input and output paths resolve to the same file", func(t *testing.T) {
		absPath, err := filepath.Abs(tmpDir)
		assert.NoError(t, err, "setup failed")

		path1 := filepath.Join(absPath, "file.zip")

		// Create another path by going up one directory and back down.
		// This creates a different path string that resolves to the same file.
		path2 := filepath.Join(tmpDir, "..", filepath.Base(tmpDir), "file.zip")

		err = validateDistinctPaths(path1, path2)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be the same file")
	})

	t.Run("Returns no error when input and output paths are different", func(t *testing.T) {
		inputPath := filepath.Join(tmpDir, "input.zip")
		outputPath := filepath.Join(tmpDir, "output.zip")

		err := validateDistinctPaths(inputPath, outputPath)

		assert.NoError(t, err)
	})
}

func createValidZip(t *testing.T, path string) {
	err := createValidZipFile(path)
	assert.NoError(t, err, "Failed to create test zip file")
}

func createValidZipFile(path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	zipWriter := zip.NewWriter(file)
	defer zipWriter.Close()

	return nil
}
